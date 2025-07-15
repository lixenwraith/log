// FILE: logger.go
package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lixenwraith/config"
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

// getConfig returns the current configuration (thread-safe)
func (l *Logger) getConfig() *Config {
	return l.currentConfig.Load().(*Config)
}

// LoadConfig loads logger configuration from a file
func (l *Logger) LoadConfig(path string) error {
	cfg, err := NewConfigFromFile(path)
	if err != nil {
		return err
	}

	l.initMu.Lock()
	defer l.initMu.Unlock()

	return l.apply(cfg)
}

// SaveConfig saves the current logger configuration to a file
func (l *Logger) SaveConfig(path string) error {
	// Create a lixenwraith/config instance for saving
	saver := config.New()
	cfg := l.getConfig()

	// Register all fields with their current values
	if err := saver.RegisterStruct("log.", *cfg); err != nil {
		return fmt.Errorf("failed to register config for saving: %w", err)
	}

	return saver.Save(path)
}

// apply applies a validated configuration and reconfigures logger components
// Assumes initMu is held
func (l *Logger) apply(cfg *Config) error {
	// Store the new configuration
	oldCfg := l.getConfig()
	l.currentConfig.Store(cfg)

	// Update serializer format
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

// getCurrentLogChannel safely retrieves the current log channel
func (l *Logger) getCurrentLogChannel() chan logRecord {
	chVal := l.state.ActiveLogChannel.Load()
	return chVal.(chan logRecord)
}

// getFlags from config
func (l *Logger) getFlags() int64 {
	var flags int64 = 0
	cfg := l.getConfig()

	if cfg.ShowLevel {
		flags |= FlagShowLevel
	}
	if cfg.ShowTimestamp {
		flags |= FlagShowTimestamp
	}
	return flags
}

// log handles the core logging logic
func (l *Logger) log(flags int64, level int64, depth int64, args ...any) {
	if !l.state.IsInitialized.Load() {
		return
	}

	cfg := l.getConfig()
	if level < cfg.Level {
		return
	}

	var trace string
	if depth > 0 {
		const skipTrace = 3 // log.Info -> log -> getTrace (Adjust if call stack changes)
		trace = getTrace(depth, skipTrace)
	}

	record := logRecord{
		Flags:           flags,
		TimeStamp:       time.Now(),
		Level:           level,
		Trace:           trace,
		Args:            args,
		unreportedDrops: 0, // 0 for regular logs
	}
	l.sendLogRecord(record)
}

// sendLogRecord handles safe sending to the active channel
func (l *Logger) sendLogRecord(record logRecord) {
	defer func() {
		if r := recover(); r != nil { // Catch panic on send to closed channel
			l.handleFailedSend(record)
		}
	}()

	if l.state.ShutdownCalled.Load() || l.state.LoggerDisabled.Load() {
		// Process drops even if logger is disabled or shutting down
		l.handleFailedSend(record)
		return
	}

	ch := l.getCurrentLogChannel()

	// Non-blocking send
	select {
	case ch <- record:
		// Success: record sent, channel was not full, check if log drops need to be reported
		if record.unreportedDrops == 0 {
			// Get number of dropped logs and reset the counter to zero
			droppedCount := l.state.DroppedLogs.Swap(0)

			if droppedCount > 0 {
				// Dropped logs report
				dropRecord := logRecord{
					Flags:           FlagDefault,
					TimeStamp:       time.Now(),
					Level:           LevelError,
					Args:            []any{"Logs were dropped", "dropped_count", droppedCount},
					unreportedDrops: droppedCount, // Carry the count for recovery
				}
				// No success check is required, count is restored if it fails
				l.sendLogRecord(dropRecord)
			}
		}
	default:
		l.handleFailedSend(record)
	}
}

// handleFailedSend restores or increments drop counter
func (l *Logger) handleFailedSend(record logRecord) {
	// If the record was a drop report, add its carried count back.
	// Otherwise, it was a regular log, so add 1.
	amountToAdd := uint64(1)
	if record.unreportedDrops > 0 {
		amountToAdd = record.unreportedDrops
	}
	l.state.DroppedLogs.Add(amountToAdd)
}

// internalLog handles writing internal logger diagnostics to stderr, if enabled.
func (l *Logger) internalLog(format string, args ...any) {
	// Check if internal error reporting is enabled
	cfg := l.getConfig()
	if !cfg.InternalErrorsToStderr {
		return
	}

	// Ensure consistent "log: " prefix
	if !strings.HasPrefix(format, "log: ") {
		format = "log: " + format
	}

	// Write to stderr
	fmt.Fprintf(os.Stderr, format, args...)
}