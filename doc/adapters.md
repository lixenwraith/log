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

### Simple integration example suite

Below simple client and server examples can be used to test the basic functionality of the adapters. They are not included in the package to avoid dependency creep.


#### gnet server


```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lixenwraith/log"
	"github.com/lixenwraith/log/compat"
	"github.com/panjf2000/gnet/v2"
)

type echoServer struct {
	gnet.BuiltinEventEngine
	adapter *compat.GnetAdapter
}

func (es *echoServer) OnTraffic(c gnet.Conn) gnet.Action {
	buf, _ := c.Next(-1)
	if len(buf) > 0 {
		es.adapter.Infof("Echo %d bytes", len(buf))
		c.Write(buf)
	}
	return gnet.None
}

func main() {
	// Minimal logger config
	logger, err := log.NewBuilder().
		Directory("./logs_gnet").
		Format("json").
		LevelString("info").
		HeartbeatLevel(0).
		Build()
	if err != nil {
		panic(err)
	}

	if err := logger.Start(); err != nil {
		panic(err)
	}

	adapter, err := compat.NewBuilder().WithLogger(logger).BuildGnet()
	if err != nil {
		panic(err)
	}

	handler := &echoServer{adapter: adapter}

	fmt.Println("Starting gnet server on :9000")
	fmt.Println("Press Ctrl+C to stop")

	// Signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := gnet.Run(handler, "tcp://:9000",
			gnet.WithLogger(adapter),
		); err != nil {
			fmt.Printf("gnet error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")
	logger.Shutdown()
}
```

#### fasthttp server


```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/lixenwraith/log"
	"github.com/lixenwraith/log/compat"
	"github.com/valyala/fasthttp"
)

func main() {
	// Minimal logger config
	logger, err := log.NewBuilder().
		Directory("./logs_fasthttp").
		Format("json").
		LevelString("info").
		HeartbeatLevel(0).
		Build()
	if err != nil {
		panic(err)
	}

	if err := logger.Start(); err != nil {
		panic(err)
	}

	adapter, err := compat.NewBuilder().WithLogger(logger).BuildFastHTTP()
	if err != nil {
		panic(err)
	}

	server := &fasthttp.Server{
		Handler: func(ctx *fasthttp.RequestCtx) {
			adapter.Printf("Request: %s %s", ctx.Method(), ctx.Path())
			ctx.WriteString("OK")
		},
		Logger: adapter,
		Name:   "TestServer",
	}

	fmt.Println("Starting FastHTTP server on :8080")
	fmt.Println("Press Ctrl+C to stop")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := server.ListenAndServe(":8080"); err != nil {
			fmt.Printf("FastHTTP error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")
	server.Shutdown()
	logger.Shutdown()
}
```

#### Fiber server


```go
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/lixenwraith/log"
	"github.com/lixenwraith/log/compat"
)

func main() {
	// Minimal logger config
	logger, err := log.NewBuilder().
		Directory("./logs_fiber").
		Format("json").
		LevelString("info").
		HeartbeatLevel(0).
		Build()
	if err != nil {
		panic(err)
	}

	if err := logger.Start(); err != nil {
		panic(err)
	}

	adapter, err := compat.NewBuilder().WithLogger(logger).BuildFiber()
	if err != nil {
		panic(err)
	}

	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(func(c *fiber.Ctx) error {
		adapter.Infow("Request", "method", c.Method(), "path", c.Path())
		return c.Next()
	})

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("OK")
	})

	fmt.Println("Starting Fiber server on :3000")
	fmt.Println("Press Ctrl+C to stop")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := app.Listen(":3000"); err != nil {
			fmt.Printf("Fiber error: %v\n", err)
			os.Exit(1)
		}
	}()

	<-sigChan
	fmt.Println("\nShutting down...")
	app.ShutdownWithTimeout(2 * time.Second)
	logger.Shutdown()
}
```

#### Client

Client for all adapter servers.

```bash
# Run with:
go run client.go -target=gnet
go run client.go -target=fasthttp
go run client.go -target=fiber
```


```go
package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
)

var target = flag.String("target", "fiber", "Target: gnet|fasthttp|fiber")

func main() {
	flag.Parse()

	switch *target {
	case "gnet":
		conn, err := net.Dial("tcp", "localhost:9000")
		if err != nil {
			panic(err)
		}
		conn.Write([]byte("TEST"))
		buf := make([]byte, 4)
		conn.Read(buf)
		conn.Close()
		fmt.Println("gnet: received echo")

	case "fasthttp":
		resp, err := http.Get("http://localhost:8080/")
		if err != nil {
			panic(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("fasthttp: %s\n", body)

	case "fiber":
		resp, err := http.Get("http://localhost:3000/")
		if err != nil {
			panic(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Printf("fiber: %s\n", body)
	}
}
```

