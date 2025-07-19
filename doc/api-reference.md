# API Reference

Complete API documentation for the lixenwraith/log package.

## Logger Creation

### NewLogger

```go
func NewLogger() *Logger
```

Creates a new, uninitialized logger instance with default configuration parameters registered internally.

**Example:**
```go
logger := log.NewLogger()
```

## Initialization Methods

### ApplyConfig

```go
func (l *Logger) ApplyConfig(cfg *Config) error
```

Applies a validated configuration to the logger. This is the recommended method for applications that need full control over configuration.

**Parameters:**
- `cfg`: A `*Config` struct with desired settings

**Returns:**
- `error`: Configuration error if invalid

**Example:**
```go
logger := log.NewLogger()

cfg := log.GetConfig()
cfg.Level = log.LevelDebug
cfg.Directory = "/var/log/app"
err := logger.ApplyConfig(cfg)
```

### ApplyOverride

```go
func (l *Logger) ApplyOverride(overrides ...string) error
```

Applies key-value overrides to the logger. Convenient interface for minor changes.

**Parameters:**
- `overrides`: Variadic overrides in the format "key=value"

**Returns:**
- `error`: Configuration error if invalid

**Example:**
```go
logger := log.NewLogger()

err := logger.ApplyOverride("directory=/var/log/app", "name=app")
```

## Logging Methods

All logging methods accept variadic arguments, typically used as key-value pairs for structured logging.

### Debug

```go
func (l *Logger) Debug(args ...any)
```

Logs a message at debug level (-4).

**Example:**
```go
logger.Debug("Processing started", "items", 100, "mode", "batch")
```

### Info

```go
func (l *Logger) Info(args ...any)
```

Logs a message at info level (0).

**Example:**
```go
logger.Info("Server started", "port", 8080, "tls", true)
```

### Warn

```go
func (l *Logger) Warn(args ...any)
```

Logs a message at warning level (4).

**Example:**
```go
logger.Warn("High memory usage", "used_mb", 1800, "limit_mb", 2048)
```

### Error

```go
func (l *Logger) Error(args ...any)
```

Logs a message at error level (8).

**Example:**
```go
logger.Error("Database connection failed", "host", "db.example.com", "error", err)
```

### LogStructured

```go
func (l *Logger) LogStructured(level int64, message string, fields map[string]any)
```

Logs a message with structured fields as proper JSON (when format="json").

**Example:**
```go
logger.LogStructured(log.LevelInfo, "User action", map[string]any{
    "user_id": 42,
    "action": "login",
    "metadata": map[string]any{"ip": "192.168.1.1"},
     })
```

### Write

```go
func (l *Logger) Write(args ...any)
```

Outputs raw, unformatted data regardless of configured format. Bypasses all formatting (timestamps, levels, JSON structure) and writes args as space-separated strings without a trailing newline.

**Example:**
```go
logger.Write("METRIC", "cpu_usage", 85.5, "timestamp", 1234567890)
// Output: METRIC cpu_usage 85.5 timestamp 1234567890
```

## Trace Logging Methods

These methods include function call traces in the log output.

### DebugTrace

```go
func (l *Logger) DebugTrace(depth int, args ...any)
```

Logs at debug level with function call trace.

**Parameters:**
- `depth`: Number of stack frames to include (0-10)
- `args`: Log message and fields

**Example:**
```go
logger.DebugTrace(3, "Entering critical section", "mutex", "db_lock")
```

### InfoTrace

```go
func (l *Logger) InfoTrace(depth int, args ...any)
```

Logs at info level with function call trace.

### WarnTrace

```go
func (l *Logger) WarnTrace(depth int, args ...any)
```

Logs at warning level with function call trace.

### ErrorTrace

```go
func (l *Logger) ErrorTrace(depth int, args ...any)
```

Logs at error level with function call trace.

## Special Logging Methods

### Log

```go
func (l *Logger) Log(args ...any)
```

Logs with timestamp only, no level information (uses Info level internally).

**Example:**
```go
logger.Log("Checkpoint reached", "step", 5)
```

### Message

```go
func (l *Logger) Message(args ...any)
```

Logs raw message without timestamp or level.

**Example:**
```go
logger.Message("Raw output for special formatting")
```

### LogTrace

```go
func (l *Logger) LogTrace(depth int, args ...any)
```

Logs with timestamp and trace, but no level information.

**Example:**
```go
logger.LogTrace(2, "Function boundary", "entering", true)
```

## Control Methods

### Shutdown

```go
func (l *Logger) Shutdown(timeout ...time.Duration) error
```

Gracefully shuts down the logger, attempting to flush pending logs.

**Parameters:**
- `timeout`: Optional timeout duration (defaults to 2x flush interval)

**Returns:**
- `error`: Shutdown error if flush fails or timeout exceeded

**Example:**
```go
err := logger.Shutdown(5 * time.Second)
if err != nil {
    fmt.Printf("Shutdown error: %v\n", err)
}
```

### Flush

```go
func (l *Logger) Flush(timeout time.Duration) error
```

Explicitly triggers a sync of the current log file buffer to disk.

**Parameters:**
- `timeout`: Maximum time to wait for flush completion

**Returns:**
- `error`: Flush error if timeout exceeded

**Example:**
```go
err := logger.Flush(1 * time.Second)
```

## Constants

### Log Levels

```go
const (
    LevelDebug int64 = -4
    LevelInfo  int64 = 0
    LevelWarn  int64 = 4
    LevelError int64 = 8
)
```

Standard log levels for filtering output.

### Heartbeat Levels

```go
const (
    LevelProc int64 = 12  // Process statistics
    LevelDisk int64 = 16  // Disk usage statistics
    LevelSys  int64 = 20  // System statistics
)
```

Special levels for heartbeat monitoring that bypass level filtering.

### Level Helper Function

```go
func Level(levelStr string) (int64, error)
```

Converts level string to numeric constant.

**Parameters:**
- `levelStr`: Level name ("debug", "info", "warn", "error", "proc", "disk", "sys")

**Returns:**
- `int64`: Numeric level value
- `error`: Conversion error for invalid strings

**Example:**
```go
level, err := log.Level("debug")  // Returns -4
```

## Error Types

The logger returns errors prefixed with "log: " for easy identification:

```go
// Configuration errors
"log: invalid format: 'xml' (use txt, json, or raw)"
"log: buffer_size must be positive: 0"

// Initialization errors
"log: failed to create log directory '/var/log/app': permission denied"
"log: logger previously failed to initialize and is disabled"

// Runtime errors
"log: logger not initialized or already shut down"
"log: timeout waiting for flush confirmation (1s)"
```

## Thread Safety

All public methods are thread-safe and can be called concurrently from multiple goroutines. The logger uses atomic operations and channels to ensure safe concurrent access without locks in the critical path.

### Usage Pattern Example

```go
type Service struct {
    logger *log.Logger
}

func NewService() (*Service, error) {
    logger := log.NewLogger()
    err := logger.ApplyOverride(
        "directory=/var/log/service",
        "format=json",
        "buffer_size=2048",
        "heartbeat_level=1")
    if err != nil {
        return nil, fmt.Errorf("logger init: %w", err)
    }
    
    return &Service{logger: logger}, nil
}

func (s *Service) ProcessRequest(id string) error {
    s.logger.InfoTrace(1, "Processing request", "id", id)
    
    if err := s.doWork(id); err != nil {
        s.logger.Error("Request failed", "id", id, "error", err)
        return err
    }
    
    s.logger.Info("Request completed", "id", id)
    return nil
}

func (s *Service) Shutdown() error {
    return s.logger.Shutdown(5 * time.Second)
}
```

---

[← Configuration Builder](config-builder.md) | [← Back to README](../README.md) | [Logging Guide →](logging-guide.md)