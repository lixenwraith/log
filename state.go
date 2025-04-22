// --- File: state.go ---
package log

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/LixenWraith/config"
)

// State encapsulates the runtime state of the logger
type State struct {
	IsInitialized   atomic.Bool
	LoggerDisabled  atomic.Bool
	ShutdownCalled  atomic.Bool
	DiskFullLogged  atomic.Bool
	DiskStatusOK    atomic.Bool
	ProcessorExited atomic.Bool // Tracks if the processor goroutine is running or has exited

	flushRequestChan chan chan struct{} // Channel to request a flush
	flushMutex       sync.Mutex         // Protect concurrent Flush calls

	CurrentFile      atomic.Value  // stores *os.File
	CurrentSize      atomic.Int64  // Size of the current log file
	EarliestFileTime atomic.Value  // stores time.Time for retention
	DroppedLogs      atomic.Uint64 // Counter for logs dropped
	LoggedDrops      atomic.Uint64 // Counter for dropped logs message already logged

	ActiveLogChannel atomic.Value // stores chan logRecord
}

// Init initializes or reconfigures the logger using the provided config.Config instance
func (l *Logger) Init(cfg *config.Config, basePath string) error {
	if cfg == nil {
		l.state.LoggerDisabled.Store(true)
		return fmtErrorf("config instance cannot be nil")
	}

	l.initMu.Lock()
	defer l.initMu.Unlock()

	if l.state.LoggerDisabled.Load() {
		return fmtErrorf("logger previously failed to initialize and is disabled")
	}

	// Update configuration from external config
	if err := l.updateConfigFromExternal(cfg, basePath); err != nil {
		return err
	}

	// Apply configuration and reconfigure logger components
	return l.applyAndReconfigureLocked()
}

// InitWithDefaults initializes the logger with built-in defaults and optional overrides
func (l *Logger) InitWithDefaults(overrides ...string) error {
	l.initMu.Lock()
	defer l.initMu.Unlock()

	if l.state.LoggerDisabled.Load() {
		return fmtErrorf("logger previously failed to initialize and is disabled")
	}

	// Apply provided overrides
	for _, override := range overrides {
		key, valueStr, err := parseKeyValue(override)
		if err != nil {
			return err
		}

		keyLower := strings.ToLower(key)
		path := "log." + keyLower

		// Check if this is a valid config key
		if _, exists := l.config.Get(path); !exists {
			return fmtErrorf("unknown config key in override: %s", key)
		}

		// Get current value to determine type for parsing
		currentVal, found := l.config.Get(path)
		if !found {
			return fmtErrorf("failed to get current value for '%s'", key)
		}

		// Parse according to type
		var parsedValue interface{}
		var parseErr error

		switch currentVal.(type) {
		case int64:
			parsedValue, parseErr = strconv.ParseInt(valueStr, 10, 64)
		case string:
			parsedValue = valueStr
		case bool:
			parsedValue, parseErr = strconv.ParseBool(valueStr)
		case float64:
			parsedValue, parseErr = strconv.ParseFloat(valueStr, 64)
		default:
			return fmtErrorf("unsupported type for key '%s'", key)
		}

		if parseErr != nil {
			return fmtErrorf("invalid value format for '%s': %w", key, parseErr)
		}

		// Validate the parsed value
		if err := validateConfigValue(keyLower, parsedValue); err != nil {
			return fmtErrorf("invalid value for '%s': %w", key, err)
		}

		// Update config with new value
		err = l.config.Set(path, parsedValue)
		if err != nil {
			return fmtErrorf("failed to update config value for '%s': %w", key, err)
		}
	}

	// Apply configuration and reconfigure logger components
	return l.applyAndReconfigureLocked()
}

// Shutdown gracefully closes the logger, attempting to flush pending records
func (l *Logger) Shutdown(timeout time.Duration) error {
	// Ensure shutdown runs only once
	if !l.state.ShutdownCalled.CompareAndSwap(false, true) {
		return nil
	}

	// Prevent new logs from being processed or sent
	l.state.LoggerDisabled.Store(true)

	// If the logger was never initialized, there's nothing to shut down
	if !l.state.IsInitialized.Load() {
		l.state.ShutdownCalled.Store(false) // Allow potential future init/shutdown cycle
		l.state.LoggerDisabled.Store(false)
		l.state.ProcessorExited.Store(true) // Mark as not running
		return nil
	}

	// Signal the processor goroutine to stop by closing its channel
	l.initMu.Lock()
	ch := l.getCurrentLogChannel()
	closedChan := make(chan logRecord) // Create a dummy closed channel
	close(closedChan)
	l.state.ActiveLogChannel.Store(closedChan) // Point producers to the dummy channel
	// Close the actual channel the processor is reading from
	if ch != closedChan {
		close(ch)
	}
	l.initMu.Unlock()

	// Determine the maximum time to wait for the processor to finish
	effectiveTimeout := timeout
	if effectiveTimeout <= 0 {
		// Use the configured flush interval as the default timeout if none provided
		flushMs, _ := l.config.Int64("log.flush_interval_ms")
		effectiveTimeout = 2 * time.Duration(flushMs) * time.Millisecond
	}

	// Wait for the processor goroutine to signal its exit, or until the timeout
	deadline := time.Now().Add(effectiveTimeout)
	pollInterval := 10 * time.Millisecond // Check status periodically
	processorCleanlyExited := false
	for time.Now().Before(deadline) {
		if l.state.ProcessorExited.Load() {
			processorCleanlyExited = true
			break // Processor finished cleanly
		}
		time.Sleep(pollInterval)
	}

	// Mark the logger as uninitialized
	l.state.IsInitialized.Store(false)

	// Sync and close the current log file
	var finalErr error
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if currentLogFile, ok := cfPtr.(*os.File); ok && currentLogFile != nil {
			// Attempt to sync data to disk
			if err := currentLogFile.Sync(); err != nil {
				syncErr := fmtErrorf("failed to sync log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, syncErr)
			}
			// Attempt to close the file descriptor
			if err := currentLogFile.Close(); err != nil {
				closeErr := fmtErrorf("failed to close log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, closeErr)
			}
			// Clear the atomic reference to the file
			l.state.CurrentFile.Store((*os.File)(nil))
		}
	}

	// Report timeout error if processor didn't exit cleanly
	if !processorCleanlyExited {
		timeoutErr := fmtErrorf("logger processor did not exit within timeout (%v)", effectiveTimeout)
		finalErr = combineErrors(finalErr, timeoutErr)
	}

	return finalErr
}

// Flush explicitly triggers a sync of the current log file buffer to disk and waits for completion or timeout.
func (l *Logger) Flush(timeout time.Duration) error {
	// Prevent concurrent flushes overwhelming the processor or channel
	l.state.flushMutex.Lock()
	defer l.state.flushMutex.Unlock()

	if !l.state.IsInitialized.Load() || l.state.ShutdownCalled.Load() {
		return fmtErrorf("logger not initialized or already shut down")
	}

	// Create a channel to wait for confirmation from the processor
	confirmChan := make(chan struct{})

	// Send the request with the confirmation channel
	select {
	case l.state.flushRequestChan <- confirmChan:
		// Request sent
	case <-time.After(10 * time.Millisecond): // Short timeout to prevent blocking if processor is stuck
		return fmtErrorf("failed to send flush request to processor (possible deadlock or high load)")
	}

	// Wait for the processor to signal completion or timeout
	select {
	case <-confirmChan:
		return nil // Flush completed successfully
	case <-time.After(timeout):
		return fmtErrorf("timeout waiting for flush confirmation (%v)", timeout)
	}
}