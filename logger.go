package log

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LixenWraith/config"
)

// Logger is the core struct that encapsulates all logger functionality
type Logger struct {
	config     *config.Config // Config management
	state      State
	initMu     sync.Mutex  // Only mutex we need to keep
	serializer *serializer // Encapsulated serializer instance
}

// configDefaults holds the default values for logger configuration
var configDefaults = map[string]interface{}{
	"log.level":                    LevelInfo,
	"log.name":                     "log",
	"log.directory":                "./logs",
	"log.format":                   "txt",
	"log.extension":                "log",
	"log.show_timestamp":           true,
	"log.show_level":               true,
	"log.buffer_size":              int64(1024),
	"log.max_size_mb":              int64(10),
	"log.max_total_size_mb":        int64(50),
	"log.min_disk_free_mb":         int64(100),
	"log.flush_interval_ms":        int64(100),
	"log.trace_depth":              int64(0),
	"log.retention_period_hrs":     float64(0.0),
	"log.retention_check_mins":     float64(60.0),
	"log.disk_check_interval_ms":   int64(5000),
	"log.enable_adaptive_interval": true,
	"log.min_check_interval_ms":    int64(100),
	"log.max_check_interval_ms":    int64(60000),
}

// Global instance for package-level functions
var defaultLogger = NewLogger()

// NewLogger creates a new Logger instance with default settings
func NewLogger() *Logger {
	l := &Logger{
		config:     config.New(),
		serializer: newSerializer(),
	}

	// Register all configuration parameters with their defaults
	l.registerConfigValues()

	// Initialize the state
	l.state.IsInitialized.Store(false)
	l.state.LoggerDisabled.Store(false)
	l.state.ShutdownCalled.Store(false)
	l.state.DiskFullLogged.Store(false)
	l.state.DiskStatusOK.Store(true)
	l.state.ProcessorExited.Store(true)
	l.state.CurrentSize.Store(0)
	l.state.EarliestFileTime.Store(time.Time{})

	// Create a closed channel initially to prevent nil pointer issues
	initialChan := make(chan logRecord)
	close(initialChan)
	l.state.ActiveLogChannel.Store(initialChan)

	return l
}

// registerConfigValues registers all configuration parameters with the config instance
func (l *Logger) registerConfigValues() {
	// Register each configuration value with its default
	for path, defaultValue := range configDefaults {
		err := l.config.Register(path, defaultValue)
		if err != nil {
			// If registration fails, we'll handle it gracefully
			fmt.Fprintf(os.Stderr, "log: warning - failed to register config key '%s': %v\n", path, err)
		}
	}
}

// getCurrentLogChannel safely retrieves the current log channel
func (l *Logger) getCurrentLogChannel() chan logRecord {
	chVal := l.state.ActiveLogChannel.Load()
	return chVal.(chan logRecord)
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

// updateConfigFromExternal updates the logger config from an external config.Config instance
func (l *Logger) updateConfigFromExternal(extCfg *config.Config, basePath string) error {
	// For each config key, get value from external config and update local config
	for path := range configDefaults {
		// Extract the local name without the "log." prefix
		localName := strings.TrimPrefix(path, "log.")

		// Create the full path for the external config
		fullPath := localName
		if basePath != "" {
			fullPath = basePath + "." + localName
		}

		// Get current value from our config to use as default in external config
		currentVal, found := l.config.Get(path)
		if !found {
			// Use the original default if not found in current config
			currentVal = configDefaults[path]
		}

		// Register in external config with our current value as the default
		err := extCfg.Register(fullPath, currentVal)
		if err != nil {
			return fmtErrorf("failed to register config key '%s': %w", fullPath, err)
		}

		// Get value from external config
		val, found := extCfg.Get(fullPath)
		if !found {
			continue // Use existing value if not found in external config
		}

		// Validate the value before updating
		if err := validateConfigValue(localName, val); err != nil {
			return fmtErrorf("invalid value for '%s': %w", localName, err)
		}

		// Update our config with the new value
		err = l.config.Set(path, val)
		if err != nil {
			return fmtErrorf("failed to update config value for '%s': %w", path, err)
		}
	}
	return nil
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

// applyAndReconfigureLocked applies the configuration and reconfigures logger components
// Assumes initMu is held
func (l *Logger) applyAndReconfigureLocked() error {
	// Check parameter relationship issues
	minInterval, _ := l.config.Int64("log.min_check_interval_ms")
	maxInterval, _ := l.config.Int64("log.max_check_interval_ms")
	if minInterval > maxInterval {
		fmt.Fprintf(os.Stderr, "log: warning - min_check_interval_ms (%d) > max_check_interval_ms (%d), max will be used\n",
			minInterval, maxInterval)

		// Update min_check_interval_ms to equal max_check_interval_ms
		err := l.config.Set("log.min_check_interval_ms", maxInterval)
		if err != nil {
			fmt.Fprintf(os.Stderr, "log: warning - failed to update min_check_interval_ms: %v\n", err)
		}
	}

	// Ensure log directory exists
	dir, _ := l.config.String("log.directory")
	if err := os.MkdirAll(dir, 0755); err != nil {
		l.state.LoggerDisabled.Store(true)
		return fmtErrorf("failed to create log directory '%s': %w", dir, err)
	}

	// Check if we need to restart the processor
	wasInitialized := l.state.IsInitialized.Load()
	processorNeedsRestart := !wasInitialized

	// Always restart the processor if initialized, to handle any config changes
	// This is the simplest approach that works reliably for all config changes
	if wasInitialized {
		processorNeedsRestart = true
	}

	// Restart processor if needed
	if processorNeedsRestart {
		// Close the old channel if reconfiguring
		if wasInitialized {
			oldCh := l.getCurrentLogChannel()
			if oldCh != nil {
				// Swap in a temporary closed channel
				tempClosedChan := make(chan logRecord)
				close(tempClosedChan)
				l.state.ActiveLogChannel.Store(tempClosedChan)

				// Close the actual old channel
				close(oldCh)
			}
		}

		// Create the new channel
		bufferSize, _ := l.config.Int64("log.buffer_size")
		newLogChannel := make(chan logRecord, bufferSize)
		l.state.ActiveLogChannel.Store(newLogChannel)

		// Start the new processor
		l.state.ProcessorExited.Store(false)
		go l.processLogs(newLogChannel)
	}

	// Initialize new log file if needed
	currentFileHandle := l.state.CurrentFile.Load()
	needsNewFile := !wasInitialized || currentFileHandle == nil

	if needsNewFile {
		logFile, err := l.createNewLogFile()
		if err != nil {
			l.state.LoggerDisabled.Store(true)
			return fmtErrorf("failed to create initial/new log file: %w", err)
		}
		l.state.CurrentFile.Store(logFile)
		l.state.CurrentSize.Store(0)
		if fi, errStat := logFile.Stat(); errStat == nil {
			l.state.CurrentSize.Store(fi.Size())
		}
	}

	// Mark as initialized
	l.state.IsInitialized.Store(true)
	l.state.ShutdownCalled.Store(false)
	l.state.DiskFullLogged.Store(false)
	l.state.DiskStatusOK.Store(true)

	return nil
}

// Default package-level functions that delegate to the default logger

// Init initializes or reconfigures the logger using the provided config.Config instance
func Init(cfg *config.Config, basePath string) error {
	return defaultLogger.Init(cfg, basePath)
}

// InitWithDefaults initializes the logger with built-in defaults and optional overrides
func InitWithDefaults(overrides ...string) error {
	return defaultLogger.InitWithDefaults(overrides...)
}

// Shutdown gracefully closes the logger, attempting to flush pending records
func Shutdown(timeout time.Duration) error {
	return defaultLogger.Shutdown(timeout)
}

// Debug logs a message at debug level
func Debug(args ...any) {
	defaultLogger.Debug(args...)
}

// Info logs a message at info level
func Info(args ...any) {
	defaultLogger.Info(args...)
}

// Warn logs a message at warning level
func Warn(args ...any) {
	defaultLogger.Warn(args...)
}

// Error logs a message at error level
func Error(args ...any) {
	defaultLogger.Error(args...)
}

// DebugTrace logs a debug message with function call trace
func DebugTrace(depth int, args ...any) {
	defaultLogger.DebugTrace(depth, args...)
}

// InfoTrace logs an info message with function call trace
func InfoTrace(depth int, args ...any) {
	defaultLogger.InfoTrace(depth, args...)
}

// WarnTrace logs a warning message with function call trace
func WarnTrace(depth int, args ...any) {
	defaultLogger.WarnTrace(depth, args...)
}

// ErrorTrace logs an error message with function call trace
func ErrorTrace(depth int, args ...any) {
	defaultLogger.ErrorTrace(depth, args...)
}

// Log writes a timestamp-only record without level information
func Log(args ...any) {
	defaultLogger.Log(args...)
}

// Message writes a plain record without timestamp or level info
func Message(args ...any) {
	defaultLogger.Message(args...)
}

// LogTrace writes a timestamp record with call trace but no level info
func LogTrace(depth int, args ...any) {
	defaultLogger.LogTrace(depth, args...)
}

// SaveConfig saves the current logger configuration to a file
func SaveConfig(path string) error {
	return defaultLogger.SaveConfig(path)
}

// LoadConfig loads logger configuration from a file with optional CLI overrides
func LoadConfig(path string, args []string) error {
	return defaultLogger.LoadConfig(path, args)
}

// SaveConfig saves the current logger configuration to a file
func (l *Logger) SaveConfig(path string) error {
	return l.config.Save(path)
}

// LoadConfig loads logger configuration from a file with optional CLI overrides
func (l *Logger) LoadConfig(path string, args []string) error {
	configExists, err := l.config.Load(path, args)
	if err != nil {
		return err
	}

	// If no config file exists and no CLI args were provided, there's nothing to apply
	if !configExists && len(args) == 0 {
		return nil
	}

	l.initMu.Lock()
	defer l.initMu.Unlock()
	return l.applyAndReconfigureLocked()
}

// Helper functions
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
	if ch != closedChan { // Avoid closing the dummy channel itself
		close(ch)
	}
	l.initMu.Unlock()

	// Determine the maximum time to wait for the processor to finish
	effectiveTimeout := timeout
	if effectiveTimeout <= 0 {
		// Use the configured flush interval as the default timeout if none provided
		flushMs, _ := l.config.Int64("log.flush_interval_ms")
		effectiveTimeout = time.Duration(flushMs) * time.Millisecond
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
				finalErr = fmtErrorf("failed to sync log file '%s' during shutdown: %w", currentLogFile.Name(), err)
			}
			// Attempt to close the file descriptor
			if err := currentLogFile.Close(); err != nil {
				closeErr := fmtErrorf("failed to close log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, closeErr) // Combine sync/close errors
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

// Logger instance methods for logging at different levels

// Debug logs a message at debug level.
func (l *Logger) Debug(args ...any) {
	flags := l.getFlags()
	traceDepth, _ := l.config.Int64("log.trace_depth")
	l.log(flags, LevelDebug, traceDepth, args...)
}

// Info logs a message at info level.
func (l *Logger) Info(args ...any) {
	flags := l.getFlags()
	traceDepth, _ := l.config.Int64("log.trace_depth")
	l.log(flags, LevelInfo, traceDepth, args...)
}

// Warn logs a message at warning level.
func (l *Logger) Warn(args ...any) {
	flags := l.getFlags()
	traceDepth, _ := l.config.Int64("log.trace_depth")
	l.log(flags, LevelWarn, traceDepth, args...)
}

// Error logs a message at error level.
func (l *Logger) Error(args ...any) {
	flags := l.getFlags()
	traceDepth, _ := l.config.Int64("log.trace_depth")
	l.log(flags, LevelError, traceDepth, args...)
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

// Helper method to get flags from config
func (l *Logger) getFlags() int64 {
	var flags int64 = 0
	showLevel, _ := l.config.Bool("log.show_level")
	showTimestamp, _ := l.config.Bool("log.show_timestamp")

	if showLevel {
		flags |= FlagShowLevel
	}
	if showTimestamp {
		flags |= FlagShowTimestamp
	}
	return flags
}

// log handles the core logging logic
func (l *Logger) log(flags int64, level int64, depth int64, args ...any) {
	// Quick checks first
	if l.state.LoggerDisabled.Load() || !l.state.IsInitialized.Load() {
		return
	}

	// Check if this log level should be processed
	configLevel, _ := l.config.Int64("log.level")
	if level < configLevel {
		return
	}

	// Report dropped logs if necessary
	currentDrops := l.state.DroppedLogs.Load()
	logged := l.state.LoggedDrops.Load()
	if currentDrops > logged {
		if l.state.LoggedDrops.CompareAndSwap(logged, currentDrops) {
			dropRecord := logRecord{
				Flags:     FlagDefault, // Use default flags for drop message
				TimeStamp: time.Now(),
				Level:     LevelError,
				Args:      []any{"Logs were dropped", "dropped_count", currentDrops - logged, "total_dropped", currentDrops},
			}
			l.sendLogRecord(dropRecord) // Best effort send
		}
	}

	// Get trace if needed
	var trace string
	if depth > 0 {
		const skipTrace = 3 // log.Info -> logInternal -> getTrace (Adjust if call stack changes)
		trace = getTrace(depth, skipTrace)
	}

	// Create record and send
	record := logRecord{
		Flags:     flags,
		TimeStamp: time.Now(),
		Level:     level,
		Trace:     trace,
		Args:      args,
	}
	l.sendLogRecord(record)
}

// sendLogRecord handles safe sending to the active channel
func (l *Logger) sendLogRecord(record logRecord) {
	defer func() {
		if recover() != nil { // Catch panic on send to closed channel
			l.state.DroppedLogs.Add(1)
		}
	}()

	if l.state.ShutdownCalled.Load() || l.state.LoggerDisabled.Load() {
		l.state.DroppedLogs.Add(1)
		return
	}

	// Load current channel reference atomically
	ch := l.getCurrentLogChannel()

	// Non-blocking send
	select {
	case ch <- record:
		// Success
	default:
		// Channel buffer is full or channel is closed
		l.state.DroppedLogs.Add(1)
	}
}