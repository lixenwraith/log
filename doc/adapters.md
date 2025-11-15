# Compatibility Adapters

Guide to using lixenwraith/log with popular Go networking frameworks through compatibility adapters.

## Overview

The `compat` package provides adapters that allow the lixenwraith/log logger to work seamlessly with:

- **gnet v2**: High-performance event-driven networking framework
- **fasthttp**: Fast HTTP implementation

### Features

- Full interface compatibility
- Preserves structured logging
- Configurable behavior
- Shared logger instances
- Optional field extraction

## gnet Adapter

### Basic Usage

```go
import (
    "github.com/lixenwraith/log"
    "github.com/lixenwraith/log/compat"
    "github.com/panjf2000/gnet/v2"
)

// Create logger
logger := log.NewLogger()
cfg := log.DefaultConfig()
cfg.Directory = "/var/log/gnet"
logger.ApplyConfig(cfg)
defer logger.Shutdown()

// Create adapter
adapter := compat.NewGnetAdapter(logger)

// Use with gnet
gnet.Run(eventHandler, "tcp://127.0.0.1:9000", 
    gnet.WithLogger(adapter),
)
```

### gnet Interface Implementation

The adapter implements all gnet logger methods:

```go
type GnetAdapter struct {
    logger *log.Logger
}

// Methods implemented:
// - Debugf(format string, args ...any)
// - Infof(format string, args ...any)
// - Warnf(format string, args ...any)
// - Errorf(format string, args ...any)
// - Fatalf(format string, args ...any)
```

### Custom Fatal Behavior

Override default fatal handling:

```go
adapter := compat.NewGnetAdapter(logger,
    compat.WithFatalHandler(func(msg string) {
        // Custom cleanup
        saveApplicationState()
        notifyOperations(msg)
        gracefulShutdown()
        os.Exit(1)
    }),
)
```

### Complete gnet Example

```go
type echoServer struct {
    gnet.BuiltinEventEngine
    logger gnet.Logger
}

func (es *echoServer) OnBoot(eng gnet.Engine) gnet.Action {
    es.logger.Infof("Server started on %s", eng.Addrs)
    return gnet.None
}

func (es *echoServer) OnTraffic(c gnet.Conn) gnet.Action {
    buf, _ := c.Next(-1)
    es.logger.Debugf("Received %d bytes from %s", len(buf), c.RemoteAddr())
    c.Write(buf)
    return gnet.None
}

func main() {
    logger := log.NewLogger()
	cfg := log.DefaultConfig()
    cfg.Directory = "/var/log/gnet"
	cfg.Format = "json"
	cfg.BufferSize = 2048
	logger.ApplyConfig(cfg)
    defer logger.Shutdown()
    
    adapter := compat.NewGnetAdapter(logger)
    
    gnet.Run(
        &echoServer{logger: adapter},
        "tcp://127.0.0.1:9000",
        gnet.WithMulticore(true),
        gnet.WithLogger(adapter),
    )
}
```

## fasthttp Adapter

### Basic Usage

```go
import (
    "github.com/lixenwraith/log"
    "github.com/lixenwraith/log/compat"
    "github.com/valyala/fasthttp"
)

// Create logger
logger := log.NewLogger()
cfg := log.DefaultConfig()
cfg.Directory = "/var/log/fasthttp"
logger.ApplyConfig(cfg)
defer logger.Shutdown()

// Create adapter
adapter := compat.NewFastHTTPAdapter(logger)

// Configure server
server := &fasthttp.Server{
    Handler: requestHandler,
    Logger:  adapter,
}
```

### Level Detection

The adapter automatically detects log levels from message content:

```go
// Default detection rules:
// - Contains "error", "failed", "fatal", "panic" → ERROR
// - Contains "warn", "warning", "deprecated" → WARN  
// - Contains "debug", "trace" → DEBUG
// - Otherwise → INFO
```

### Custom Level Detection

```go
adapter := compat.NewFastHTTPAdapter(logger,
    compat.WithDefaultLevel(log.LevelInfo),
    compat.WithLevelDetector(func(msg string) int64 {
        // Custom detection logic
        if strings.Contains(msg, "CRITICAL") {
            return log.LevelError
        }
        if strings.Contains(msg, "performance") {
            return log.LevelWarn
        }
        // Return 0 to use the adapter's default log level (log.LevelInfo by default)
        return 0
    }),
)
```

## Builder Pattern

### Using Existing Logger (Recommended)

Share a configured logger across adapters:

```go
// Create and configure your main logger
logger := log.NewLogger()
cfg := log.DefaultConfig()
cfg.Level = log.LevelDebug
logger.ApplyConfig(cfg)
logger.Start()
defer logger.Shutdown()

// Create builder with existing logger
builder := compat.NewBuilder().WithLogger(logger)

// Build adapters
gnetAdapter, _ := builder.BuildGnet()
if err != nil { return err }

fasthttpAdapter, _ := builder.BuildFastHTTP()
if err != nil { return err }
```

### Creating New Logger

Let the builder create a logger with config:

```go
// Option 1: With custom config
cfg := log.DefaultConfig()
cfg.Directory = "/var/log/app"
builder := compat.NewBuilder().WithConfig(cfg)

// Option 2: Default config (created on first build)
builder := compat.NewBuilder()
if err != nil { return err }

// Build adapters
gnetAdapter, _ := builder.BuildGnet()
logger, _ := builder.GetLogger() // Retrieve for direct use
```

### Structured gnet Adapter

Extract fields from printf-style formats:

```go
structuredAdapter, _ := builder.BuildStructuredGnet()
// "client=%s port=%d" → {"client": "...", "port": ...}
```

## Structured Logging

### Field Extraction

Structured adapters can extract fields from printf-style formats:

```go
// Regular adapter output:
// "client=192.168.1.1 port=8080"

// Structured adapter output:
// {"client": "192.168.1.1", "port": 8080, "source": "gnet"}
```

### Pattern Detection

The structured adapter recognizes patterns like:
- `key=%v`
- `key: %v`
- `key = %v`

```go
adapter := compat.NewStructuredGnetAdapter(logger)

// These will extract structured fields:
adapter.Infof("client=%s port=%d", "192.168.1.1", 8080)
// → {"client": "192.168.1.1", "port": 8080}

adapter.Errorf("user: %s, error: %s", "john", "auth failed")
// → {"user": "john", "error": "auth failed"}

// These remain as messages:
adapter.Infof("Connected to server")
// → {"msg": "Connected to server"}
```

### Integration Examples

#### Microservice with Both Frameworks

```go
type Service struct {
    gnetAdapter     *compat.GnetAdapter
    fasthttpAdapter *compat.FastHTTPAdapter
    logger          *log.Logger
}

func NewService() (*Service, error) {
    // Create and configure logger
    logger := log.NewLogger()
    cfg := log.DefaultConfig()
    cfg.Directory = "/var/log/service"
    cfg.Format = "json"
    cfg.HeartbeatLevel = 2
    if err := logger.ApplyConfig(cfg); err != nil {
        return nil, err
    }
    if err := logger.Start(); err != nil {
        return nil, err
    }

    // Create builder with the logger
    builder := compat.NewBuilder().WithLogger(logger)

    // Build adapters
    gnetAdapter, err := builder.BuildGnet()
    if err != nil {
        logger.Shutdown()
        return nil, err
    }

    fasthttpAdapter, err := builder.BuildFastHTTP()
    if err != nil {
        logger.Shutdown()
        return nil, err
    }

    return &Service{
        gnetAdapter:     gnetAdapter,
        fasthttpAdapter: fasthttpAdapter,
        logger:          logger,
    }, nil
}
```

#### Middleware Integration

```go
// gnet middleware
func loggingMiddleware(adapter *compat.GnetAdapter) gnet.EventHandler {
    return func(c gnet.Conn) gnet.Action {
        start := time.Now()
        addr := c.RemoteAddr()
        
        // Process connection
        action := next(c)
        
        adapter.Infof("conn_duration=%v remote=%s action=%v",
            time.Since(start), addr, action)
        
        return action
    }
}

// fasthttp middleware
func requestLogger(adapter *compat.FastHTTPAdapter) fasthttp.RequestHandler {
    return func(ctx *fasthttp.RequestCtx) {
        start := time.Now()
        
        // Process request
        next(ctx)
        
        // Adapter will detect level from status
        adapter.Printf("method=%s path=%s status=%d duration=%v",
            ctx.Method(), ctx.Path(), 
            ctx.Response.StatusCode(),
            time.Since(start))
    }
}
```