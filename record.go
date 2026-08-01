package log

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// getCurrentLogChannel returns the active record channel, nil when detached
func (l *Logger) getCurrentLogChannel() chan logRecord {
	ch, _ := l.state.ActiveLogChannel.Load().(chan logRecord)
	return ch
}

// getFlags returns the cached default record flags
func (l *Logger) getFlags() int64 {
	return l.state.Flags.Load()
}

// sendLogRecord queues a record without blocking. The channel is never closed,
// so no recovery is needed: a detached (nil) channel takes the default branch.
func (l *Logger) sendLogRecord(record logRecord) {
	if l.state.LoggerDisabled.Load() {
		l.handleFailedSend()
		return
	}
	ch := l.getCurrentLogChannel()
	select {
	case ch <- record:
	default:
		l.handleFailedSend()
	}
}

// handleFailedSend increments drop counters
func (l *Logger) handleFailedSend() {
	l.state.DroppedLogs.Add(1)      // Interval counter
	l.state.TotalDroppedLogs.Add(1) // Total counter
}

// emit builds and queues a record; the caller has already passed the level gate
func (l *Logger) emit(ctx Context, flags, level, depth int64, args []any) {
	// Depth filter hard-coded based on call stack of current package design
	var trace string
	if depth > 0 {
		const skipTrace = 3 // Logger.Info -> emit -> getTrace
		trace = getTrace(depth, skipTrace)
	}

	l.sendLogRecord(logRecord{
		Ctx:       ctx,
		Flags:     flags,
		TimeStamp: time.Now(),
		Level:     level,
		Trace:     trace,
		Args:      args,
	})
}

// internalLog reports logger diagnostics to the registered handler, or to
// stderr when configured. Hosts owning a terminal must register a handler.
func (l *Logger) internalLog(format string, args ...any) {
	if !strings.HasPrefix(format, "log: ") {
		format = "log: " + format
	}

	if h := l.errHandler.Load(); h != nil {
		(*h)(strings.TrimRight(fmt.Sprintf(format, args...), "\n"))
		return
	}

	if !l.getConfig().InternalErrorsToStderr {
		return
	}
	fmt.Fprintf(os.Stderr, format, args...)
}

// // log handles the core logging logic
// func (l *Logger) log(flags int64, level int64, depth int64, args ...any) {
// 	// State checks
// 	if !l.state.IsInitialized.Load() {
// 		return
// 	}
//
// 	if !l.state.Started.Load() {
// 		// Log to internal error channel if configured
// 		cfg := l.getConfig()
// 		if cfg.InternalErrorsToStderr {
// 			l.internalLog("warning - logger not started, dropping log entry\n")
// 		}
// 		return
// 	}
//
// 	// Discard or proceed based on level
// 	cfg := l.getConfig()
// 	if level < cfg.Level {
// 		return
// 	}
//
// 	// Get trace info from runtime
// 	// Depth filter hard-coded based on call stack of current package design
// 	var trace string
// 	if depth > 0 {
// 		const skipTrace = 3 // log.Info -> log -> getTrace (Adjust if call stack changes)
// 		trace = getTrace(depth, skipTrace)
// 	}
//
// 	record := logRecord{
// 		Flags:     flags,
// 		TimeStamp: time.Now(),
// 		Level:     level,
// 		Trace:     trace,
// 		Args:      args,
// 	}
// 	l.sendLogRecord(record)
// }
