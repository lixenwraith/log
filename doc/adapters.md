# Compatibility adapters

The `compat` package provides logging adapters for gnet v2, fasthttp, and Fiber v2. Framework dependencies are not imported by this module; applications supply them.

## Shared logger setup

Configure and start the logger before passing an adapter to a framework:

```go
logger, err := log.NewBuilder().
    Directory("/var/log/service").
    EnableFile(true).
    Format("json").
    LevelString("info").
    Build()
if err != nil { return err }
if err := logger.Start(); err != nil {
    _ = logger.Shutdown()
    return err
}
defer logger.Shutdown(5 * time.Second)

builder := compat.NewBuilder().WithLogger(logger)
gnetAdapter, err := builder.BuildGnet()
if err != nil { return err }
fastHTTPAdapter, err := builder.BuildFastHTTP()
if err != nil { return err }
fiberAdapter, err := builder.BuildFiber()
if err != nil { return err }
```

Alternatively, `compat.NewBuilder().WithConfig(cfg)` creates an initialized logger on the first build. Retrieve it with `GetLogger()` and call `Start()` explicitly. `WithLogger` takes precedence over `WithConfig`. Builders and option application are single-goroutine operations; completed adapters may be shared with a shared Logger.

## gnet

`GnetAdapter` implements `Debugf`, `Infof`, `Warnf`, `Errorf`, and `Fatalf`. Pass it through `gnet.WithLogger(adapter)`:

```go
adapter := compat.NewGnetAdapter(logger)
err := gnet.Run(eventHandler, "tcp://127.0.0.1:9000", gnet.WithLogger(adapter))
```

Ordinary messages include `msg` and `source` (`gnet`) as positional arguments. With JSON output, these appear in the `fields` array. Use the logger's `LogStructured` or `LogContext` with `FlagKV` directly if a keyed JSON object is required.

### Structured field extraction

`NewStructuredGnetAdapter` extracts simple `key=%v` or `key: %v` expressions with unindexed, single-argument verbs:

```go
adapter := compat.NewStructuredGnetAdapter(logger)
adapter.Infof("client=%s port=%d", "192.168.1.1", 8080)
// JSON fields: ["client", "192.168.1.1", "port", 8080, "source", "gnet"]
```

Text outside the expressions is retained in `msg`. Formats mixing unrelated conversions, argument indexes, width/precision, escaped percent signs, or a different argument count fall back to a complete `fmt.Sprintf` message. Extraction never guesses which argument belongs to a key.

## fasthttp

`FastHTTPAdapter` implements the `Printf` method accepted by `fasthttp.Server.Logger`:

```go
adapter := compat.NewFastHTTPAdapter(logger)
server := &fasthttp.Server{
    Handler: requestHandler,
    Logger: adapter,
}
```

The default detector performs a case-insensitive substring check:

| Message contains | Level |
|---|---|
| `error`, `failed`, `fatal`, or `panic` | Error |
| `warn` or `deprecated` | Warn |
| `debug` or `trace` | Debug |
| Otherwise | Configured default (Info initially) |

This heuristic does not interpret HTTP status numbers. Disable it with `WithLevelDetector(nil)`, or provide a custom function. A detector result of zero selects the configured default; nonzero custom levels are preserved.

```go
adapter := compat.NewFastHTTPAdapter(logger,
    compat.WithDefaultLevel(log.LevelWarn),
    compat.WithLevelDetector(nil),
)
```

## Fiber

`FiberAdapter` provides plain, printf-style (`Infof`), and key/value (`Infow`) logging methods, and implements `io.Writer` through `Write([]byte)`. Trace methods currently map to Debug with a `level`, `trace` marker. Use it directly in Fiber middleware:

```go
adapter := compat.NewFiberAdapter(logger)
app.Use(func(c *fiber.Ctx) error {
    err := c.Next()
    adapter.Infow("request", "method", c.Method(), "path", c.Path(),
        "status", c.Response().StatusCode())
    return err
})
```

## Fatal and panic behavior

Fatal methods enqueue an Error record, attempt a flush for 100 ms, and invoke the configured handler. Defaults call `os.Exit(1)` for fatal and `panic(msg)` for Fiber panic methods. These effects still occur when the log level suppresses the record. A timed-out or failed flush cannot guarantee that the final message was written.

Override behavior with `WithFatalHandler`, `WithFiberFatalHandler`, or `WithFiberPanicHandler`. A nil handler disables that action. Shut down the application/framework before the shared logger so shutdown messages can still be recorded.

### Simple integration example suite

These client and server examples can be used to test the basic functionality of the adapters. They are not included in the package to avoid dependency creep.


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
		EnableFile(true).
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
		EnableFile(true).
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
		EnableFile(true).
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

