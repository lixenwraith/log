package main

import (
	"fmt"
	"os"
	"time"

	"github.com/LixenWraith/log"
)

func main() {
	// Create test log directory if it doesn't exist
	if err := os.MkdirAll("./logs", 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create test logs directory: %v\n", err)
		os.Exit(1)
	}

	// Test cycle: disable -> PROC -> PROC+DISK -> PROC+DISK+SYS -> PROC+DISK -> PROC -> disable
	levels := []struct {
		level       int64
		description string
	}{
		{0, "Heartbeats disabled"},
		{1, "PROC heartbeats only"},
		{2, "PROC+DISK heartbeats"},
		{3, "PROC+DISK+SYS heartbeats"},
		{2, "PROC+DISK heartbeats (reducing from 3)"},
		{1, "PROC heartbeats only (reducing from 2)"},
		{0, "Heartbeats disabled (final)"},
	}

	// Create a single logger instance that we'll reconfigure
	logger := log.NewLogger()

	for _, levelConfig := range levels {
		// Set up configuration overrides
		overrides := []string{
			"directory=./logs",
			"level=-4",               // Debug level to see everything
			"format=txt",             // Use text format for easier reading
			"heartbeat_interval_s=5", // Short interval for testing
			fmt.Sprintf("heartbeat_level=%d", levelConfig.level),
		}

		// Initialize logger with the new configuration
		// Note: InitWithDefaults handles reconfiguration of an existing logger
		if err := logger.InitWithDefaults(overrides...); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
			os.Exit(1)
		}

		// Log the current test state
		fmt.Printf("\n--- Testing heartbeat level %d: %s ---\n", levelConfig.level, levelConfig.description)
		logger.Info("Heartbeat test started", "level", levelConfig.level, "description", levelConfig.description)

		// Generate some logs to trigger heartbeat counters
		for j := 0; j < 10; j++ {
			logger.Debug("Debug test log", "iteration", j, "level_test", levelConfig.level)
			logger.Info("Info test log", "iteration", j, "level_test", levelConfig.level)
			logger.Warn("Warning test log", "iteration", j, "level_test", levelConfig.level)
			logger.Error("Error test log", "iteration", j, "level_test", levelConfig.level)
			time.Sleep(100 * time.Millisecond)
		}

		// Wait for heartbeats to generate (slightly longer than the interval)
		waitTime := 6 * time.Second
		fmt.Printf("Waiting %v for heartbeats to generate...\n", waitTime)
		time.Sleep(waitTime)

		logger.Info("Heartbeat test completed for level", "level", levelConfig.level)
	}

	// Final shutdown
	if err := logger.Shutdown(2 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to shut down logger: %v\n", err)
	}

	fmt.Println("\nHeartbeat test program completed successfully")
	fmt.Println("Check logs directory for generated log files")
}