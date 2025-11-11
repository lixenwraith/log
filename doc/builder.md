# Builder Pattern Guide

The Builder provides a fluent API for constructing and initializing logger instances with compile-time safety and deferred validation.

## Creating a Builder

NewBuilder creates a new builder for constructing a logger instance.
```go
func NewBuilder() *Builder
```
```go
builder := log.NewBuilder()
```

## Builder Methods

All builder methods return `*Builder` for chaining. Errors are accumulated and returned by `Build()`.

### Common Methods

| Method | Parameters | Description |
|--------|------------|-------------|
| `Level(level int64)` | `level`: Numeric log level | Sets log level (-4 to 8) |
| `LevelString(level string)` | `level`: Named level | Sets level by name ("debug", "info", etc.) |
| `Name(name string)` | `name`: Base filename | Sets log file base name |
| `Directory(dir string)` | `dir`: Path | Sets log directory |
| `Format(format string)` | `format`: Output format | Sets format ("txt", "json", "raw") |
| `Extension(ext string)` | `ext`: File extension | Sets log file extension |
| `BufferSize(size int64)` | `size`: Buffer size | Sets channel buffer size |
| `MaxSizeKB(size int64)` | `size`: Size in KB | Sets max file size in KB |
| `MaxSizeMB(size int64)` | `size`: Size in MB | Sets max file size in MB |
| `MaxTotalSizeKB(size int64)` | `size`: Size in KB | Sets max total log directory size in KB |
| `MaxTotalSizeMB(size int64)` | `size`: Size in MB | Sets max total log directory size in MB |
| `MinDiskFreeKB(size int64)` | `size`: Size in KB | Sets minimum required free disk space in KB |
| `MinDiskFreeMB(size int64)` | `size`: Size in MB | Sets minimum required free disk space in MB |
| `EnableConsole(enable bool)` | `enable`: Boolean | Enables console output |
| `EnableFile(enable bool)` | `enable`: Boolean | Enables file output |
| `ConsoleTarget(target string)` | `target`: "stdout"/"stderr" | Sets console output target |
| `ShowTimestamp(show bool)` | `show`: Boolean | Controls timestamp display |
| `ShowLevel(show bool)` | `show`: Boolean | Controls log level display |
| `TimestampFormat(format string)` | `format`: Time format | Sets timestamp format (Go time format) |
| `HeartbeatLevel(level int64)` | `level`: 0-3 | Sets monitoring level (0=off) |
| `HeartbeatIntervalS(interval int64)` | `interval`: Seconds | Sets heartbeat interval |
| `FlushIntervalMs(interval int64)` | `interval`: Milliseconds | Sets buffer flush interval |
| `TraceDepth(depth int64)` | `depth`: 0-10 | Sets default function trace depth |
| `DiskCheckIntervalMs(interval int64)` | `interval`: Milliseconds | Sets disk check interval |
| `EnableAdaptiveInterval(enable bool)` | `enable`: Boolean | Enables adaptive disk check intervals |
| `MinCheckIntervalMs(interval int64)` | `interval`: Milliseconds | Sets minimum adaptive interval |
| `MaxCheckIntervalMs(interval int64)` | `interval`: Milliseconds | Sets maximum adaptive interval |
| `EnablePeriodicSync(enable bool)` | `enable`: Boolean | Enables periodic disk sync |
| `RetentionPeriodHrs(hours float64)` | `hours`: Hours | Sets log retention period |
| `RetentionCheckMins(mins float64)` | `mins`: Minutes | Sets retention check interval |
| `InternalErrorsToStderr(enable bool)` | `enable`: Boolean | Send internal errors to stderr |

## Build
```go
func (b *Builder) Build() (*Logger, error)
```

Creates and initializes a logger instance with the configured settings.
Returns accumulated errors if any builder operations failed.
```go
logger, err := builder.Build()
if err != nil {
    // Handle validation or initialization errors
}
defer logger.Shutdown()
```

## Usage Pattern
```go
// Single-step logger creation and initialization
logger, err := log.NewBuilder().
    Directory("/var/log/app").
    Format("json").
    LevelString("debug").
    Build()

if err != nil { return err }
defer logger.Shutdown()

// Start the logger
err = logger.Start()
if err != nil { return err }

logger.Info("Application started")
```

---
[← Configuration](configuration.md) | [← Back to README](../README.md) | [API Reference →](api-reference.md)