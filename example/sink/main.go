// FILE: main.go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/lixenwraith/log"
)

const (
	logDirectory = "./temp_logs"
	logInterval  = 200 * time.Millisecond // Shorter interval for quicker tests
)

// main orchestrates the different test scenarios.
func main() {
	// Ensure a clean state by removing the previous log directory.
	if err := os.RemoveAll(logDirectory); err != nil {
		fmt.Printf("Warning: could not remove old log directory: %v\n", err)
	}
	if err := os.MkdirAll(logDirectory, 0755); err != nil {
		fmt.Printf("Fatal: could not create log directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("--- Running Logger Test Suite ---")
	fmt.Printf("! All file-based logs will be in the '%s' directory.\n\n", logDirectory)

	// --- Scenario 1: Test different configurations on fresh logger instances ---
	fmt.Println("--- SCENARIO 1: Testing configurations in isolation (new logger per test) ---")
	testFileOnly()
	testStdoutOnly()
	testStderrOnly()
	testNoOutput()

	// --- Scenario 2: Test reconfiguration on a single logger instance ---
	fmt.Println("\n--- SCENARIO 2: Testing reconfiguration on a single logger instance ---")
	testReconfigurationTransitions()

	fmt.Println("\n--- Logger Test Suite Complete ---")
	fmt.Printf("Check the '%s' directory for log files.\n", logDirectory)
}

// testFileOnly tests the default behavior: writing only to a file.
func testFileOnly() {
	logger := log.NewLogger()
	runTestPhase(logger, "1.1: File-Only",
		"directory="+logDirectory,
		"name=file_only_log", // Give it a unique name
		"level=-4",
	)
	shutdownLogger(logger, "1.1: File-Only")
}

// testStdoutOnly tests writing only to the standard output.
func testStdoutOnly() {
	logger := log.NewLogger()
	runTestPhase(logger, "1.2: Stdout-Only",
		"enable_stdout=true",
		"disable_file=true", // Explicitly disable file
		"level=-4",
	)
	shutdownLogger(logger, "1.2: Stdout-Only")
}

// testStderrOnly tests writing only to the standard error stream.
func testStderrOnly() {
	fmt.Fprintln(os.Stderr, "\n---") // Separator for stderr output
	logger := log.NewLogger()
	runTestPhase(logger, "1.3: Stderr-Only",
		"enable_stdout=true",
		"stdout_target=stderr",
		"disable_file=true",
		"level=-4",
	)
	fmt.Fprintln(os.Stderr, "---") // Separator for stderr output
	shutdownLogger(logger, "1.3: Stderr-Only")
}

// testNoOutput tests a configuration where all logging is disabled.
func testNoOutput() {
	logger := log.NewLogger()
	runTestPhase(logger, "1.4: No-Output (logs should be dropped)",
		"enable_stdout=false", // Ensure stdout is off
		"disable_file=true",   // Ensure file is off
		"level=-4",
	)
	shutdownLogger(logger, "1.4: No-Output")
}

// testReconfigurationTransitions tests the logger's ability to handle state changes.
func testReconfigurationTransitions() {
	logger := log.NewLogger()

	// Phase A: Start with dual output
	runTestPhase(logger, "2.1: Reconfig - Initial (Dual File+Stdout)",
		"directory="+logDirectory,
		"name=reconfig_log",
		"enable_stdout=true",
		"disable_file=false",
		"level=-4",
	)

	// Phase B: Transition to file-disabled
	runTestPhase(logger, "2.2: Reconfig - Transition to Stdout-Only",
		"enable_stdout=true",
		"disable_file=true", // The key change
		"level=-4",
	)

	// Phase C: Transition back to dual-output. This is the critical test.
	runTestPhase(logger, "2.3: Reconfig - Transition back to Dual (File+Stdout)",
		"directory="+logDirectory, // Re-specify directory
		"name=reconfig_log",
		"enable_stdout=true",
		"disable_file=false", // Re-enable file
		"level=-4",
	)

	// Phase D: Test different levels on the final reconfigured state
	fmt.Println("\n[Phase 2.4: Reconfig - Testing log levels on final state]")
	logger.Debug("final-state", "This is a debug message.")
	logger.Info("final-state", "This is an info message.")
	logger.Warn("final-state", "This is a warning message.")
	logger.Error("final-state", "This is an error message.")
	time.Sleep(logInterval)

	shutdownLogger(logger, "2: Reconfiguration")
}

// runTestPhase is a helper to initialize and run a standard logging test.
func runTestPhase(logger *log.Logger, phaseName string, overrides ...string) {
	fmt.Printf("\n[Phase %s]\n", phaseName)
	fmt.Println("  Config:", overrides)

	err := logger.InitWithDefaults(overrides...)
	if err != nil {
		fmt.Printf("  ERROR: Failed to initialize/reconfigure logger: %v\n", err)
		os.Exit(1)
	}

	logger.Info("event", "start_phase", "name", phaseName)
	time.Sleep(logInterval)
	logger.Info("event", "end_phase", "name", phaseName)
	time.Sleep(logInterval) // Give time for flush
}

// shutdownLogger is a helper to gracefully shut down the logger instance.
func shutdownLogger(l *log.Logger, phaseName string) {
	if err := l.Shutdown(500 * time.Millisecond); err != nil {
		fmt.Printf("  WARNING: Shutdown error in phase '%s': %v\n", phaseName, err)
	}
}