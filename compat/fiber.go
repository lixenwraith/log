package compat

import (
	"fmt"
	"os"
	"time"

	"github.com/lixenwraith/log"
)

// FiberAdapter wraps lixenwraith/log.Logger to implement Fiber's CommonLogger interface
// This provides compatibility with Fiber v2.54.x logging requirements
type FiberAdapter struct {
	logger       *log.Logger
	fatalHandler func(msg string) // Customizable fatal behavior
	panicHandler func(msg string) // Customizable panic behavior
}

// NewFiberAdapter creates a new Fiber-compatible logger adapter
func NewFiberAdapter(logger *log.Logger, opts ...FiberOption) *FiberAdapter {
	adapter := &FiberAdapter{
		logger: logger,
		fatalHandler: func(msg string) {
			os.Exit(1) // Default behavior
		},
		panicHandler: func(msg string) {
			panic(msg) // Default behavior
		},
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// FiberOption allows customizing adapter behavior
type FiberOption func(*FiberAdapter)

// WithFiberFatalHandler sets a custom fatal handler
func WithFiberFatalHandler(handler func(string)) FiberOption {
	return func(a *FiberAdapter) {
		a.fatalHandler = handler
	}
}

// WithFiberPanicHandler sets a custom panic handler
func WithFiberPanicHandler(handler func(string)) FiberOption {
	return func(a *FiberAdapter) {
		a.panicHandler = handler
	}
}

// --- Logger interface implementation (7 methods) ---

// Trace logs at trace/debug level
func (a *FiberAdapter) Trace(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Debug("msg", msg, "source", "fiber", "level", "trace")
}

// Debug logs at debug level
func (a *FiberAdapter) Debug(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Debug("msg", msg, "source", "fiber")
}

// Info logs at info level
func (a *FiberAdapter) Info(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Info("msg", msg, "source", "fiber")
}

// Warn logs at warn level
func (a *FiberAdapter) Warn(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Warn("msg", msg, "source", "fiber")
}

// Error logs at error level
func (a *FiberAdapter) Error(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Error("msg", msg, "source", "fiber")
}

// Fatal logs at error level and triggers fatal handler
func (a *FiberAdapter) Fatal(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Error("msg", msg, "source", "fiber", "fatal", true)

	// Ensure log is flushed before exit
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.fatalHandler != nil {
		a.fatalHandler(msg)
	}
}

// Panic logs at error level and triggers panic handler
func (a *FiberAdapter) Panic(v ...any) {
	msg := fmt.Sprint(v...)
	a.logger.Error("msg", msg, "source", "fiber", "panic", true)

	// Ensure log is flushed before panic
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.panicHandler != nil {
		a.panicHandler(msg)
	}
}

// Write makes FiberAdapter implement io.Writer interface
// This allows it to be used with fiber.Config.ErrorHandler output redirection
func (a *FiberAdapter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Trim trailing newline if present
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	a.logger.Info("msg", msg, "source", "fiber")
	return len(p), nil
}

// --- FormatLogger interface implementation (7 methods) ---

// Tracef logs at trace/debug level with printf-style formatting
func (a *FiberAdapter) Tracef(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Debug("msg", msg, "source", "fiber", "level", "trace")
}

// Debugf logs at debug level with printf-style formatting
func (a *FiberAdapter) Debugf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Debug("msg", msg, "source", "fiber")
}

// Infof logs at info level with printf-style formatting
func (a *FiberAdapter) Infof(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Info("msg", msg, "source", "fiber")
}

// Warnf logs at warn level with printf-style formatting
func (a *FiberAdapter) Warnf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Warn("msg", msg, "source", "fiber")
}

// Errorf logs at error level with printf-style formatting
func (a *FiberAdapter) Errorf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Error("msg", msg, "source", "fiber")
}

// Fatalf logs at error level and triggers fatal handler
func (a *FiberAdapter) Fatalf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Error("msg", msg, "source", "fiber", "fatal", true)

	// Ensure log is flushed before exit
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.fatalHandler != nil {
		a.fatalHandler(msg)
	}
}

// Panicf logs at error level and triggers panic handler
func (a *FiberAdapter) Panicf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	a.logger.Error("msg", msg, "source", "fiber", "panic", true)

	// Ensure log is flushed before panic
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.panicHandler != nil {
		a.panicHandler(msg)
	}
}

// --- WithLogger interface implementation (7 methods) ---

// Tracew logs at trace/debug level with structured key-value pairs
func (a *FiberAdapter) Tracew(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+6)
	fields = append(fields, "msg", msg, "source", "fiber", "level", "trace")
	fields = append(fields, keysAndValues...)
	a.logger.Debug(fields...)
}

// Debugw logs at debug level with structured key-value pairs
func (a *FiberAdapter) Debugw(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+4)
	fields = append(fields, "msg", msg, "source", "fiber")
	fields = append(fields, keysAndValues...)
	a.logger.Debug(fields...)
}

// Infow logs at info level with structured key-value pairs
func (a *FiberAdapter) Infow(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+4)
	fields = append(fields, "msg", msg, "source", "fiber")
	fields = append(fields, keysAndValues...)
	a.logger.Info(fields...)
}

// Warnw logs at warn level with structured key-value pairs
func (a *FiberAdapter) Warnw(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+4)
	fields = append(fields, "msg", msg, "source", "fiber")
	fields = append(fields, keysAndValues...)
	a.logger.Warn(fields...)
}

// Errorw logs at error level with structured key-value pairs
func (a *FiberAdapter) Errorw(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+4)
	fields = append(fields, "msg", msg, "source", "fiber")
	fields = append(fields, keysAndValues...)
	a.logger.Error(fields...)
}

// Fatalw logs at error level with structured key-value pairs and triggers fatal handler
func (a *FiberAdapter) Fatalw(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+6)
	fields = append(fields, "msg", msg, "source", "fiber", "fatal", true)
	fields = append(fields, keysAndValues...)
	a.logger.Error(fields...)

	// Ensure log is flushed before exit
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.fatalHandler != nil {
		a.fatalHandler(msg)
	}
}

// Panicw logs at error level with structured key-value pairs and triggers panic handler
func (a *FiberAdapter) Panicw(msg string, keysAndValues ...any) {
	fields := make([]any, 0, len(keysAndValues)+6)
	fields = append(fields, "msg", msg, "source", "fiber", "panic", true)
	fields = append(fields, keysAndValues...)
	a.logger.Error(fields...)

	// Ensure log is flushed before panic
	_ = a.logger.Flush(100 * time.Millisecond)

	if a.panicHandler != nil {
		a.panicHandler(msg)
	}
}