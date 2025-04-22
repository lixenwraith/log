package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/LixenWraith/config"
	"github.com/LixenWraith/log"
)

const configFile = "simple_config.toml"
const configBasePath = "logging" // Base path for log settings in config

// Example TOML content
var tomlContent = `
# Example simple_config.toml
[logging]
  level = -4 # Debug
  directory = "./simple_logs"
  format = "txt"
  extension = "log"
  show_timestamp = true
  show_level = true
  buffer_size = 1024
  flush_interval_ms = 100
  trace_depth = 0
  retention_period_hrs = 0.0
  retention_check_mins = 60.0
  # Other settings use defaults registered by log.Init
`

func main() {
	fmt.Println("--- Simple Logger Example ---")

	// --- Setup Config ---
	// Create dummy config file
	err := os.WriteFile(configFile, []byte(tomlContent), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write dummy config: %v\n", err)
		// Continue with defaults potentially
	} else {
		fmt.Printf("Created dummy config file: %s\n", configFile)
		// defer os.Remove(configFile) // Remove to keep the saved config file
		// defer os.RemoveAll(logsDir) // Remove to keep the log directory
	}

	// Initialize the external config manager
	cfg := config.New()

	// Load config from file (and potentially CLI args - none provided here)
	// The log package will register its keys during Init
	_, err = cfg.Load(configFile, nil) // os.Args[1:] could be used here
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v. Using defaults.\n", err)
		// Proceeding, log.Init will use registered defaults
	}

	// --- Initialize Logger ---
	// Pass the config instance and the base path for logger settings
	err = log.Init(cfg, configBasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Logger initialized.")

	// --- SAVE CONFIGURATION ---
	// Save the config state *after* log.Init has registered its keys/defaults
	// This will write the merged configuration (defaults + file overrides) back.
	err = cfg.Save(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save configuration to '%s': %v\n", configFile, err)
	} else {
		fmt.Printf("Configuration saved to: %s\n", configFile)
	}
	// --- End Save Configuration ---

	// --- Logging ---
	log.Debug("This is a debug message.", "user_id", 123)
	log.Info("Application starting...")
	log.Warn("Potential issue detected.", "threshold", 0.95)
	log.Error("An error occurred!", "code", 500)

	// Logging from goroutines
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			log.Info("Goroutine started", "id", id)
			time.Sleep(time.Duration(50+id*50) * time.Millisecond)
			log.InfoTrace(1, "Goroutine finished", "id", id) // Log with trace
		}(i)
	}

	// Wait for goroutines to finish before shutting down logger
	wg.Wait()
	fmt.Println("Goroutines finished.")

	// --- Shutdown Logger ---
	fmt.Println("Shutting down logger...")
	// Provide a reasonable timeout for logs to flush
	shutdownTimeout := 2 * time.Second
	err = log.Shutdown(shutdownTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger shutdown error: %v\n", err)
	} else {
		fmt.Println("Logger shutdown complete.")
	}

	// NO time.Sleep needed here - log.Shutdown waits.
	fmt.Println("--- Example Finished ---")
	fmt.Printf("Check log files in './simple_logs' and the saved config '%s'.\n", configFile)
}