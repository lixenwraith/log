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

// BuildFiber creates a Fiber v2 adapter
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

// Configure and Start the shared Logger before building adapters. WithConfig
// initializes a new Logger but does not Start it; use GetLogger for lifecycle
// control. See doc/adapters.md for framework integration examples.
