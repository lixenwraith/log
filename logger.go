package log

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lixenwraith/log/formatter"
	"github.com/lixenwraith/log/sanitizer"
)

// Logger is the core struct that encapsulates all logger functionality
type Logger struct {
	currentConfig atomic.Value // stores *Config
	formatter     atomic.Value // stores *formatter.Formatter
	ctxKeys       atomic.Pointer[formatter.ContextKeys]
	spawner       atomic.Pointer[func(func())]
	errHandler    atomic.Pointer[func(string)]
	state         State
	initMu        sync.Mutex
}

// levelOff closes the emit gate without touching configuration
const levelOff int64 = math.MaxInt64

// NewLogger creates a new Logger instance with default settings
func NewLogger() *Logger {
	l := &Logger{}

	// Set default configuration
	defaultCfg := DefaultConfig()
	l.currentConfig.Store(defaultCfg)
	l.rebuildFormatter(defaultCfg)

	// Emission stays closed until ApplyConfig and Start succeed
	l.state.Level.Store(levelOff)
	l.state.Flags.Store(flagsFromConfig(defaultCfg))
	l.state.TraceDepth.Store(defaultCfg.TraceDepth)

	// // Initialize default formatter to prevent nil access
	// defaultFormatter := formatter.New(sanitizer.New()).
	// 	Type(defaultCfg.Format).
	// 	TimestampFormat(defaultCfg.TimestampFormat).
	// 	ShowLevel(defaultCfg.ShowLevel).
	// 	ShowTimestamp(defaultCfg.ShowTimestamp)
	// l.formatter.Store(defaultFormatter)

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

	// Typed nil: a non-blocking send on a nil channel always takes default
	l.state.ActiveLogChannel.Store((chan logRecord)(nil))
	l.state.flushRequestChan = make(chan chan struct{}, 1)

	// // Create a closed channel initially to prevent nil pointer issues
	// initialChan := make(chan logRecord)
	// close(initialChan)
	// l.state.ActiveLogChannel.Store(initialChan)
	//
	// l.state.flushRequestChan = make(chan chan struct{}, 1)

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

// ApplyConfigString applies string key-value overrides to the logger's current configuration
// Each override should be in the format "key=value"
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

// getConfig returns the current configuration (thread-safe)
func (l *Logger) getConfig() *Config {
	return l.currentConfig.Load().(*Config)
}

// applyConfig is the internal implementation for applying configuration, assuming initMu is held
func (l *Logger) applyConfig(cfg *Config) error {
	oldCfg := l.getConfig()
	l.currentConfig.Store(cfg)

	// Shared formatter and sanitizer constructor with SetContextKeys
	l.rebuildFormatter(cfg)

	// Emit fast-path mirrors
	l.state.Flags.Store(flagsFromConfig(cfg))
	l.state.TraceDepth.Store(cfg.TraceDepth)

	// Ensure log directory exists if file output is enabled
	if cfg.EnableFile {
		if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
			l.state.LoggerDisabled.Store(true)
			l.currentConfig.Store(oldCfg) // Rollback
			l.refreshLevelGate()
			return fmtErrorf("failed to create log directory '%s': %w", cfg.Directory, err)
		}
	}

	// Get current state
	wasInitialized := l.state.IsInitialized.Load()
	wasStarted := l.state.Started.Load()

	// Determine if restart is needed
	needsRestart := wasStarted && wasInitialized && configRequiresRestart(oldCfg, cfg)

	// Stop processor if restart needed
	if needsRestart {
		if err := l.Stop(); err != nil {
			l.currentConfig.Store(oldCfg) // Rollback
			return fmtErrorf("failed to stop processor for restart: %w", err)
		}
	}

	// Get current file handle
	currentFilePtr := l.state.CurrentFile.Load()
	var currentFile *os.File
	if currentFilePtr != nil {
		currentFile, _ = currentFilePtr.(*os.File)
	}

	// Determine if we need a new file
	needsNewFile := !wasInitialized || currentFile == nil ||
		oldCfg.Directory != cfg.Directory ||
		oldCfg.Name != cfg.Name ||
		oldCfg.Extension != cfg.Extension

	// Handle file state transitions
	if !cfg.EnableFile {
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

	// Setup console writer based on config
	if cfg.EnableConsole {
		var writer io.Writer
		if cfg.ConsoleTarget == "stderr" {
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
	l.refreshLevelGate()

	// Restart processor if it was running and needs restart
	if needsRestart {
		return l.Start()
	}

	return nil
}

// Start begins log processing. Safe to call multiple times
// Returns error if logger is not initialized
func (l *Logger) Start() error {
	if !l.state.IsInitialized.Load() {
		return fmtErrorf("logger not initialized, call ApplyConfig first")
	}

	// Check if processor didn't exit cleanly last time
	if l.state.Started.Load() && !l.state.ProcessorExited.Load() {
		// Force stop to clean up
		l.internalLog("warning - processor still running from previous start, forcing stop\n")
		if err := l.Stop(); err != nil {
			return fmtErrorf("failed to stop hung processor: %w", err)
		}
	}

	// Only start if not already started
	if l.state.Started.CompareAndSwap(false, true) {
		cfg := l.getConfig()

		// Create log channels
		ch := make(chan logRecord, cfg.BufferSize)
		stop := make(chan struct{})
		done := make(chan struct{})

		l.state.ActiveLogChannel.Store(ch)
		l.state.ProcStop.Store(stop)
		l.state.ProcDone.Store(done)
		l.state.ProcessorExited.Store(false)

		// Start processor
		l.spawn(func() { l.processLogs(ch, stop, done) })
	}

	l.refreshLevelGate()
	return nil
}

// Stop halts log processing. Can be restarted with Start()
// The record channel is never closed: producers are detached to a nil channel
// first, so a concurrent send falls through to the drop counter instead of
// racing a close.
func (l *Logger) Stop(timeout ...time.Duration) error {
	if !l.state.Started.CompareAndSwap(true, false) {
		return nil // Already stopped
	}
	l.refreshLevelGate()

	// Calculate effective timeout
	var effectiveTimeout time.Duration
	if len(timeout) > 0 {
		effectiveTimeout = timeout[0]
	} else {
		effectiveTimeout = 2 * time.Duration(l.getConfig().FlushIntervalMs) * time.Millisecond
	}
	if effectiveTimeout < minWaitTime {
		effectiveTimeout = minWaitTime
	}

	// 1. Detach producers
	l.state.ActiveLogChannel.Store((chan logRecord)(nil))

	// 2. Signal the processor to drain and exit
	if s, ok := l.state.ProcStop.Load().(chan struct{}); ok && s != nil {
		close(s)
	}

	// 3. Join
	d, _ := l.state.ProcDone.Load().(chan struct{})
	if d == nil {
		return nil
	}
	select {
	case <-d:
		return nil
	case <-time.After(effectiveTimeout):
		return fmtErrorf("processor did not exit within timeout (%v)", effectiveTimeout)
	}
}

// Shutdown gracefully closes the logger, attempting to flush pending records
// If no timeout is provided, uses a default of 2x flush interval
func (l *Logger) Shutdown(timeout ...time.Duration) error {
	if !l.state.ShutdownCalled.CompareAndSwap(false, true) {
		return nil
	}

	l.state.LoggerDisabled.Store(true)
	l.refreshLevelGate()

	if !l.state.IsInitialized.Load() {
		l.state.ShutdownCalled.Store(false)
		l.state.LoggerDisabled.Store(false)
		l.state.ProcessorExited.Store(true)
		l.refreshLevelGate()
		return nil
	}

	var stopErr error
	if l.state.Started.Load() {
		stopErr = l.Stop(timeout...)
	}

	l.state.IsInitialized.Store(false)

	var finalErr error
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if currentLogFile, ok := cfPtr.(*os.File); ok && currentLogFile != nil {
			if err := currentLogFile.Sync(); err != nil {
				syncErr := fmtErrorf("failed to sync log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = errors.Join(finalErr, syncErr)
			}
			if err := currentLogFile.Close(); err != nil {
				closeErr := fmtErrorf("failed to close log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = errors.Join(finalErr, closeErr)
			}
			l.state.CurrentFile.Store((*os.File)(nil))
		}
	}

	if stopErr != nil {
		finalErr = errors.Join(finalErr, stopErr)
	}

	return finalErr
}

// Flush explicitly triggers a sync of the current log file buffer to disk and waits for completion or timeout
func (l *Logger) Flush(timeout time.Duration) error {
	l.state.flushMutex.Lock()
	defer l.state.flushMutex.Unlock()

	// State checks
	if !l.state.IsInitialized.Load() || l.state.ShutdownCalled.Load() {
		return fmtErrorf("logger not initialized or already shut down")
	}
	if !l.state.Started.Load() {
		return fmtErrorf("logger not started")
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

// SetSpawn installs the goroutine launcher used by Start. Hosts that own
// panic recovery and terminal teardown pass their own launcher here.
// Call before Start; a nil fn restores the default.
func (l *Logger) SetSpawn(fn func(func())) {
	if fn == nil {
		l.spawner.Store(nil)
		return
	}
	l.spawner.Store(&fn)
}

// SetErrorHandler routes internal diagnostics to fn instead of stderr.
// Required for TUI hosts, where stderr writes corrupt the display.
func (l *Logger) SetErrorHandler(fn func(string)) {
	if fn == nil {
		l.errHandler.Store(nil)
		return
	}
	l.errHandler.Store(&fn)
}

// SetContextKeys names the record keys for Context values; empty names are
// omitted. Safe to call at any time; rebuilds the formatter.
func (l *Logger) SetContextKeys(tag string, vals ...string) {
	l.initMu.Lock()
	defer l.initMu.Unlock()

	k := formatter.ContextKeys{Tag: tag}
	for i := 0; i < len(vals) && i < formatter.ContextSlots; i++ {
		k.Vals[i] = vals[i]
	}
	l.ctxKeys.Store(&k)
	l.rebuildFormatter(l.getConfig())
}

// SetLevel changes the emit threshold in place, leaving the formatter and the
// processor untouched
func (l *Logger) SetLevel(level int64) {
	l.initMu.Lock()
	cfg := l.getConfig().Clone()
	cfg.Level = level
	l.currentConfig.Store(cfg)
	l.initMu.Unlock()
	l.refreshLevelGate()
}

// Enabled reports whether a record at level would be emitted. Single atomic
// load: the intended guard for hot call sites, where argument slices are
// built before the call and would otherwise escape to the heap.
func (l *Logger) Enabled(level int64) bool {
	return level >= l.state.Level.Load()
}

// Flags returns the default record flags derived from display config
func (l *Logger) Flags() int64 {
	return l.state.Flags.Load()
}

// LogContext emits a record with caller-supplied context and explicit flags
func (l *Logger) LogContext(ctx Context, flags, level, depth int64, args ...any) {
	if level < l.state.Level.Load() {
		return
	}
	l.emit(ctx, flags, level, depth, args)
}

// spawn runs fn via the configured launcher; the default is a bare goroutine
func (l *Logger) spawn(fn func()) {
	if p := l.spawner.Load(); p != nil {
		(*p)(fn)
		return
	}
	go fn()
}

// rebuildFormatter installs a formatter matching cfg and the current context keys
func (l *Logger) rebuildFormatter(cfg *Config) {
	f := formatter.New(sanitizer.New().Policy(cfg.Sanitization)).
		Type(cfg.Format).
		TimestampFormat(cfg.TimestampFormat).
		ShowLevel(cfg.ShowLevel).
		ShowTimestamp(cfg.ShowTimestamp)
	if k := l.ctxKeys.Load(); k != nil {
		f.ContextKeys(k.Tag, k.Vals[:]...)
	}
	l.formatter.Store(f)
}

// flagsFromConfig derives the default record flags from display settings
func flagsFromConfig(cfg *Config) int64 {
	var flags int64
	if cfg.ShowLevel {
		flags |= FlagShowLevel
	}
	if cfg.ShowTimestamp {
		flags |= FlagShowTimestamp
	}
	return flags
}

// refreshLevelGate recomputes the emit gate from lifecycle state.
// MUST be called after every transition of IsInitialized, Started,
// LoggerDisabled, or ShutdownCalled.
func (l *Logger) refreshLevelGate() {
	if !l.state.IsInitialized.Load() || !l.state.Started.Load() ||
		l.state.LoggerDisabled.Load() || l.state.ShutdownCalled.Load() {
		l.state.Level.Store(levelOff)
		return
	}
	l.state.Level.Store(l.getConfig().Level)
}

// === Logging methods ===

// Debug logs a message at debug level
func (l *Logger) Debug(args ...any) {
	if LevelDebug < l.state.Level.Load() {
		return
	}
	l.emit(Context{}, l.getFlags(), LevelDebug, l.state.TraceDepth.Load(), args)
}

// Info logs a message at info level
func (l *Logger) Info(args ...any) {
	if LevelInfo < l.state.Level.Load() {
		return
	}
	l.emit(Context{}, l.getFlags(), LevelInfo, l.state.TraceDepth.Load(), args)
}

// Warn logs a message at warning level
func (l *Logger) Warn(args ...any) {
	if LevelWarn < l.state.Level.Load() {
		return
	}
	l.emit(Context{}, l.getFlags(), LevelWarn, l.state.TraceDepth.Load(), args)
}

// Error logs a message at error level
func (l *Logger) Error(args ...any) {
	if LevelError < l.state.Level.Load() {
		return
	}
	l.emit(Context{}, l.getFlags(), LevelError, l.state.TraceDepth.Load(), args)
}

// DebugTrace logs a debug message with function call trace
func (l *Logger) DebugTrace(depth int, args ...any) {
	l.LogContext(Context{}, l.getFlags(), LevelDebug, int64(depth), args...)
}

// InfoTrace logs an info message with function call trace
func (l *Logger) InfoTrace(depth int, args ...any) {
	l.LogContext(Context{}, l.getFlags(), LevelInfo, int64(depth), args...)
}

// WarnTrace logs a warning message with function call trace
func (l *Logger) WarnTrace(depth int, args ...any) {
	l.LogContext(Context{}, l.getFlags(), LevelWarn, int64(depth), args...)
}

// ErrorTrace logs an error message with function call trace
func (l *Logger) ErrorTrace(depth int, args ...any) {
	l.LogContext(Context{}, l.getFlags(), LevelError, int64(depth), args...)
}

// Log writes a timestamp-only record without level information
func (l *Logger) Log(args ...any) {
	l.LogContext(Context{}, FlagShowTimestamp|FlagNoLevel, LevelInfo, 0, args...)
}

// Message writes a plain record without timestamp or level info
func (l *Logger) Message(args ...any) {
	l.LogContext(Context{}, FlagNoTimestamp|FlagNoLevel, LevelInfo, 0, args...)
}

// LogTrace writes a timestamp record with call trace but no level info
func (l *Logger) LogTrace(depth int, args ...any) {
	l.LogContext(Context{}, FlagShowTimestamp|FlagNoLevel, LevelInfo, int64(depth), args...)
}

// LogStructured logs a message with structured fields as proper JSON
func (l *Logger) LogStructured(level int64, message string, fields map[string]any) {
	l.LogContext(Context{}, l.getFlags()|FlagStructuredJSON, level, 0, message, fields)
}

// Write outputs raw, unformatted data ignoring configured format and sanitization
func (l *Logger) Write(args ...any) {
	l.LogContext(Context{}, FlagRaw, LevelInfo, 0, args...)
}
