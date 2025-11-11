# lixenwraith/log LLM Usage Guide

This guide details the `lixenwraith/log` package, a high-performance, buffered, rotating file logger for Go with built-in disk management, operational monitoring, and framework compatibility adapters.

## Quick Start: Recommended Usage

The recommended pattern uses the **Builder** with type-safe configuration. This provides compile-time safety and eliminates runtime errors.

```go
package main

import (
	"fmt"
	"time"
	
	"github.com/lixenwraith/log"
)

func main() {
	// 1. Use the builder pattern for configuration (recommended).
	logger, err := log.NewBuilder().
		Directory("/var/log/myapp").     // Log directory path
		LevelString("info").              // Minimum log level
		Format("json").                   // Output format
		BufferSize(2048).                 // Channel buffer size
		MaxSizeMB(10).                    // Max file size before rotation
		HeartbeatLevel(1).                // Enable operational monitoring
		HeartbeatIntervalS(300).          // Every 5 minutes
		Build()                           // Build the logger instance
	if err != nil {
		panic(fmt.Errorf("logger build failed: %w", err))
	}
	defer logger.Shutdown(5 * time.Second)

	// 2. Start the logger (required before logging).
	if err := logger.Start(); err != nil {
		panic(fmt.Errorf("logger start failed: %w", err))
	}

	// 3. Begin logging with structured key-value pairs.
	logger.Info("Application started", "version", "1.0.0", "pid", os.Getpid())
	logger.Debug("Debug information", "user_id", 12345)
	logger.Warn("High memory usage", "used_mb", 1800, "limit_mb", 2048)
	logger.Error("Connection failed", "host", "db.example.com", "error", err)
}
```

## Alternative Initialization Methods

### Using ApplyConfigString (Quick Configuration)

```go
logger := log.NewLogger()
err := logger.ApplyConfigString(
	"directory=/var/log/app",
	"format=json",
	"level=debug",
	"max_size_kb=5000",
)
if err != nil {
	return fmt.Errorf("config failed: %w", err)
}
defer logger.Shutdown()
logger.Start()
```

### Using ApplyConfig (Full Control)

```go
logger := log.NewLogger()
cfg := log.DefaultConfig()
cfg.Directory = "/var/log/app"
cfg.Format = "json"
cfg.Level = log.LevelDebug
cfg.MaxSizeKB = 5000
cfg.HeartbeatLevel = 2  // Process + disk stats
err := logger.ApplyConfig(cfg)
if err != nil {
	return fmt.Errorf("config failed: %w", err)
}
defer logger.Shutdown()
logger.Start()
```

## Builder Pattern

The `Builder` is the primary way to construct a `Logger` instance with compile-time safety.

```go
// NewBuilder creates a new logger builder.
func NewBuilder() *Builder
// Build finalizes configuration and creates the logger.
func (b *Builder) Build() (*Logger, error)
```

### Builder Methods

All builder methods return `*Builder` for chaining.

**Basic Configuration:**
- `Level(level int64)`: Set numeric log level (-4 to 8)
- `LevelString(level string)`: Set level by name ("debug", "info", "warn", "error")
- `Directory(dir string)`: Set log directory path
- `Name(name string)`: Set base filename (default: "log")
- `Format(format string)`: Set format ("txt", "json", "raw")
- `Extension(ext string)`: Set file extension (default: ".log")

**Buffer and Performance:**
- `BufferSize(size int64)`: Channel buffer size (default: 1024)
- `FlushIntervalMs(ms int64)`: Buffer flush interval (default: 100ms)
- `TraceDepth(depth int64)`: Default function trace depth 0-10 (default: 0)

**File Management:**
- `MaxSizeKB(size int64)` / `MaxSizeMB(size int64)`: Max file size before rotation
- `MaxTotalSizeKB(size int64)` / `MaxTotalSizeMB(size int64)`: Max total directory size
- `MinDiskFreeKB(size int64)` / `MinDiskFreeMB(size int64)`: Required free disk space
- `RetentionPeriodHrs(hours float64)`: Hours to keep logs (0=disabled)
- `RetentionCheckMins(mins float64)`: Retention check interval

**Output Control:**
- `EnableConsole(enable bool)`: Enable stdout/stderr output
- `EnableFile(enable bool)`: Enable file output
- `ConsoleTarget(target string)`: "stdout", "stderr", or "split"

**Formatting:**
- `ShowTimestamp(show bool)`: Add timestamps
- `ShowLevel(show bool)`: Add level labels
- `TimestampFormat(format string)`: Go time format string

**Monitoring:**
- `HeartbeatLevel(level int64)`: 0=off, 1=proc, 2=+disk, 3=+sys
- `HeartbeatIntervalS(seconds int64)`: Heartbeat interval

**Disk Monitoring:**
- `DiskCheckIntervalMs(ms int64)`: Base disk check interval
- `EnableAdaptiveInterval(enable bool)`: Adjust interval based on load
- `MinCheckIntervalMs(ms int64)`: Minimum adaptive interval
- `MaxCheckIntervalMs(ms int64)`: Maximum adaptive interval
- `EnablePeriodicSync(enable bool)`: Periodic disk sync

**Error Handling:**
- `InternalErrorsToStderr(enable bool)`: Send internal errors to stderr

## API Reference

### Logger Creation

```go
func NewLogger() *Logger
```
Creates a new uninitialized logger with default configuration.

### Configuration Methods

```go
func (l *Logger) ApplyConfig(cfg *Config) error
func (l *Logger) ApplyConfigString(overrides ...string) error
func (l *Logger) GetConfig() *Config
```

### Lifecycle Methods

```go
func (l *Logger) Start() error                      // Start log processing
func (l *Logger) Stop(timeout ...time.Duration) error    // Stop (can restart)
func (l *Logger) Shutdown(timeout ...time.Duration) error // Terminal shutdown
func (l *Logger) Flush(timeout time.Duration) error      // Force buffer flush
```

### Standard Logging Methods

```go
func (l *Logger) Debug(args ...any)  // Level -4
func (l *Logger) Info(args ...any)   // Level 0
func (l *Logger) Warn(args ...any)   // Level 4
func (l *Logger) Error(args ...any)  // Level 8
```

### Trace Logging Methods

Include function call traces (depth 0-10):

```go
func (l *Logger) DebugTrace(depth int, args ...any)
func (l *Logger) InfoTrace(depth int, args ...any)
func (l *Logger) WarnTrace(depth int, args ...any)
func (l *Logger) ErrorTrace(depth int, args ...any)
```

### Special Logging Methods

```go
func (l *Logger) LogStructured(level int64, message string, fields map[string]any)
func (l *Logger) Write(args ...any)      // Raw output, no formatting
func (l *Logger) Log(args ...any)        // Timestamp only, no level
func (l *Logger) Message(args ...any)    // No timestamp or level
func (l *Logger) LogTrace(depth int, args ...any) // Timestamp + trace, no level
```

## Constants and Levels

### Standard Log Levels

```go
const (
    LevelDebug int64 = -4  // Verbose debugging
    LevelInfo  int64 = 0   // Informational messages
    LevelWarn  int64 = 4   // Warning conditions
    LevelError int64 = 8   // Error conditions
)
```

### Heartbeat Monitoring Levels

Special levels that bypass filtering:

```go
const (
    LevelProc int64 = 12  // Process statistics
    LevelDisk int64 = 16  // Disk usage statistics
    LevelSys  int64 = 20  // System statistics
)
```

### Level Helper

```go
func Level(levelStr string) (int64, error)
```
Converts level string to numeric constant: "debug", "info", "warn", "error", "proc", "disk", "sys".

## Configuration Structure

```go
type Config struct {
    // Output Settings
    EnableConsole bool    // Enable stdout/stderr output
    ConsoleTarget string  // "stdout", "stderr", or "split"
    EnableFile    bool    // Enable file output
    
    // Basic Settings
    Level     int64   // Minimum log level
    Name      string  // Base filename (default: "log")
    Directory string  // Log directory path
    Format    string  // "txt", "json", or "raw"
    Extension string  // File extension (default: ".log")
    
    // Formatting
    ShowTimestamp   bool   // Add timestamps
    ShowLevel       bool   // Add level labels
    TimestampFormat string // Go time format
    
    // Buffer and Performance
    BufferSize      int64 // Channel buffer size
    FlushIntervalMs int64 // Buffer flush interval
    TraceDepth      int64 // Default trace depth (0-10)
    
    // File Management
    MaxSizeKB       int64   // Max file size (KB)
    MaxTotalSizeKB  int64   // Max total directory size (KB)
    MinDiskFreeKB   int64   // Required free disk space (KB)
    RetentionPeriodHrs float64 // Hours to keep logs
    RetentionCheckMins float64 // Retention check interval
    
    // Disk Monitoring
    DiskCheckIntervalMs    int64 // Base check interval
    EnableAdaptiveInterval bool  // Adjust based on load
    MinCheckIntervalMs     int64 // Minimum interval
    MaxCheckIntervalMs     int64 // Maximum interval
    EnablePeriodicSync     bool  // Periodic disk sync
    
    // Heartbeat
    HeartbeatLevel     int64 // 0=off, 1=proc, 2=+disk, 3=+sys
    HeartbeatIntervalS int64 // Heartbeat interval
    
    // Error Handling
    InternalErrorsToStderr bool // Write internal errors to stderr
}
```

### Default Configuration

```go
func DefaultConfig() *Config
```

Returns default configuration with sensible values:
- Console output enabled to stdout
- File output enabled
- Info level logging
- 1MB max file size
- 5MB max total size
- 100ms flush interval

## Output Formats

### Text Format (default)

Human-readable format with optional timestamps and levels:
```
2024-01-15T10:30:00.123456Z INFO Application started version="1.0.0" pid=1234
2024-01-15T10:30:01.456789Z ERROR Connection failed host="db.example.com" error="timeout"
```

### JSON Format

Structured JSON output for log aggregation:
```json
{"time":"2024-01-15T10:30:00.123456Z","level":"INFO","fields":["Application started","version","1.0.0","pid",1234]}
{"time":"2024-01-15T10:30:01.456789Z","level":"ERROR","fields":["Connection failed","host","db.example.com","error","timeout"]}
```

### Raw Format

Minimal format without timestamps or levels:
```
Application started version="1.0.0" pid=1234
Connection failed host="db.example.com" error="timeout"
```

## Framework Adapters (compat package)

### gnet v2 Adapter

```go
import (
    "github.com/lixenwraith/log"
    "github.com/lixenwraith/log/compat"
    "github.com/panjf2000/gnet/v2"
)

// Create adapter
adapter := compat.NewGnetAdapter(logger)

// Use with gnet
gnet.Run(handler, "tcp://127.0.0.1:9000", gnet.WithLogger(adapter))
```

### fasthttp Adapter

```go
import (
    "github.com/lixenwraith/log"
    "github.com/lixenwraith/log/compat"
    "github.com/valyala/fasthttp"
)

// Create adapter
adapter := compat.NewFastHTTPAdapter(logger)

// Use with fasthttp
server := &fasthttp.Server{
    Handler: requestHandler,
    Logger:  adapter,
}
```

### Adapter Builder Pattern

```go
// Share logger across adapters
builder := compat.NewBuilder().WithLogger(logger)

gnetAdapter, err := builder.BuildGnet()
fasthttpAdapter, err := builder.BuildFastHTTP()

// Or create structured adapters
structuredGnet, err := builder.BuildStructuredGnet()
```

## Common Patterns

### Service with Shared Logger

```go
type Service struct {
    logger *log.Logger
}

func NewService() (*Service, error) {
    logger, err := log.NewBuilder().
        Directory("/var/log/service").
        Format("json").
        BufferSize(2048).
        HeartbeatLevel(2).
        Build()
    if err != nil {
        return nil, err
    }
    
    if err := logger.Start(); err != nil {
        return nil, err
    }
    
    return &Service{logger: logger}, nil
}

func (s *Service) Close() error {
    return s.logger.Shutdown(5 * time.Second)
}

func (s *Service) ProcessRequest(id string) {
    s.logger.Info("Processing", "request_id", id)
    // ... process ...
    s.logger.Info("Completed", "request_id", id)
}
```

### HTTP Middleware

```go
func loggingMiddleware(logger *log.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
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

### Hot Reconfiguration

```go
// Initial configuration
logger.ApplyConfigString("level=info")

// Debugging reconfiguration
logger.ApplyConfigString(
    "level=debug",
    "heartbeat_level=3",
    "heartbeat_interval_s=60",
)

// Revert to normal
logger.ApplyConfigString(
    "level=info",
    "heartbeat_level=1",
    "heartbeat_interval_s=300",
)
```

### Graceful Shutdown

```go
// Setup signal handling
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

// Shutdown sequence
<-sigChan
logger.Info("Shutdown initiated")

// Flush pending logs with timeout
if err := logger.Shutdown(5 * time.Second); err != nil {
    fmt.Fprintf(os.Stderr, "Logger shutdown error: %v\n", err)
}
```

## Thread Safety

All public methods are thread-safe. The logger uses:
- Atomic operations for state management
- Channels for log record passing
- No locks in the critical logging path

## Performance Characteristics

- **Zero-allocation logging path**: Uses pre-allocated buffers
- **Lock-free async design**: Non-blocking sends to buffered channel
- **Adaptive disk checks**: Adjusts I/O based on load
- **Batch writes**: Flushes buffer periodically, not per-record
- **Drop tracking**: Counts dropped logs when buffer full

## Migration Guide

### From standard log package

```go
// Before: standard log
log.Printf("User login: id=%d name=%s", id, name)

// After: lixenwraith/log
logger.Info("User login", "id", id, "name", name)
```

### From other structured loggers

```go
// Before: zap
zap.Info("User login",
    zap.Int("id", id),
    zap.String("name", name))

// After: lixenwraith/log
logger.Info("User login", "id", id, "name", name)
```

## Best Practices

1. **Use Builder pattern** for configuration - compile-time safety
2. **Use structured logging** - consistent key-value pairs
3. **Use appropriate levels** - filter noise in logs