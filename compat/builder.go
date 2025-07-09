// FILE: compat/builder.go
package compat

import (
	"github.com/lixenwraith/log"
	"github.com/panjf2000/gnet/v2"
	"github.com/valyala/fasthttp"
)

// Builder provides a convenient way to create configured loggers for both frameworks
type Builder struct {
	logger  *log.Logger
	options []string // InitWithDefaults options
}

// NewBuilder creates a new adapter builder
func NewBuilder() *Builder {
	return &Builder{
		logger: log.NewLogger(),
	}
}

// WithOptions adds configuration options for the underlying logger
func (b *Builder) WithOptions(opts ...string) *Builder {
	b.options = append(b.options, opts...)
	return b
}

// Build initializes the logger and returns adapters for both frameworks
func (b *Builder) Build() (*GnetAdapter, *FastHTTPAdapter, error) {
	// Initialize the logger
	if err := b.logger.InitWithDefaults(b.options...); err != nil {
		return nil, nil, err
	}

	// Create adapters
	gnetAdapter := NewGnetAdapter(b.logger)
	fasthttpAdapter := NewFastHTTPAdapter(b.logger)

	return gnetAdapter, fasthttpAdapter, nil
}

// BuildStructured initializes the logger and returns structured adapters
func (b *Builder) BuildStructured() (*StructuredGnetAdapter, *FastHTTPAdapter, error) {
	// Initialize the logger
	if err := b.logger.InitWithDefaults(b.options...); err != nil {
		return nil, nil, err
	}

	// Create adapters
	gnetAdapter := NewStructuredGnetAdapter(b.logger)
	fasthttpAdapter := NewFastHTTPAdapter(b.logger)

	return gnetAdapter, fasthttpAdapter, nil
}

// GetLogger returns the underlying logger for direct access
func (b *Builder) GetLogger() *log.Logger {
	return b.logger
}

// Example usage functions

// ConfigureGnetServer configures a gnet server with the logger
func ConfigureGnetServer(adapter *GnetAdapter, opts ...gnet.Option) []gnet.Option {
	return append(opts, gnet.WithLogger(adapter))
}

// ConfigureFastHTTPServer configures a fasthttp server with the logger
func ConfigureFastHTTPServer(adapter *FastHTTPAdapter, server *fasthttp.Server) {
	server.Logger = adapter
}