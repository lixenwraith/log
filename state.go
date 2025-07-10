// FILE: state.go
package log

import (
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lixenwraith/config"
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
	StdoutWriter     atomic.Value // stores io.Writer (os.Stdout, os.Stderr, or io.Discard)

	// Heartbeat statistics
	HeartbeatSequence  atomic.Uint64 // Counter for heartbeat sequence numbers
	LoggerStartTime    atomic.Value  // Stores time.Time for uptime calculation
	TotalLogsProcessed atomic.Uint64 // Counter for non-heartbeat logs successfully processed
	TotalRotations     atomic.Uint64 // Counter for successful log rotations
	TotalDeletions     atomic.Uint64 // Counter for successful log deletions (cleanup/retention)
}

// sink is a wrapper around an io.Writer, atomic value type change workaround
type sink struct {
	w io.Writer
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

	if err := l.updateConfigFromExternal(cfg, basePath); err != nil {
		return err
	}

	return l.applyAndReconfigureLocked()
}

// InitWithDefaults initializes the logger with built-in defaults and optional overrides
func (l *Logger) InitWithDefaults(overrides ...string) error {
	l.initMu.Lock()
	defer l.initMu.Unlock()

	if l.state.LoggerDisabled.Load() {
		return fmtErrorf("logger previously failed to initialize and is disabled")
	}

	for _, override := range overrides {
		key, valueStr, err := parseKeyValue(override)
		if err != nil {
			return err
		}

		keyLower := strings.ToLower(key)
		path := "log." + keyLower

		if _, exists := l.config.Get(path); !exists {
			return fmtErrorf("unknown config key in override: %s", key)
		}

		currentVal, found := l.config.Get(path)
		if !found {
			return fmtErrorf("failed to get current value for '%s'", key)
		}

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

		if err := validateConfigValue(keyLower, parsedValue); err != nil {
			return fmtErrorf("invalid value for '%s': %w", key, err)
		}

		err = l.config.Set(path, parsedValue)
		if err != nil {
			return fmtErrorf("failed to update config value for '%s': %w", key, err)
		}
	}

	return l.applyAndReconfigureLocked()
}

// Shutdown gracefully closes the logger, attempting to flush pending records
// If no timeout is provided, uses a default of 2x flush interval
func (l *Logger) Shutdown(timeout ...time.Duration) error {

	if !l.state.ShutdownCalled.CompareAndSwap(false, true) {
		return nil
	}

	l.state.LoggerDisabled.Store(true)

	if !l.state.IsInitialized.Load() {
		l.state.ShutdownCalled.Store(false)
		l.state.LoggerDisabled.Store(false)
		l.state.ProcessorExited.Store(true)
		return nil
	}

	l.initMu.Lock()
	ch := l.getCurrentLogChannel()
	closedChan := make(chan logRecord)
	close(closedChan)
	l.state.ActiveLogChannel.Store(closedChan)
	if ch != closedChan {
		close(ch)
	}
	l.initMu.Unlock()

	var effectiveTimeout time.Duration
	if len(timeout) > 0 {
		effectiveTimeout = timeout[0]
	} else {
		// Default to 2x flush interval
		flushMs, _ := l.config.Int64("log.flush_interval_ms")
		effectiveTimeout = 2 * time.Duration(flushMs) * time.Millisecond
	}

	deadline := time.Now().Add(effectiveTimeout)
	pollInterval := 10 * time.Millisecond // Reasonable check period
	processorCleanlyExited := false
	for time.Now().Before(deadline) {
		if l.state.ProcessorExited.Load() {
			processorCleanlyExited = true
			break
		}
		time.Sleep(pollInterval)
	}

	l.state.IsInitialized.Store(false)

	var finalErr error
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if currentLogFile, ok := cfPtr.(*os.File); ok && currentLogFile != nil {
			if err := currentLogFile.Sync(); err != nil {
				syncErr := fmtErrorf("failed to sync log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, syncErr)
			}
			if err := currentLogFile.Close(); err != nil {
				closeErr := fmtErrorf("failed to close log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, closeErr)
			}
			l.state.CurrentFile.Store((*os.File)(nil))
		}
	}

	if !processorCleanlyExited {
		timeoutErr := fmtErrorf("logger processor did not exit within timeout (%v)", effectiveTimeout)
		finalErr = combineErrors(finalErr, timeoutErr)
	}

	return finalErr
}

// Flush explicitly triggers a sync of the current log file buffer to disk and waits for completion or timeout.
func (l *Logger) Flush(timeout time.Duration) error {
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

	select {
	case <-confirmChan:
		return nil
	case <-time.After(timeout):
		return fmtErrorf("timeout waiting for flush confirmation (%v)", timeout)
	}
}