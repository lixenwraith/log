# Getting Started

[← Back to README](../README.md) | [Configuration →](configuration.md)

This guide will help you get started with the lixenwraith/log package, from installation through basic usage.

## Table of Contents

- [Installation](#installation)
- [Basic Usage](#basic-usage)
- [Initialization Methods](#initialization-methods)
- [Your First Logger](#your-first-logger)
- [Console Output](#console-output)
- [Next Steps](#next-steps)

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
    // Create a new logger instance
    logger := log.NewLogger()
    
    // Initialize with defaults
    err := logger.InitWithDefaults()
    if err != nil {
        panic(err)
    }
    defer logger.Shutdown()
    
    // Start logging!
    logger.Info("Application started")
    logger.Debug("Debug mode enabled", "verbose", true)
}
```

## Initialization Methods

The logger provides two initialization methods:

### 1. Simple Initialization (Recommended for most cases)

Use `InitWithDefaults` with optional string overrides:

```go
logger := log.NewLogger()
err := logger.InitWithDefaults(
    "directory=/var/log/myapp",
    "level=-4",  // Debug level
    "format=json",
)
```

### 2. Configuration-Based Initialization

For complex applications with centralized configuration:

```go
import (
    "github.com/lixenwraith/config"
    "github.com/lixenwraith/log"
)

// Load configuration
cfg := config.New()
cfg.Load("app.toml", os.Args[1:])

// Initialize logger with config
logger := log.NewLogger()
err := logger.Init(cfg, "logging")  // Uses [logging] section in config
```

## Your First Logger

Here's a complete example demonstrating basic logging features:

```go
package main

import (
    "fmt"
    "time"
	
    "github.com/lixenwraith/log"
)

func main() {
    // Create logger
    logger := log.NewLogger()
    
    // Initialize with custom settings
    err := logger.InitWithDefaults(
        "directory=./logs",     // Log directory
        "name=myapp",          // Log file prefix
        "level=0",             // Info level and above
        "format=txt",          // Human-readable format
        "max_size_mb=10",      // Rotate at 10MB
    )
    if err != nil {
        fmt.Printf("Failed to initialize logger: %v\n", err)
        return
    }
    
    // Always shut down gracefully
    defer func() {
        if err := logger.Shutdown(2 * time.Second); err != nil {
            fmt.Printf("Logger shutdown error: %v\n", err)
        }
    }()
    
    // Log at different levels
    logger.Debug("This won't appear (below Info level)")
    logger.Info("Application started", "pid", 12345)
    logger.Warn("Resource usage high", "cpu", 85.5)
    logger.Error("Failed to connect", "host", "db.example.com", "port", 5432)
    
    // Structured logging with key-value pairs
    logger.Info("User action",
        "user_id", 42,
        "action", "login",
        "ip", "192.168.1.100",
        "timestamp", time.Now(),
    )
}
```

## Console Output

For development or container environments, you might want console output:

```go
// Console-only logging (no files)
logger.InitWithDefaults(
    "enable_stdout=true",
    "disable_file=true",
    "level=-4",  // Debug level
)

// Dual output (both file and console)
logger.InitWithDefaults(
    "directory=/var/log/app",
    "enable_stdout=true",
    "stdout_target=stderr",  // Keep stdout clean
)
```

## Next Steps

Now that you have a working logger:

1. **[Learn about configuration options](configuration.md)** - Customize behavior for your needs
2. **[Explore the API](api-reference.md)** - See all available methods
3. **[Understand logging best practices](logging-guide.md)** - Write better logs
4. **[Check out examples](examples.md)** - See real-world usage patterns

## Common Patterns

### Service Initialization

```go
type Service struct {
    logger *log.Logger
    // other fields...
}

func NewService() (*Service, error) {
    logger := log.NewLogger()
    if err := logger.InitWithDefaults(
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