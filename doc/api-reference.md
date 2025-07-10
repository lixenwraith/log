# API Reference

[← Configuration](configuration.md) | [← Back to README](../README.md) | [Logging Guide →](logging-guide.md)

Complete API documentation for the lixenwraith/log package.

## Table of Contents

- [Logger Creation](#logger-creation)
- [Initialization Methods](#initialization-methods)
- [Logging Methods](#logging-methods)
- [Trace Logging Methods](#trace-logging-methods)
- [Special Logging Methods](#special-logging-methods)
- [Control Methods](#control-methods)
- [Constants](#constants)
- [Error Types](#error-types)

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

### Init

```go
func (l *Logger) Init(cfg *config.Config, basePath string) error
```

Initializes the logger using settings from a `config.Config` instance.

**Parameters:**
- `cfg`: Configuration instance containing logger settings
- `basePath`: Prefix for configuration keys (e.g., "logging" looks for "logging.level", "logging.directory", etc.)

**Returns:**
- `error`: Initialization error if configuration is invalid

**Example:**
```go
cfg := config.New()
cfg.Load("app.toml", os.Args[1:])
err := logger.Init(cfg, "logging")
```

### InitWithDefaults

```go
func (l *Logger) InitWithDefaults(overrides ...string) error
```

Initializes the logger using built-in defaults with optional overrides.

**Parameters:**
- `overrides`: Variable number of "key=value" strings

**Returns:**
- `error`: Initialization error if overrides are invalid

**Example:**
```go
err := logger.InitWithDefaults(
    "directory=/var/log/app",
    "level=-4",
    "format=json",
)
```

### LoadConfig

```go
func (l *Logger) LoadConfig(path string, args []string) error
```

Loads configuration from a TOML file with CLI overrides.

**Parameters:**
- `path`: Path to TOML configuration file
- `args`: Command-line arguments for overrides

**Returns:**
- `error`: Load or initialization error

**Example:**
```go
err := logger.LoadConfig("config.toml", os.Args[1:])
```

### SaveConfig

```go
func (l *Logger) SaveConfig(path string) error
```

Saves the current logger configuration to a file.

**Parameters:**
- `path`: Path where configuration should be saved

**Returns:**
- `error`: Save error if write fails

**Example:**
```go
err := logger.SaveConfig("current-config.toml")
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

### Format Flags

```go
const (
    FlagShowTimestamp int64 = 0b01
    FlagShowLevel     int64 = 0b10
    FlagDefault             = FlagShowTimestamp | FlagShowLevel
)
```

Flags controlling log entry format.

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
"log: invalid format: 'xml' (use txt or json)"
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

## Usage Examples

### Complete Service Example

```go
type Service struct {
    logger *log.Logger
}

func NewService() (*Service, error) {
    logger := log.NewLogger()
    err := logger.InitWithDefaults(
        "directory=/var/log/service",
        "format=json",
        "buffer_size=2048",
        "heartbeat_level=1",
    )
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

[← Configuration](configuration.md) | [← Back to README](../README.md) | [Logging Guide →](logging-guide.md)