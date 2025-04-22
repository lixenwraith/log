// --- File: interface.go ---
package log

import (
	"time"

	"github.com/LixenWraith/config"
)

// Log level constants
const (
	LevelDebug int64 = -4
	LevelInfo  int64 = 0
	LevelWarn  int64 = 4
	LevelError int64 = 8
)

// Record flags for controlling output structure
const (
	FlagShowTimestamp int64 = 0b01
	FlagShowLevel     int64 = 0b10
	FlagDefault             = FlagShowTimestamp | FlagShowLevel
)

// logRecord represents a single log entry.
type logRecord struct {
	Flags     int64
	TimeStamp time.Time
	Level     int64
	Trace     string
	Args      []any
}

// LoggerInterface defines the public methods for a logger implementation.
type LoggerInterface interface {
	// Init initializes or reconfigures the logger using the provided config.Config instance
	Init(cfg *config.Config, basePath string) error

	// InitWithDefaults initializes the logger with built-in defaults and optional overrides
	InitWithDefaults(overrides ...string) error

	// Shutdown gracefully closes the logger, attempting to flush pending records
	Shutdown(timeout time.Duration) error

	// Debug logs a message at debug level
	Debug(args ...any)

	// Info logs a message at info level
	Info(args ...any)

	// Warn logs a message at warning level
	Warn(args ...any)

	// Error logs a message at error level
	Error(args ...any)

	// DebugTrace logs a debug message with function call trace
	DebugTrace(depth int, args ...any)

	// InfoTrace logs an info message with function call trace
	InfoTrace(depth int, args ...any)

	// WarnTrace logs a warning message with function call trace
	WarnTrace(depth int, args ...any)

	// ErrorTrace logs an error message with function call trace
	ErrorTrace(depth int, args ...any)

	// Log writes a timestamp-only record without level information
	Log(args ...any)

	// Message writes a plain record without timestamp or level info
	Message(args ...any)

	// LogTrace writes a timestamp record with call trace but no level info
	LogTrace(depth int, args ...any)

	// SaveConfig saves the current logger configuration to a file
	SaveConfig(path string) error

	// LoadConfig loads logger configuration from a file with optional CLI overrides
	LoadConfig(path string, args []string) error
}

// Compile-time check to ensure Logger implements LoggerInterface
var _ LoggerInterface = (*Logger)(nil)