# Examples

[← Compatibility Adapters](compatibility-adapters.md) | [← Back to README](../README.md) | [Troubleshooting →](troubleshooting.md)

Sample applications demonstrating various features and use cases of the lixenwraith/log package.

## Table of Contents

- [Example Programs](#example-programs)
- [Running Examples](#running-examples)
- [Simple Example](#simple-example)
- [Stress Test](#stress-test)
- [Heartbeat Monitoring](#heartbeat-monitoring)
- [Reconfiguration](#reconfiguration)
- [Console Output](#console-output)
- [Framework Integration](#framework-integration)

## Example Programs

The `examples/` directory contains several demonstration programs:

| Example | Description | Key Features |
|---------|-------------|--------------|
| `simple` | Basic usage with config management | Configuration, basic logging |
| `stress` | High-volume stress testing | Performance testing, cleanup |
| `heartbeat` | Heartbeat monitoring demo | All heartbeat levels |
| `reconfig` | Dynamic reconfiguration | Hot reload, state management |
| `sink` | Console output configurations | stdout/stderr, dual output |
| `gnet` | gnet framework integration | Event-driven server |
| `fasthttp` | fasthttp framework integration | HTTP server logging |

## Running Examples

### Prerequisites

```bash
# Clone the repository
git clone https://github.com/lixenwraith/log
cd log

# Get dependencies
go mod download
```

### Running Individual Examples

```bash
# Simple example
go run examples/simple/main.go

# Stress test
go run examples/stress/main.go

# Heartbeat demo
go run examples/heartbeat/main.go

# View generated logs
ls -la ./logs/
```

## Simple Example

Demonstrates basic logger usage with configuration management.

### Key Features
- Configuration file creation
- Logger initialization
- Different log levels
- Structured logging
- Graceful shutdown

### Code Highlights

```go
// Initialize with external config
cfg := config.New()
cfg.Load("simple_config.toml", nil)

logger := log.NewLogger()
err := logger.Init(cfg, "logging")

// Log at different levels
logger.Debug("Debug message", "user_id", 123)
logger.Info("Application starting...")
logger.Warn("Warning", "threshold", 0.95)
logger.Error("Error occurred!", "code", 500)

// Save configuration
cfg.Save("simple_config.toml")
```

### What to Observe
- TOML configuration file generation
- Log file creation in `./logs`
- Structured output format
- Proper shutdown sequence

## Stress Test

Tests logger performance under high load.

### Key Features
- Concurrent logging from multiple workers
- Large message generation
- File rotation testing
- Retention policy testing
- Drop detection

### Configuration

```toml
[logstress]
  level = -4
  buffer_size = 500           # Small buffer to test drops
  max_size_mb = 1            # Force frequent rotation
  max_total_size_mb = 20     # Test cleanup
  retention_period_hrs = 0.0028  # ~10 seconds
  retention_check_mins = 0.084   # ~5 seconds
```

### What to Observe
- Log throughput (logs/second)
- File rotation behavior
- Automatic cleanup when limits exceeded
- "Logs were dropped" messages under load
- Memory and CPU usage

### Metrics to Monitor

```bash
# Watch file rotation
watch -n 1 'ls -lh ./logs/ | wc -l'

# Monitor log growth
watch -n 1 'du -sh ./logs/'

# Check for dropped logs
grep "dropped" ./logs/*.log
```

## Heartbeat Monitoring

Demonstrates all heartbeat levels and transitions.

### Test Sequence

1. Heartbeats disabled
2. PROC only (level 1)
3. PROC + DISK (level 2)
4. PROC + DISK + SYS (level 3)
5. Scale down to level 2
6. Scale down to level 1
7. Disable heartbeats

### What to Observe

```
--- Testing heartbeat level 1: PROC heartbeats only ---
2024-01-15T10:30:00Z PROC type="proc" sequence=1 uptime_hours="0.00" processed_logs=40 dropped_logs=0

--- Testing heartbeat level 2: PROC+DISK heartbeats ---
2024-01-15T10:30:05Z PROC type="proc" sequence=2 uptime_hours="0.00" processed_logs=80 dropped_logs=0
2024-01-15T10:30:05Z DISK type="disk" sequence=2 rotated_files=0 deleted_files=0 total_log_size_mb="0.12" log_file_count=1

--- Testing heartbeat level 3: PROC+DISK+SYS heartbeats ---
2024-01-15T10:30:10Z SYS type="sys" sequence=3 alloc_mb="4.23" sys_mb="12.45" num_gc=5 num_goroutine=8
```

### Use Cases
- Understanding heartbeat output
- Testing monitoring integration
- Verifying heartbeat configuration

## Reconfiguration

Tests dynamic logger reconfiguration without data loss.

### Test Scenario

```go
// Rapid reconfiguration loop
for i := 0; i < 10; i++ {
    bufSize := fmt.Sprintf("buffer_size=%d", 100*(i+1))
    err := logger.InitWithDefaults(bufSize)
    time.Sleep(10 * time.Millisecond)
}
```

### What to Observe
- No log loss during reconfiguration
- Smooth transitions between configurations
- File handle management
- Channel recreation

### Verification

```bash
# Check total logs attempted vs written
# Should see minimal/no drops
```

## Console Output

Demonstrates various output configurations.

### Configurations Tested

1. **File Only** (default)
   ```go
   "directory=./temp_logs",
   "name=file_only_log"
   ```

2. **Console Only**
   ```go
   "enable_stdout=true",
   "disable_file=true"
   ```

3. **Dual Output**
   ```go
   "enable_stdout=true",
   "disable_file=false"
   ```

4. **Stderr Output**
   ```go
   "enable_stdout=true",
   "stdout_target=stderr"
   ```

### What to Observe
- Console output appearing immediately
- File creation behavior
- Transition between modes
- Separation of stdout/stderr

## Framework Integration

### gnet Example

High-performance TCP echo server:

```go
type echoServer struct {
    gnet.BuiltinEventEngine
}

func main() {
    logger := log.NewLogger()
    logger.InitWithDefaults(
        "directory=/var/log/gnet",
        "format=json",
    )
    
    adapter := compat.NewGnetAdapter(logger)
    
    gnet.Run(&echoServer{}, "tcp://127.0.0.1:9000",
        gnet.WithLogger(adapter),
    )
}
```

**Test with:**
```bash
# Terminal 1: Run server
go run examples/gnet/main.go

# Terminal 2: Test connection
echo "Hello gnet" | nc localhost 9000
```

### fasthttp Example

HTTP server with custom level detection:

```go
func main() {
    logger := log.NewLogger()
    adapter := compat.NewFastHTTPAdapter(logger,
        compat.WithLevelDetector(customLevelDetector),
    )
    
    server := &fasthttp.Server{
        Handler: requestHandler,
        Logger:  adapter,
    }
    
    server.ListenAndServe(":8080")
}
```

**Test with:**
```bash
# Terminal 1: Run server
go run examples/fasthttp/main.go

# Terminal 2: Send requests
curl http://localhost:8080/
curl http://localhost:8080/test
```

## Creating Your Own Examples

### Template Structure

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
    
    // Initialize with your configuration
    err := logger.InitWithDefaults(
        "directory=./my_logs",
        "level=-4",
        // Add your config...
    )
    if err != nil {
        panic(err)
    }
    
    // Always shut down properly
    defer func() {
        if err := logger.Shutdown(2 * time.Second); err != nil {
            fmt.Printf("Shutdown error: %v\n", err)
        }
    }()
    
    // Your logging logic here
    logger.Info("Example started")
    
    // Test your specific use case
    testYourFeature(logger)
}

func testYourFeature(logger *log.Logger) {
    // Implementation
}
```

### Testing Checklist

When creating examples, test:

- [ ] Configuration loading
- [ ] Log output (file and/or console)
- [ ] Graceful shutdown
- [ ] Error handling
- [ ] Performance characteristics
- [ ] Resource cleanup

---

[← Compatibility Adapters](compatibility-adapters.md) | [← Back to README](../README.md) | [Troubleshooting →](troubleshooting.md)