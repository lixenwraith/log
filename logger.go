// FILE: logger.go
package log

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lixenwraith/config"
)

// Logger is the core struct that encapsulates all logger functionality
type Logger struct {
	config     *config.Config
	state      State
	initMu     sync.Mutex
	serializer *serializer
}

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

// LoadConfig loads logger configuration from a file with optional CLI overrides
func (l *Logger) LoadConfig(path string, args []string) error {
	err := l.config.Load(path, args)

	// Check if the error indicates that the file was not found
	configExists := !errors.Is(err, config.ErrConfigNotFound)

	// If there's an error other than "file not found", return it
	if err != nil && !errors.Is(err, config.ErrConfigNotFound) {
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

// SaveConfig saves the current logger configuration to a file
func (l *Logger) SaveConfig(path string) error {
	return l.config.Save(path)
}

// registerConfigValues registers all configuration parameters with the config instance
func (l *Logger) registerConfigValues() {
	// Register the entire config struct at once
	err := l.config.RegisterStruct("log.", defaultConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: warning - failed to register config values: %v\n", err)
	}
}

// updateConfigFromExternal updates the logger config from an external config.Config instance
func (l *Logger) updateConfigFromExternal(extCfg *config.Config, basePath string) error {
	// Get our registered config paths (already registered during initialization)
	registeredPaths := l.config.GetRegisteredPaths("log.")
	if len(registeredPaths) == 0 {
		// Register defaults first if not already done
		l.registerConfigValues()
		registeredPaths = l.config.GetRegisteredPaths("log.")
	}

	// For each registered path
	for path := range registeredPaths {
		// Extract local name and build external path
		localName := strings.TrimPrefix(path, "log.")
		fullPath := basePath + "." + localName
		if basePath == "" {
			fullPath = localName
		}

		// Get current value to use as default in external config
		currentVal, found := l.config.Get(path)
		if !found {
			continue // Skip if not found (shouldn't happen)
		}

		// Register in external config with current value as default
		err := extCfg.Register(fullPath, currentVal)
		if err != nil {
			return fmtErrorf("failed to register config key '%s': %w", fullPath, err)
		}

		// Get value from external config
		val, found := extCfg.Get(fullPath)
		if !found {
			continue // Use existing value if not found in external config
		}

		// Validate and update
		if err := validateConfigValue(localName, val); err != nil {
			return fmtErrorf("invalid value for '%s': %w", localName, err)
		}

		if err := l.config.Set(path, val); err != nil {
			return fmtErrorf("failed to update config value for '%s': %w", path, err)
		}
	}
	return nil
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

	// Validate config (Basic)
	currentCfg := l.loadCurrentConfig() // Helper to load struct from l.config
	if err := currentCfg.validate(); err != nil {
		l.state.LoggerDisabled.Store(true) // Disable logger on validation failure
		return fmtErrorf("invalid configuration detected: %w", err)
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

// loadCurrentConfig loads the current configuration for validation
func (l *Logger) loadCurrentConfig() *Config {
	cfg := &Config{}
	cfg.Level, _ = l.config.Int64("log.level")
	cfg.Name, _ = l.config.String("log.name")
	cfg.Directory, _ = l.config.String("log.directory")
	cfg.Format, _ = l.config.String("log.format")
	cfg.Extension, _ = l.config.String("log.extension")
	cfg.ShowTimestamp, _ = l.config.Bool("log.show_timestamp")
	cfg.ShowLevel, _ = l.config.Bool("log.show_level")
	cfg.BufferSize, _ = l.config.Int64("log.buffer_size")
	cfg.MaxSizeMB, _ = l.config.Int64("log.max_size_mb")
	cfg.MaxTotalSizeMB, _ = l.config.Int64("log.max_total_size_mb")
	cfg.MinDiskFreeMB, _ = l.config.Int64("log.min_disk_free_mb")
	cfg.FlushIntervalMs, _ = l.config.Int64("log.flush_interval_ms")
	cfg.TraceDepth, _ = l.config.Int64("log.trace_depth")
	cfg.RetentionPeriodHrs, _ = l.config.Float64("log.retention_period_hrs")
	cfg.RetentionCheckMins, _ = l.config.Float64("log.retention_check_mins")
	cfg.DiskCheckIntervalMs, _ = l.config.Int64("log.disk_check_interval_ms")
	cfg.EnableAdaptiveInterval, _ = l.config.Bool("log.enable_adaptive_interval")
	cfg.MinCheckIntervalMs, _ = l.config.Int64("log.min_check_interval_ms")
	cfg.MaxCheckIntervalMs, _ = l.config.Int64("log.max_check_interval_ms")
	cfg.EnablePeriodicSync, _ = l.config.Bool("log.enable_periodic_sync")
	cfg.HeartbeatLevel, _ = l.config.Int64("log.heartbeat_level")
	cfg.HeartbeatIntervalS, _ = l.config.Int64("log.heartbeat_interval_s")
	return cfg
}

// getCurrentLogChannel safely retrieves the current log channel
func (l *Logger) getCurrentLogChannel() chan logRecord {
	chVal := l.state.ActiveLogChannel.Load()
	return chVal.(chan logRecord)
}

// getFlags from config
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
	if l.state.LoggerDisabled.Load() || !l.state.IsInitialized.Load() {
		return
	}

	configLevel, _ := l.config.Int64("log.level")
	if level < configLevel {
		return
	}

	// Report dropped logs first if there has been any
	currentDrops := l.state.DroppedLogs.Load()
	logged := l.state.LoggedDrops.Load()
	if currentDrops > logged {
		if l.state.LoggedDrops.CompareAndSwap(logged, currentDrops) {
			dropRecord := logRecord{
				Flags:     FlagDefault,
				TimeStamp: time.Now(),
				Level:     LevelError,
				Args:      []any{"Logs were dropped", "dropped_count", currentDrops - logged, "total_dropped", currentDrops},
			}
			l.sendLogRecord(dropRecord)
		}
	}

	var trace string
	if depth > 0 {
		const skipTrace = 3 // log.Info -> log -> getTrace (Adjust if call stack changes)
		trace = getTrace(depth, skipTrace)
	}

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