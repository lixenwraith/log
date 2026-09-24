package log

import (
	"errors"
	"fmt"
	"io"
	"maps"
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
	initMu        sync.Mutex   // serializes lifecycle and configuration changes
	sendMu        sync.RWMutex // joins enqueues before detaching a processor
	processorErr  error        // written by processor, read only after ProcDone closes
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

	return l
}

// ApplyConfig applies a validated configuration to the logger
// This is the primary way applications should configure the logger
func (l *Logger) ApplyConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("log: configuration cannot be nil")
	}

	cfg = cfg.Clone()
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
	l.initMu.Lock()
	defer l.initMu.Unlock()
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

	if err := cfg.Validate(); err != nil {
		return fmtErrorf("invalid configuration: %w", err)
	}
	return l.applyConfig(cfg)
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
	wasStarted := l.state.Started.Load()
	needsRestart := wasStarted && configRequiresRestart(oldCfg, cfg)

	// A timed-out processor still owns its file and formatter. Do not replace
	// either resource until it has actually exited.
	if !wasStarted && !l.state.ProcessorExited.Load() {
		return fmtErrorf("previous processor is still stopping")
	}

	currentFile, _ := l.state.CurrentFile.Load().(*os.File)
	needsNewFile := cfg.EnableFile && (!l.state.IsInitialized.Load() || !oldCfg.EnableFile ||
		!wasStarted && currentFile == nil || l.state.LoggerDisabled.Load() ||
		oldCfg.Directory != cfg.Directory || oldCfg.Name != cfg.Name || oldCfg.Extension != cfg.Extension)
	var newFile *os.File
	if cfg.EnableFile {
		if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
			return fmtErrorf("failed to create log directory '%s': %w", cfg.Directory, err)
		}
		if needsNewFile {
			var err error
			newFile, err = openLogFile(cfg)
			if err != nil {
				return fmtErrorf("failed to create log file: %w", err)
			}
		}
	}

	// Drain with the old configuration and file before publishing the new one.
	if needsRestart || wasStarted && needsNewFile {
		needsRestart = true
		if err := l.stopLocked(); err != nil {
			if newFile != nil {
				_ = newFile.Close()
			}
			return fmtErrorf("failed to stop processor for restart: %w", err)
		}
	}

	// Draining may rotate the old file; close the handle the processor left behind.
	currentFile, _ = l.state.CurrentFile.Load().(*os.File)
	if !cfg.EnableFile || newFile != nil {
		if currentFile != nil {
			if err := currentFile.Close(); err != nil {
				l.internalLog("failed to close old log file: %v\n", err)
			}
		}
		l.state.CurrentFile.Store(newFile)
		l.state.CurrentSize.Store(0)
		if newFile != nil {
			if info, err := newFile.Stat(); err == nil {
				l.state.CurrentSize.Store(info.Size())
			}
		}
	}

	var writer io.Writer = io.Discard
	if cfg.EnableConsole {
		writer = os.Stdout
		if cfg.ConsoleTarget == "stderr" {
			writer = os.Stderr
		}
	}
	l.state.StdoutWriter.Store(&sink{w: writer})
	l.currentConfig.Store(cfg)
	l.rebuildFormatter(cfg)
	l.state.Flags.Store(flagsFromConfig(cfg))
	l.state.TraceDepth.Store(cfg.TraceDepth)
	l.state.IsInitialized.Store(true)
	l.state.ShutdownCalled.Store(false)
	// A running processor may have just reported an output failure. A hot
	// configuration update must not revive its invalid file handle.
	if !wasStarted || needsRestart {
		l.state.LoggerDisabled.Store(false)
		l.state.DiskFullLogged.Store(false)
	}
	l.state.DiskStatusOK.Store(true)
	l.refreshLevelGate()
	if needsRestart {
		return l.startLocked()
	}
	return nil
}

// Start begins log processing. Repeated calls while running are no-ops.
// ApplyConfig must succeed first. A processor still stopping cannot be restarted.
func (l *Logger) Start() error {
	l.initMu.Lock()
	defer l.initMu.Unlock()
	return l.startLocked()
}

func (l *Logger) startLocked() error {
	if !l.state.IsInitialized.Load() || l.state.ShutdownCalled.Load() {
		return fmtErrorf("logger not initialized, call ApplyConfig first")
	}
	if l.state.Started.Load() {
		if l.state.ProcessorExited.Load() {
			return fmtErrorf("processor exited unexpectedly; call Stop before restarting")
		}
		return nil
	}
	if !l.state.ProcessorExited.Load() {
		return fmtErrorf("previous processor is still stopping")
	}

	ch := make(chan logRecord, l.getConfig().BufferSize)
	stop, done := make(chan struct{}), make(chan struct{})
	flush := make(chan flushRequest, 1)
	l.processorErr = nil
	l.sendMu.Lock()
	l.state.ActiveLogChannel.Store(ch)
	l.state.ProcStop.Store(stop)
	l.state.ProcDone.Store(done)
	l.state.flushRequestChan = flush
	l.state.ProcessorExited.Store(false)
	l.state.Started.Store(true)
	l.refreshLevelGate()
	l.sendMu.Unlock()
	l.spawn(func() { l.processLogs(ch, stop, done, flush) })
	return nil
}

// Stop detaches producers, drains accepted records, and joins the processor.
// After a timeout, Stop can be retried; Start and resource replacement remain
// blocked until that processor exits. The default timeout is 2x flush interval.
func (l *Logger) Stop(timeout ...time.Duration) error {
	l.initMu.Lock()
	defer l.initMu.Unlock()
	return l.stopLocked(timeout...)
}

func (l *Logger) stopLocked(timeout ...time.Duration) error {
	l.sendMu.Lock()
	if l.state.Started.Swap(false) {
		l.refreshLevelGate()
		l.state.ActiveLogChannel.Store((chan logRecord)(nil))
		close(l.state.ProcStop.Load().(chan struct{}))
	}
	l.sendMu.Unlock()

	done, _ := l.state.ProcDone.Load().(chan struct{})
	if done == nil {
		return nil
	}
	effectiveTimeout := 2 * time.Duration(l.getConfig().FlushIntervalMs) * time.Millisecond
	if len(timeout) > 0 {
		effectiveTimeout = timeout[0]
	}
	effectiveTimeout = max(effectiveTimeout, minWaitTime)
	// Prefer an already completed processor even with a tiny timeout.
	select {
	case <-done:
		return l.processorErr
	default:
	}
	timer := time.NewTimer(effectiveTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return l.processorErr
	case <-timer.C:
		return fmtErrorf("processor did not exit within timeout (%v)", effectiveTimeout)
	}
}

// Shutdown stops the logger and closes its file after the processor exits.
// If joining times out, resources remain owned by the processor; retry Shutdown
// to finish cleanup. A completed shutdown can be reset by ApplyConfig.
func (l *Logger) Shutdown(timeout ...time.Duration) error {
	l.initMu.Lock()
	defer l.initMu.Unlock()
	if !l.state.IsInitialized.Load() {
		return nil
	}
	l.state.ShutdownCalled.Store(true)
	l.state.LoggerDisabled.Store(true)
	l.refreshLevelGate()
	err := l.stopLocked(timeout...)
	if !l.state.ProcessorExited.Load() {
		return err
	}
	l.state.IsInitialized.Store(false)
	if file, _ := l.state.CurrentFile.Load().(*os.File); file != nil {
		err = errors.Join(err, file.Sync(), file.Close())
		l.state.CurrentFile.Store((*os.File)(nil))
	}
	return err
}

// Flush processes records queued before the request and syncs the current file.
// The timeout covers queueing and confirmation. Concurrent later records need
// not be drained, so sustained producers cannot starve the barrier.
func (l *Logger) Flush(timeout time.Duration) error {
	l.sendMu.RLock()
	if !l.state.IsInitialized.Load() || l.state.ShutdownCalled.Load() {
		l.sendMu.RUnlock()
		return fmtErrorf("logger not initialized or already shut down")
	}
	if !l.state.Started.Load() {
		l.sendMu.RUnlock()
		return fmtErrorf("logger not started")
	}
	requests := l.state.flushRequestChan
	done := l.state.ProcDone.Load().(chan struct{})
	l.sendMu.RUnlock()
	if timeout <= 0 {
		return fmtErrorf("timeout waiting for flush confirmation (%v)", timeout)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	request := flushRequest{done: make(chan error, 1)}
	select {
	case requests <- request:
	case <-done:
		return fmtErrorf("processor stopped during flush")
	case <-timer.C:
		return fmtErrorf("timeout sending flush request (%v)", timeout)
	}
	select {
	case err := <-request.done:
		return err
	case <-done:
		return fmtErrorf("processor stopped during flush")
	case <-timer.C:
		return fmtErrorf("timeout waiting for flush confirmation (%v)", timeout)
	}
}

// SetSpawn installs the goroutine launcher used by Start. Hosts that own
// panic recovery and terminal teardown pass their own launcher here.
// Call before Start; fn must launch asynchronously and return promptly.
// A nil fn restores the default.
func (l *Logger) SetSpawn(fn func(func())) {
	if fn == nil {
		l.spawner.Store(nil)
		return
	}
	l.spawner.Store(&fn)
}

// SetErrorHandler routes internal diagnostics to fn instead of stderr.
// Required for TUI hosts, where stderr writes corrupt the display. The callback
// must return promptly and must not call lifecycle/configuration methods or Flush.
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
	l.refreshLevelGate()
	l.initMu.Unlock()
}

// Enabled reports whether a record at level would be emitted. Single atomic
// load: the intended guard for hot call sites, where argument slices are
// built before the call and would otherwise escape to the heap.
func (l *Logger) Enabled(level int64) bool {
	threshold := l.state.Level.Load()
	return threshold != levelOff && level >= threshold
}

// Flags returns the default record flags derived from display config
func (l *Logger) Flags() int64 {
	return l.state.Flags.Load()
}

// LogContext emits a record with caller-supplied context and explicit flags
func (l *Logger) LogContext(ctx Context, flags, level, depth int64, args ...any) {
	if !l.Enabled(level) {
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
	if !l.Enabled(level) {
		return
	}
	l.LogContext(Context{}, l.getFlags()|FlagStructuredJSON, level, 0, message, maps.Clone(fields))
}

// Write outputs raw, unformatted data ignoring configured format and sanitization
func (l *Logger) Write(args ...any) {
	l.LogContext(Context{}, FlagRaw, LevelInfo, 0, args...)
}
