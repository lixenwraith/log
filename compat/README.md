# Compatibility Adapters for lixenwraith/log

This package provides compatibility adapters to use the `github.com/lixenwraith/log` logger with popular Go networking frameworks:

- **gnet v2**: High-performance event-driven networking framework
- **fasthttp**: Fast HTTP implementation

## Features

- ✅ Full interface compatibility with both frameworks
- ✅ Preserves structured logging capabilities
- ✅ Configurable fatal behavior for gnet
- ✅ Automatic log level detection for fasthttp
- ✅ Optional structured field extraction from printf formats
- ✅ Thread-safe and high-performance
- ✅ Shared logger instance for multiple adapters

## Installation

```bash
go get github.com/lixenwraith/log
```

## Quick Start

### Basic Usage with gnet

```go
import (
    "github.com/lixenwraith/log"
    "github.com/lixenwraith/log/compat"
    "github.com/panjf2000/gnet/v2"
)

// Create and configure logger
logger := log.NewLogger()
logger.InitWithDefaults("directory=/var/log/gnet", "level=-4")
defer logger.Shutdown()

// Create gnet adapter
adapter := compat.NewGnetAdapter(logger)

// Use with gnet
gnet.Run(eventHandler, "tcp://127.0.0.1:9000", gnet.WithLogger(adapter))
```

### Basic Usage with fasthttp

```go
import (
    "github.com/lixenwraith/log"
    "github.com/lixenwraith/log/compat"
    "github.com/valyala/fasthttp"
)

// Create and configure logger
logger := log.NewLogger()
logger.InitWithDefaults("directory=/var/log/fasthttp")
defer logger.Shutdown()

// Create fasthttp adapter
adapter := compat.NewFastHTTPAdapter(logger)

// Use with fasthttp
server := &fasthttp.Server{
    Handler: requestHandler,
    Logger:  adapter,
}
server.ListenAndServe(":8080")
```

### Using the Builder Pattern

```go
// Create adapters with shared configuration
builder := compat.NewBuilder().
    WithOptions(
        "directory=/var/log/app",
        "level=0",
        "format=json",
        "max_size_mb=100",
    )

gnetAdapter, fasthttpAdapter, err := builder.Build()
if err != nil {
    panic(err)
}
defer builder.GetLogger().Shutdown()
```

## Advanced Features

### Structured Field Extraction

The structured adapters can extract key-value pairs from printf-style format strings:

```go
// Use structured adapter
adapter := compat.NewStructuredGnetAdapter(logger)

// These calls will extract structured fields:
adapter.Infof("client=%s port=%d", "192.168.1.1", 8080)
// Logs: {"client": "192.168.1.1", "port": 8080, "source": "gnet"}

adapter.Errorf("user: %s, action: %s, error: %s", "john", "login", "invalid password")
// Logs: {"user": "john", "action": "login", "error": "invalid password", "source": "gnet"}
```

### Custom Fatal Handling

```go
adapter := compat.NewGnetAdapter(logger, 
    compat.WithFatalHandler(func(msg string) {
        // Custom cleanup
        saveState()
        notifyAdmin(msg)
        os.Exit(1)
    }),
)
```

### Custom Level Detection for fasthttp

```go
adapter := compat.NewFastHTTPAdapter(logger,
    compat.WithDefaultLevel(log.LevelInfo),
    compat.WithLevelDetector(func(msg string) int64 {
        if strings.Contains(msg, "CRITICAL") {
            return log.LevelError
        }
        return 0 // Use default detection
    }),
)
```

## Configuration Examples

### High-Performance Configuration

```go
builder := compat.NewBuilder().
    WithOptions(
        "directory=/var/log/highperf",
        "level=0",                    // Info and above
        "format=json",                // Structured logs
        "buffer_size=4096",           // Large buffer
        "flush_interval_ms=1000",     // Less frequent flushes
        "enable_periodic_sync=false", // Disable periodic sync
    )
```

### Development Configuration

```go
builder := compat.NewBuilder().
    WithOptions(
        "directory=./logs",
        "level=-4",                  // Debug level
        "format=txt",                // Human-readable
        "show_timestamp=true",
        "show_level=true",
        "trace_depth=3",             // Include call traces
        "flush_interval_ms=100",     // Frequent flushes
    )
```

### Production Configuration with Monitoring

```go
builder := compat.NewBuilder().
    WithOptions(
        "directory=/var/log/prod",
        "level=0",
        "format=json",
        "max_size_mb=1000",          // 1GB files
        "max_total_size_mb=10000",   // 10GB total
        "retention_period_hrs=168",   // 7 days
        "heartbeat_level=2",          // Process + disk heartbeats
        "heartbeat_interval_s=300",   // 5 minutes
    )
```

## Performance Considerations

1. **Printf Overhead**: The adapters must format printf-style strings, adding minimal overhead
2. **Structured Extraction**: The structured adapters use regex matching, which adds ~1-2μs per call
3. **Level Detection**: FastHTTP adapter's level detection adds <100ns for simple string checks
4. **Buffering**: The underlying logger's buffering minimizes I/O impact