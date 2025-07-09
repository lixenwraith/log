// FILE: examples/fasthttp/main.go
package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/lixenwraith/log"
	"github.com/lixenwraith/log/compat"
	"github.com/valyala/fasthttp"
)

func main() {
	// Create and configure logger
	logger := log.NewLogger()
	err := logger.InitWithDefaults(
		"directory=/var/log/fasthttp",
		"level=0",
		"format=txt",
		"buffer_size=2048",
	)
	if err != nil {
		panic(err)
	}
	defer logger.Shutdown()

	// Create fasthttp adapter with custom level detection
	fasthttpAdapter := compat.NewFastHTTPAdapter(
		logger,
		compat.WithDefaultLevel(log.LevelInfo),
		compat.WithLevelDetector(customLevelDetector),
	)

	// Configure fasthttp server
	server := &fasthttp.Server{
		Handler: requestHandler,
		Logger:  fasthttpAdapter,

		// Other server settings
		Name:              "MyServer",
		Concurrency:       fasthttp.DefaultConcurrency,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
		TCPKeepalive:      true,
		ReduceMemoryUsage: true,
	}

	// Start server
	fmt.Println("Starting server on :8080")
	if err := server.ListenAndServe(":8080"); err != nil {
		panic(err)
	}
}

func requestHandler(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/plain")
	fmt.Fprintf(ctx, "Hello, world! Path: %s\n", ctx.Path())
}

func customLevelDetector(msg string) int64 {
	// Custom logic to detect log levels
	// Can inspect specific fasthttp message patterns

	if strings.Contains(msg, "connection cannot be served") {
		return log.LevelWarn
	}
	if strings.Contains(msg, "error when serving connection") {
		return log.LevelError
	}

	// Use default detection
	return compat.detectLogLevel(msg)
}