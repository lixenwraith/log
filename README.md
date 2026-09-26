# Log

[![Go](https://img.shields.io/badge/Go-1.27.1+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)
[![Documentation](https://img.shields.io/badge/Docs-Available-green.svg)](doc/)

A high-performance, buffered, rotating file logger for Go applications with built-in disk management, operational monitoring, and framework compatibility adapters.

## Key Features

- **Buffered async logging** with minimal application impact
- **Automatic file rotation** and disk space management
- **Operational heartbeats** for production monitoring
- **Hot reconfiguration** with draining on restart
- **Framework adapters** for gnet v2, fasthttp, Fiber v2
- **Production-grade reliability** with graceful shutdown

## Quick Start

```go
package main

import (
	"fmt"
	
    "github.com/lixenwraith/log"
)

func main() {
    // Create and initialize logger
    logger := log.NewLogger()
    err := logger.ApplyConfigString("directory=/var/log/myapp", "enable_file=true", "format=txt")
    if err != nil {
        panic(fmt.Errorf("failed to apply logger config: %w", err))
    }
    defer logger.Shutdown()

    // Start logging
	if err = logger.Start(); err != nil {
        panic(fmt.Errorf("failed to start logger: %w", err))
    }
    logger.Info("Application started", "version", "1.0.0")
    logger.Debug("Debug information", "user_id", 12345)
    logger.Warn("Warning message", "threshold", 0.95)
    logger.Error("Error occurred", "code", 500)
}
```

## Installation

```bash
go get github.com/lixenwraith/log
```

## Documentation

- **[Getting Started](doc/getting-started.md)** - Installation and basic usage
- **[Configuration Guide](doc/configuration.md)** - Configuration options
- **[Configuration Builder](doc/builder.md)** - Builder pattern guide
- **[API Reference](doc/api.md)** - Complete API documentation
- **[Logging Guide](doc/logging.md)** - Logging methods and best practices
- **[Formatting & Sanitization](doc/formatting.md)** - Standalone formatter and sanitizer packages
- **[Disk Management](doc/storage.md)** - File rotation and cleanup
- **[Heartbeat Monitoring](doc/heartbeat.md)** - Operational statistics
- **[Compatibility Adapters](doc/adapters.md)** - Framework integrations
- **[Testing and Performance](doc/testing.md)** - Race, stress, fuzz, and benchmark commands

## Architecture Overview

Applications enqueue records into a bounded channel. A single background processor formats records, writes output, rotates files, and performs cleanup. A short read lock coordinates enqueues with lifecycle changes; producers never wait for queue capacity or output I/O. Full queues drop records and increment counters.

Call `ApplyConfig` (or `Build`) and then `Start` before logging. `Stop` drains accepted records and can be followed by `Start`. `Shutdown` also closes the file; after a timeout, retry it to finish cleanup. See the [API reference](doc/api.md) for ownership and concurrency rules.

## Contributing

Contributions and suggestions are welcome!
There is no contribution policy, but if interested, please submit pull requests to the repository.
Submit suggestions or issues at [issue tracker](https://github.com/lixenwraith/log/issues).

## License

BSD-3-Clause