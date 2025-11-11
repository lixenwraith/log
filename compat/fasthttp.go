// FILE: lixenwraith/log/compat/fasthttp.go
package compat

import (
	"fmt"
	"strings"

	"github.com/lixenwraith/log"
)

// FastHTTPAdapter wraps lixenwraith/log.Logger to implement fasthttp Logger interface
type FastHTTPAdapter struct {
	logger        *log.Logger
	defaultLevel  int64
	levelDetector func(string) int64 // Function to detect log level from message
}

// NewFastHTTPAdapter creates a new fasthttp-compatible logger adapter
func NewFastHTTPAdapter(logger *log.Logger, opts ...FastHTTPOption) *FastHTTPAdapter {
	adapter := &FastHTTPAdapter{
		logger:        logger,
		defaultLevel:  log.LevelInfo,
		levelDetector: DetectLogLevel, // Default level detection
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// FastHTTPOption allows customizing adapter behavior
type FastHTTPOption func(*FastHTTPAdapter)

// WithDefaultLevel sets the default log level for Printf calls
func WithDefaultLevel(level int64) FastHTTPOption {
	return func(a *FastHTTPAdapter) {
		a.defaultLevel = level
	}
}

// WithLevelDetector sets a custom function to detect log level from message content
func WithLevelDetector(detector func(string) int64) FastHTTPOption {
	return func(a *FastHTTPAdapter) {
		a.levelDetector = detector
	}
}

// Printf implements fasthttp's Logger interface
func (a *FastHTTPAdapter) Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)

	// Detect log level from message content
	level := a.defaultLevel
	if a.levelDetector != nil {
		detected := a.levelDetector(msg)
		if detected != 0 {
			level = detected
		}
	}

	// Log with appropriate level
	switch level {
	case log.LevelDebug:
		a.logger.Debug("msg", msg, "source", "fasthttp")
	case log.LevelWarn:
		a.logger.Warn("msg", msg, "source", "fasthttp")
	case log.LevelError:
		a.logger.Error("msg", msg, "source", "fasthttp")
	default:
		a.logger.Info("msg", msg, "source", "fasthttp")
	}
}

// DetectLogLevel attempts to detect log level from message content
func DetectLogLevel(msg string) int64 {
	msgLower := strings.ToLower(msg)

	// Check for error indicators
	if strings.Contains(msgLower, "error") ||
		strings.Contains(msgLower, "failed") ||
		strings.Contains(msgLower, "fatal") ||
		strings.Contains(msgLower, "panic") {
		return log.LevelError
	}

	// Check for warning indicators
	if strings.Contains(msgLower, "warn") ||
		strings.Contains(msgLower, "warning") ||
		strings.Contains(msgLower, "deprecated") {
		return log.LevelWarn
	}

	// Check for debug indicators
	if strings.Contains(msgLower, "debug") ||
		strings.Contains(msgLower, "trace") {
		return log.LevelDebug
	}

	// Default to info level
	return log.LevelInfo
}