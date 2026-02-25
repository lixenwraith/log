package compat

import (
	"fmt"
	"os"
	"time"

	"github.com/lixenwraith/log"
)

// GnetAdapter wraps lixenwraith/log.Logger to implement gnet logging.Logger interface
type GnetAdapter struct {
	logger       *log.Logger
	fatalHandler func(msg string) // Customizable fatal behavior
}

// NewGnetAdapter creates a new gnet-compatible logger adapter
func NewGnetAdapter(logger *log.Logger, opts ...GnetOption) *GnetAdapter {
	adapter := &GnetAdapter{
		logger: logger,
		fatalHandler: func(msg string) {
			os.Exit(1) // Default behavior matches gnet expectations
		},
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// GnetOption allows customizing adapter behavior
type GnetOption func(*GnetAdapter)

// WithFatalHandler sets a custom fatal handler
func WithFatalHandler(handler func(string)) GnetOption {
	return func(a *GnetAdapter) {
		a.fatalHandler = handler
	}
}

// Debugf logs at debug level with printf-style formatting
func (a *GnetAdapter) Debugf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.logger.Debug("msg", msg, "source", "gnet")
}

// Infof logs at info level with printf-style formatting
func (a *GnetAdapter) Infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.logger.Info("msg", msg, "source", "gnet")
}

// Warnf logs at warn level with printf-style formatting
func (a *GnetAdapter) Warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.logger.Warn("msg", msg, "source", "gnet")
}

// Errorf logs at error level with printf-style formatting
func (a *GnetAdapter) Errorf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.logger.Error("msg", msg, "source", "gnet")
}

// Fatalf logs at error level and triggers fatal handler
func (a *GnetAdapter) Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	a.logger.Error("msg", msg, "source", "gnet", "fatal", true)

	// Ensure log is flushed before exit
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.fatalHandler != nil {
		a.fatalHandler(msg)
	}
}