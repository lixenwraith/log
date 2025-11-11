# Log

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)
[![Documentation](https://img.shields.io/badge/Docs-Available-green.svg)](doc/)

A high-performance, buffered, rotating file logger for Go applications with built-in disk management, operational monitoring, and framework compatibility adapters.

## Key Features

- **Lock-free async logging** with minimal application impact
- **Automatic file rotation** and disk space management
- **Operational heartbeats** for production monitoring
- **Hot reconfiguration** without data loss
- **Framework adapters** for gnet v2 and fasthttp
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
    err := logger.ApplyConfigString("directory=/var/log/myapp")
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

For configuration management support:
```bash
go get github.com/lixenwraith/config
```

## Documentation

- **[Getting Started](doc/getting-started.md)** - Installation and basic usage
- **[Configuration Guide](doc/configuration.md)** - Configuration options
- **[Configuration Builder](doc/builder.md)** - Builder pattern guide
- **[API Reference](doc/api.md)** - Complete API documentation
- **[Logging Guide](doc/logging.md)** - Logging methods and best practices
- **[Disk Management](doc/storage.md)** - File rotation and cleanup
- **[Heartbeat Monitoring](doc/heartbeat.md)** - Operational statistics
- **[Compatibility Adapters](doc/adapters.md)** - Framework integrations
- **[LLM Guide](doc/llm-guide_lixenwraith_log.md)** - Guide for LLM usage without full codebase

## Architecture Overview

The logger uses a lock-free, channel-based architecture for high performance:

```
Application → Log Methods → Buffered Channel → Background Processor → File/Console
                ↓                                      ↓
            (non-blocking)                    (rotation, cleanup, monitoring)
```

## Contributing

Contributions and suggestions are welcome!
There is no contribution policy, but if interested, please submit pull requests to the repository.
Submit suggestions or issues at [issue tracker](https://github.com/lixenwraith/log/issues).

## License

BSD-3-Clause