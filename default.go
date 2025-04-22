// --- File: default.go ---
package log

import (
	"time"

	"github.com/LixenWraith/config"
)

// Global instance for package-level functions
var defaultLogger = NewLogger()

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

// Flush triggers a sync of the current log file buffer to disk and waits for completion or timeout
func Flush(timeout time.Duration) error {
	return defaultLogger.Flush(timeout)
}