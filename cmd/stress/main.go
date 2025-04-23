package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/LixenWraith/config"
	"github.com/LixenWraith/log"
)

const (
	totalBursts    = 100
	logsPerBurst   = 500
	maxMessageSize = 10000
	numWorkers     = 500
)

const configFile = "stress_config.toml"
const configBasePath = "logstress" // Base path for log settings in config

// Example TOML content for stress test
var tomlContent = `
# Example stress_config.toml
[logstress]
  level = -4 # Debug
  name = "stress_test"
  directory = "./logs" # Log package will create this
  format = "txt"
  extension = "log"
  show_timestamp = true
  show_level = true
  buffer_size = 500
  max_size_mb = 1 # Force frequent rotation (1MB)
  max_total_size_mb = 20 # Limit total size to force cleanup (20MB)
  min_disk_free_mb = 50
  flush_interval_ms = 50 # ms
  trace_depth = 0
  retention_period_hrs = 0.0028 # ~10 seconds
  retention_check_mins = 0.084 # ~5 seconds
`

var levels = []int64{
	log.LevelDebug,
	log.LevelInfo,
	log.LevelWarn,
	log.LevelError,
}

var logger *log.Logger

func generateRandomMessage(size int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 "
	var sb strings.Builder
	sb.Grow(size)
	for i := 0; i < size; i++ {
		sb.WriteByte(chars[rand.Intn(len(chars))])
	}
	return sb.String()
}

// logBurst simulates a burst of logging activity
func logBurst(burstID int) {
	for i := 0; i < logsPerBurst; i++ {
		level := levels[rand.Intn(len(levels))]
		msgSize := rand.Intn(maxMessageSize) + 10
		msg := generateRandomMessage(msgSize)
		args := []any{
			msg,
			"wkr", burstID % numWorkers,
			"bst", burstID,
			"seq", i,
			"rnd", rand.Int63(),
		}
		switch level {
		case log.LevelDebug:
			logger.Debug(args...)
		case log.LevelInfo:
			logger.Info(args...)
		case log.LevelWarn:
			logger.Warn(args...)
		case log.LevelError:
			logger.Error(args...)
		}
	}
}

// worker goroutine function
func worker(burstChan chan int, wg *sync.WaitGroup, completedBursts *atomic.Int64) {
	defer wg.Done()
	for burstID := range burstChan {
		logBurst(burstID)
		completed := completedBursts.Add(1)
		if completed%10 == 0 || completed == totalBursts {
			fmt.Printf("\rProgress: %d/%d bursts completed", completed, totalBursts)
		}
	}
}

func main() {
	rand.Seed(time.Now().UnixNano()) // Replace rand.New with rand.Seed for compatibility

	fmt.Println("--- Logger Stress Test ---")

	// --- Setup Config ---
	err := os.WriteFile(configFile, []byte(tomlContent), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write dummy config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created dummy config file: %s\n", configFile)
	logsDir := "./logs"       // Match config
	_ = os.RemoveAll(logsDir) // Clean previous run's LOGS directory before starting
	// defer os.Remove(configFile) // Remove to keep the saved config file
	// defer os.RemoveAll(logsDir) // Remove to keep the log directory

	cfg := config.New()
	_, err = cfg.Load(configFile, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v.\n", err)
		os.Exit(1)
	}

	// --- Initialize Logger ---
	logger = log.NewLogger()
	err = logger.Init(cfg, configBasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Logger initialized. Logs will be written to: %s\n", logsDir)

	// --- SAVE CONFIGURATION ---
	err = cfg.Save(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save configuration to '%s': %v\n", configFile, err)
	} else {
		fmt.Printf("Configuration saved to: %s\n", configFile)
	}
	// --- End Save Configuration ---

	fmt.Printf("Starting stress test: %d workers, %d bursts, %d logs/burst.\n",
		numWorkers, totalBursts, logsPerBurst)
	fmt.Println("Watch for 'Logs were dropped' or 'disk full' messages.")
	fmt.Println("Check log directory size and file rotation.")
	fmt.Println("Press Ctrl+C to stop early.")

	// --- Setup Workers and Signal Handling ---
	burstChan := make(chan int, numWorkers)
	var wg sync.WaitGroup
	completedBursts := atomic.Int64{}
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	stopChan := make(chan struct{})

	go func() {
		<-sigChan
		fmt.Println("\n[Signal Received] Stopping burst generation...")
		close(stopChan)
	}()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(burstChan, &wg, &completedBursts)
	}

	// --- Run Test ---
	startTime := time.Now()
	for i := 1; i <= totalBursts; i++ {
		select {
		case burstChan <- i:
		case <-stopChan:
			fmt.Println("[Signal Received] Halting burst submission.")
			goto endLoop
		}
	}
endLoop:
	close(burstChan)

	fmt.Println("\nWaiting for workers to finish...")
	wg.Wait()
	duration := time.Since(startTime)
	finalCompleted := completedBursts.Load()

	fmt.Printf("\n--- Test Finished ---")
	fmt.Printf("\nCompleted %d/%d bursts in %v\n", finalCompleted, totalBursts, duration.Round(time.Millisecond))
	if finalCompleted > 0 && duration.Seconds() > 0 {
		logsPerSec := float64(finalCompleted*logsPerBurst) / duration.Seconds()
		fmt.Printf("Approximate Logs/sec: %.2f\n", logsPerSec)
	}

	// --- Shutdown Logger ---
	fmt.Println("Shutting down logger (allowing up to 10s)...")
	shutdownTimeout := 10 * time.Second
	err = logger.Shutdown(shutdownTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger shutdown error: %v\n", err)
	} else {
		fmt.Println("Logger shutdown complete.")
	}

	fmt.Printf("Check log files in '%s' and the saved config '%s'.\n", logsDir, configFile)
	fmt.Println("Check stderr output above for potential errors during cleanup.")
}