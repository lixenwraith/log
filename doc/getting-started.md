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
    "github.com/lixenwraith/log"
)

func main() {
    // Create a new logger instance with default configuration
    // Writes to both console (stdout) and file ./log/log.log
    logger := log.NewLogger()
    defer logger.Shutdown()
	
    // Start logging!
    logger.Info("Application started")
    logger.Debug("Debug mode enabled", "verbose", true)
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

---

[← Back to README](../README.md) | [Configuration →](configuration.md)