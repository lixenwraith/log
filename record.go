// FILE: lixenwraith/log/record.go
package log

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// getCurrentLogChannel safely retrieves the current log channel
func (l *Logger) getCurrentLogChannel() chan logRecord {
	chVal := l.state.ActiveLogChannel.Load()
	// No defensive nil check required in correct use of initialized logger
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

// sendLogRecord handles safe sending to the active channel
func (l *Logger) sendLogRecord(record logRecord) {
	defer func() {
		if r := recover(); r != nil {
			// A panic is only expected when a race condition occurs during shutdown
			if err, ok := r.(error); ok && err.Error() == "send on closed channel" {
				// Expected race condition between logging and shutdown
				l.handleFailedSend()
			} else {
				// Unexpected panic, re-throw to surface
				panic(r)
			}
		}
	}()

	if l.state.ShutdownCalled.Load() ||
		l.state.LoggerDisabled.Load() ||
		!l.state.Started.Load() {
		// Process drops even if logger is disabled or shutting down
		l.handleFailedSend()
		return
	}

	ch := l.getCurrentLogChannel()

	// Non-blocking send
	select {
	case ch <- record:
		// Success
	default:
		l.handleFailedSend()
	}
}

// handleFailedSend increments drop counters
func (l *Logger) handleFailedSend() {
	l.state.DroppedLogs.Add(1)      // Interval counter
	l.state.TotalDroppedLogs.Add(1) // Total counter
}

// log handles the core logging logic
func (l *Logger) log(flags int64, level int64, depth int64, args ...any) {
	// State checks
	if !l.state.IsInitialized.Load() {
		return
	}

	if !l.state.Started.Load() {
		// Log to internal error channel if configured
		cfg := l.getConfig()
		if cfg.InternalErrorsToStderr {
			l.internalLog("warning - logger not started, dropping log entry\n")
		}
		return
	}

	// Discard or proceed based on level
	cfg := l.getConfig()
	if level < cfg.Level {
		return
	}

	// Get trace info from runtime
	// Depth filter hard-coded based on call stack of current package design
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

// internalLog handles writing internal logger diagnostics to stderr if enabled
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