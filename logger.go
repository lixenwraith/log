// FILE: lixenwraith/log/logger.go
package log

import (
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Logger is the core struct that encapsulates all logger functionality
type Logger struct {
	currentConfig atomic.Value // stores *Config
	state         State
	initMu        sync.Mutex
	serializer    *serializer
}

// NewLogger creates a new Logger instance with default settings
func NewLogger() *Logger {
	l := &Logger{
		serializer: newSerializer(),
	}

	// Set default configuration
	l.currentConfig.Store(DefaultConfig())

	// Initialize the state
	l.state.IsInitialized.Store(false)
	l.state.LoggerDisabled.Store(false)
	l.state.ShutdownCalled.Store(false)
	l.state.DiskFullLogged.Store(false)
	l.state.DiskStatusOK.Store(true)
	l.state.ProcessorExited.Store(true)
	l.state.CurrentSize.Store(0)
	l.state.EarliestFileTime.Store(time.Time{})

	// Initialize heartbeat counters
	l.state.HeartbeatSequence.Store(0)
	l.state.LoggerStartTime.Store(time.Now())
	l.state.TotalLogsProcessed.Store(0)
	l.state.TotalRotations.Store(0)
	l.state.TotalDeletions.Store(0)

	// Create a closed channel initially to prevent nil pointer issues
	initialChan := make(chan logRecord)
	close(initialChan)
	l.state.ActiveLogChannel.Store(initialChan)

	l.state.flushRequestChan = make(chan chan struct{}, 1)

	return l
}

// ApplyConfig applies a validated configuration to the logger
// This is the primary way applications should configure the logger
func (l *Logger) ApplyConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("log: configuration cannot be nil")
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("log: invalid configuration: %w", err)
	}

	l.initMu.Lock()
	defer l.initMu.Unlock()

	return l.applyConfig(cfg)
}

// ApplyConfigString applies string key-value overrides to the logger's current configuration.
// Each override should be in the format "key=value".
func (l *Logger) ApplyConfigString(overrides ...string) error {
	cfg := l.getConfig().Clone()

	var errors []error

	for _, override := range overrides {
		key, value, err := parseKeyValue(override)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		if err := applyConfigField(cfg, key, value); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return combineConfigErrors(errors)
	}

	return l.ApplyConfig(cfg)
}

// GetConfig returns a copy of current configuration
func (l *Logger) GetConfig() *Config {
	return l.getConfig().Clone()
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

	c := l.getConfig()
	var effectiveTimeout time.Duration
	if len(timeout) > 0 {
		effectiveTimeout = timeout[0]
	} else {
		flushIntervalMs := c.FlushIntervalMs
		// Default to 2x flush interval
		effectiveTimeout = 2 * time.Duration(flushIntervalMs) * time.Millisecond
	}

	deadline := time.Now().Add(effectiveTimeout)
	pollInterval := minWaitTime // Reasonable check period
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
	case <-time.After(minWaitTime): // Short timeout to prevent blocking if processor is stuck
		return fmtErrorf("failed to send flush request to processor (possible deadlock or high load)")
	}

	select {
	case <-confirmChan:
		return nil
	case <-time.After(timeout):
		return fmtErrorf("timeout waiting for flush confirmation (%v)", timeout)
	}
}

// Debug logs a message at debug level.
func (l *Logger) Debug(args ...any) {
	flags := l.getFlags()
	cfg := l.getConfig()
	l.log(flags, LevelDebug, cfg.TraceDepth, args...)
}

// Info logs a message at info level.
func (l *Logger) Info(args ...any) {
	flags := l.getFlags()
	cfg := l.getConfig()
	l.log(flags, LevelInfo, cfg.TraceDepth, args...)
}

// Warn logs a message at warning level.
func (l *Logger) Warn(args ...any) {
	flags := l.getFlags()
	cfg := l.getConfig()
	l.log(flags, LevelWarn, cfg.TraceDepth, args...)
}

// Error logs a message at error level.
func (l *Logger) Error(args ...any) {
	flags := l.getFlags()
	cfg := l.getConfig()
	l.log(flags, LevelError, cfg.TraceDepth, args...)
}

// DebugTrace logs a debug message with function call trace.
func (l *Logger) DebugTrace(depth int, args ...any) {
	flags := l.getFlags()
	l.log(flags, LevelDebug, int64(depth), args...)
}

// InfoTrace logs an info message with function call trace.
func (l *Logger) InfoTrace(depth int, args ...any) {
	flags := l.getFlags()
	l.log(flags, LevelInfo, int64(depth), args...)
}

// WarnTrace logs a warning message with function call trace.
func (l *Logger) WarnTrace(depth int, args ...any) {
	flags := l.getFlags()
	l.log(flags, LevelWarn, int64(depth), args...)
}

// ErrorTrace logs an error message with function call trace.
func (l *Logger) ErrorTrace(depth int, args ...any) {
	flags := l.getFlags()
	l.log(flags, LevelError, int64(depth), args...)
}

// Log writes a timestamp-only record without level information.
func (l *Logger) Log(args ...any) {
	l.log(FlagShowTimestamp, LevelInfo, 0, args...)
}

// Message writes a plain record without timestamp or level info.
func (l *Logger) Message(args ...any) {
	l.log(0, LevelInfo, 0, args...)
}

// LogTrace writes a timestamp record with call trace but no level info.
func (l *Logger) LogTrace(depth int, args ...any) {
	l.log(FlagShowTimestamp, LevelInfo, int64(depth), args...)
}

// LogStructured logs a message with structured fields as proper JSON
func (l *Logger) LogStructured(level int64, message string, fields map[string]any) {
	l.log(l.getFlags()|FlagStructuredJSON, level, 0, []any{message, fields})
}

// Write outputs raw, unformatted data regardless of configured format.
// Writes args as space-separated strings without a trailing newline.
func (l *Logger) Write(args ...any) {
	l.log(FlagRaw, LevelInfo, 0, args...)
}

// getConfig returns the current configuration (thread-safe)
func (l *Logger) getConfig() *Config {
	return l.currentConfig.Load().(*Config)
}

// apply applies a validated configuration and reconfigures logger components
// Assumes initMu is held
func (l *Logger) applyConfig(cfg *Config) error {
	oldCfg := l.getConfig()
	l.currentConfig.Store(cfg)

	l.serializer.setTimestampFormat(cfg.TimestampFormat)

	// Ensure log directory exists
	if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
		l.state.LoggerDisabled.Store(true)
		l.currentConfig.Store(oldCfg) // Rollback
		return fmtErrorf("failed to create log directory '%s': %w", cfg.Directory, err)
	}

	// Get current state
	wasInitialized := l.state.IsInitialized.Load()

	// Get current file handle
	currentFilePtr := l.state.CurrentFile.Load()
	var currentFile *os.File
	if currentFilePtr != nil {
		currentFile, _ = currentFilePtr.(*os.File)
	}

	// Determine if we need a new file
	needsNewFile := !wasInitialized || currentFile == nil

	// Handle file state transitions
	if cfg.DisableFile {
		// When disabling file output, close the current file
		if currentFile != nil {
			// Sync and close the file
			_ = currentFile.Sync()
			if err := currentFile.Close(); err != nil {
				l.internalLog("warning - failed to close log file during disable: %v\n", err)
			}
		}
		l.state.CurrentFile.Store((*os.File)(nil))
		l.state.CurrentSize.Store(0)
	} else if needsNewFile {
		// When enabling file output or initializing, create new file
		logFile, err := l.createNewLogFile()
		if err != nil {
			l.state.LoggerDisabled.Store(true)
			l.currentConfig.Store(oldCfg) // Rollback
			return fmtErrorf("failed to create log file: %w", err)
		}

		// Close old file if transitioning from one file to another
		if currentFile != nil && currentFile != logFile {
			_ = currentFile.Sync()
			if err := currentFile.Close(); err != nil {
				l.internalLog("warning - failed to close old log file: %v\n", err)
			}
		}

		l.state.CurrentFile.Store(logFile)
		l.state.CurrentSize.Store(0)
		if fi, errStat := logFile.Stat(); errStat == nil {
			l.state.CurrentSize.Store(fi.Size())
		}
	}

	// Close the old channel if reconfiguring
	if wasInitialized {
		oldCh := l.getCurrentLogChannel()
		if oldCh != nil {
			// Create new channel then close old channel
			newLogChannel := make(chan logRecord, cfg.BufferSize)
			l.state.ActiveLogChannel.Store(newLogChannel)
			close(oldCh)

			// Start new processor with new channel
			l.state.ProcessorExited.Store(false)
			go l.processLogs(newLogChannel)
		}
	} else {
		// Initial startup
		newLogChannel := make(chan logRecord, cfg.BufferSize)
		l.state.ActiveLogChannel.Store(newLogChannel)
		l.state.ProcessorExited.Store(false)
		go l.processLogs(newLogChannel)
	}

	// Setup stdout writer based on config
	if cfg.EnableStdout {
		var writer io.Writer
		if cfg.StdoutTarget == "stderr" {
			writer = os.Stderr
		} else {
			writer = os.Stdout
		}
		l.state.StdoutWriter.Store(&sink{w: writer})
	} else {
		l.state.StdoutWriter.Store(&sink{w: io.Discard})
	}

	// Mark as initialized
	l.state.IsInitialized.Store(true)
	l.state.ShutdownCalled.Store(false)
	l.state.DiskFullLogged.Store(false)
	l.state.DiskStatusOK.Store(true)

	return nil
}