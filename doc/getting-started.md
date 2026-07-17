# Getting Started

This guide will help you get started with the lixenwraith/log package, from installation through basic usage.

## Installation

Install the logger package:

```bash
go get github.com/lixenwraith/log
```

For advanced configuration management (optional):

```bash
go get github.com/lixenwraith/config
```

## Basic Usage

The logger follows an instance-based design. You create logger instances and call methods on them:

```go
package main

import (
	"fmt"
    
    "github.com/lixenwraith/log"
)

func main() {
	// Create a new logger instance with default configuration
	logger := log.NewLogger()

	// Apply configuration (enable file output since it's disabled by default)
	err := logger.ApplyConfigString("directory=/var/log/myapp", "enable_file=true")
	if err != nil {
		panic(fmt.Errorf("failed to apply logger config: %w", err))
	}
    defer logger.Shutdown()

	// Start the logger (required before logging)
	if err = logger.Start(); err != nil {
		panic(fmt.Errorf("failed to start logger: %w", err))
	}
    
    // Start logging!
    logger.Info("Application started")
    logger.Debug("Debug mode enabled", "verbose", true)
	logger.Warn("Warning message", "threshold", 0.95)
	logger.Error("Error occurred", "code", 500)
}
```

## Recommended Usage (Builder Pattern)

The Builder pattern provides a fluent API with compile-time safety and deferred validation, making it the most robust way to initialize your logger:

```go
package main

import (
    "fmt"
    "os"
    "time"
    
    "github.com/lixenwraith/log"
)

func main() {
    // Build logger with fluent configuration
    logger, err := log.NewBuilder().
        Directory("/var/log/myapp").       // Log directory path
        LevelString("info").               // Minimum log level
        Format("json").                    // Output format
        Sanitization("json").              // Sanitization policy
        EnableFile(true).                  // Enable file output (disabled by default)
        BufferSize(2048).                  // Channel buffer size
        MaxSizeMB(10).                     // Max file size before rotation
        HeartbeatLevel(1).                 // Enable operational monitoring
        HeartbeatIntervalS(300).           // Every 5 minutes
        Build()                            // Build the logger instance
        
    if err != nil {
        panic(fmt.Errorf("logger build failed: %w", err))
    }
    
    // Ensure logs are flushed on shutdown
    defer logger.Shutdown(5 * time.Second)

    // Start the background async processor (required before logging)
    if err := logger.Start(); err != nil {
        panic(fmt.Errorf("logger start failed: %w", err))
    }

    // Begin logging with structured key-value pairs
    logger.Info("Application started", "version", "1.0.0", "pid", os.Getpid())
}
```

## Next Steps

1. **[Learn about configuration options](configuration.md)** - Customize behavior for your needs
2. **[Explore the API](api-reference.md)** - See all available methods
3. **[Logging patterns and examples](logging-guide.md)** - Write better logs

## Common Patterns

### Service Initialization

```go
type Service struct {
    logger *log.Logger
    // other fields...
}

func NewService() (*Service, error) {
    logger := log.NewLogger()
    if err := logger.ApplyConfigString(
        "directory=/var/log/service",
        "name=service",
        "format=json",
    ); err != nil {
        return nil, fmt.Errorf("logger init failed: %w", err)
    }
	
    logger.Start()
    
    return &Service{
        logger: logger,
    }, nil
}

func (s *Service) Close() error {
    return s.logger.Shutdown(5 * time.Second)
}
```

### HTTP Middleware

```go
func loggingMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            
            // Wrap response writer to capture status
            wrapped := &responseWriter{ResponseWriter: w, status: 200}
            
            next.ServeHTTP(wrapped, r)
            
            logger.Info("HTTP request",
                "method", r.Method,
                "path", r.URL.Path,
                "status", wrapped.status,
                "duration_ms", time.Since(start).Milliseconds(),
                "remote_addr", r.RemoteAddr,
            )
        })
    }
}
```
