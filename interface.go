// FILE: interface.go
package log

import (
	"time"
)

// Log level constants
const (
	LevelDebug int64 = -4
	LevelInfo  int64 = 0
	LevelWarn  int64 = 4
	LevelError int64 = 8
)

// Heartbeat log levels
const (
	LevelProc int64 = 12
	LevelDisk int64 = 16
	LevelSys  int64 = 20
)

// Record flags for controlling output structure
const (
	FlagShowTimestamp int64 = 0b001
	FlagShowLevel     int64 = 0b010
	FlagRaw           int64 = 0b100
	FlagDefault             = FlagShowTimestamp | FlagShowLevel
)

// logRecord represents a single log entry.
type logRecord struct {
	Flags           int64
	TimeStamp       time.Time
	Level           int64
	Trace           string
	Args            []any
	unreportedDrops uint64 // Dropped log tracker
}

// Logger instance methods for configuration and logging at different levels.

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

// Write outputs raw, unformatted data regardless of configured format.
// This method bypasses all formatting (timestamps, levels, JSON structure)
// and writes args as space-separated strings without a trailing newline.
func (l *Logger) Write(args ...any) {
	l.log(FlagRaw, LevelInfo, 0, args...)
}