package compat

import (
	"fmt"

	"github.com/lixenwraith/log"
)

// Builder provides a flexible way to create configured logger adapters for gnet and fasthttp
// It can use an existing *log.Logger instance or create a new one from a *log.Config
type Builder struct {
	logger *log.Logger
	logCfg *log.Config
	err    error
}

// NewBuilder creates a new adapter builder
func NewBuilder() *Builder {
	return &Builder{}
}

// WithLogger specifies an existing logger to use for the adapters
// Recommended for applications that already have a central logger instance
// If this is set WithConfig is ignored
func (b *Builder) WithLogger(l *log.Logger) *Builder {
	if l == nil {
		b.err = fmt.Errorf("log/compat: provided logger cannot be nil")
		return b
	}
	b.logger = l
	return b
}

// WithConfig provides a configuration for a new logger instance
// This is used only if an existing logger is NOT provided via WithLogger
// If neither WithLogger nor WithConfig is used, a default logger will be created
func (b *Builder) WithConfig(cfg *log.Config) *Builder {
	b.logCfg = cfg
	return b
}

// getLogger resolves the logger to be used, creating one if necessary
func (b *Builder) getLogger() (*log.Logger, error) {
	if b.err != nil {
		return nil, b.err
	}

	// An existing logger was provided, so we use it
	if b.logger != nil {
		return b.logger, nil
	}

	// Create a new logger instance
	l := log.NewLogger()
	cfg := b.logCfg
	if cfg == nil {
		// If no config was provided, use the default
		cfg = log.DefaultConfig()
	}

	// Apply the configuration
	if err := l.ApplyConfig(cfg); err != nil {
		return nil, err
	}

	// Cache the newly created logger for subsequent builds with this builder
	b.logger = l
	return l, nil
}

// BuildGnet creates a gnet adapter
// It can be used for servers that require a standard gnet logger
func (b *Builder) BuildGnet(opts ...GnetOption) (*GnetAdapter, error) {
	l, err := b.getLogger()
	if err != nil {
		return nil, err
	}
	return NewGnetAdapter(l, opts...), nil
}

// BuildStructuredGnet creates a gnet adapter that attempts to extract structured
// fields from log messages for richer, queryable logs
func (b *Builder) BuildStructuredGnet(opts ...GnetOption) (*StructuredGnetAdapter, error) {
	l, err := b.getLogger()
	if err != nil {
		return nil, err
	}
	return NewStructuredGnetAdapter(l, opts...), nil
}

// BuildFastHTTP creates a fasthttp adapter
func (b *Builder) BuildFastHTTP(opts ...FastHTTPOption) (*FastHTTPAdapter, error) {
	l, err := b.getLogger()
	if err != nil {
		return nil, err
	}
	return NewFastHTTPAdapter(l, opts...), nil
}

// BuildFiber creates a Fiber v2.54.x adapter
func (b *Builder) BuildFiber(opts ...FiberOption) (*FiberAdapter, error) {
	l, err := b.getLogger()
	if err != nil {
		return nil, err
	}
	return NewFiberAdapter(l, opts...), nil
}

// GetLogger returns the underlying *log.Logger instance
// If a logger has not been provided or created yet, it will be initialized
func (b *Builder) GetLogger() (*log.Logger, error) {
	return b.getLogger()
}

// --- Example Usage ---
//
// The following demonstrates how to integrate lixenwraith/log with gnet, fasthttp, and Fiber
// using a single, shared logger instance
//
//	// 1. Create and configure application's main logger
//	appLogger := log.NewLogger()
//	logCfg := log.DefaultConfig()
//	logCfg.Level = log.LevelDebug
//	if err := appLogger.ApplyConfig(logCfg); err != nil {
//		panic(fmt.Sprintf("failed to configure logger: %v", err))
//	}
//
//	// 2. Create a builder and provide the existing logger
//	builder := compat.NewBuilder().WithLogger(appLogger)
//
//	// 3. Build the required adapters
//	gnetLogger, err := builder.BuildGnet()
//	if err != nil { /* handle error */ }
//
//	fasthttpLogger, err := builder.BuildFastHTTP()
//	if err != nil { /* handle error */ }
//
//	fiberLogger, err := builder.BuildFiber()
//	if err != nil { /* handle error */ }
//
//	// 4. Configure your servers with the adapters
//
//	// For gnet:
//	var events gnet.EventHandler // your-event-handler
//	// The adapter is passed directly into the gnet options
//	go gnet.Run(events, "tcp://:9000", gnet.WithLogger(gnetLogger))
//
//	// For fasthttp:
//	// The adapter is assigned directly to the server's Logger field
//	server := &fasthttp.Server{
//		Handler: func(ctx *fasthttp.RequestCtx) {
//			ctx.WriteString("Hello, world!")
//		},
//		Logger: fasthttpLogger,
//	}
//	go server.ListenAndServe(":8080")
//
//	// For Fiber v2.54.x:
//	// The adapter is passed to fiber.New() via the config
//	app := fiber.New(fiber.Config{
//		AppName: "My Application",
//	})
//	app.UpdateConfig(fiber.Config{
//		AppName: "My Application",
//	})
//	// Note: Set the logger after app creation if needed
//	// fiber uses internal logging, adapter can be used in custom middleware
//	go app.Listen(":3000")