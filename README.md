# Log

A high-performance, buffered, rotating file logger for Go applications, configured via
the [LixenWraith/config](https://github.com/LixenWraith/config) package or simple overrides. Designed for
production-grade reliability with features like disk management, log retention, and lock-free asynchronous processing
using atomic operations and channels.

**Note:** This logger requires creating an instance using `NewLogger()` and calling methods on that instance (e.g.,
`l.Info(...)`). It does not use package-level logging functions.

## Features

- **Instance-Based API:** Create logger instances via `NewLogger()` and use methods like `l.Info()`, `l.Warn()`, etc.
- **Lock-free Asynchronous Logging:** Non-blocking log operations with minimal application impact. Logs are sent via a
  buffered channel, processed by a dedicated background goroutine. Uses atomic operations for state management, avoiding
  mutexes in the hot path.
- **External Configuration:** Fully configured using `github.com/LixenWraith/config`, supporting both TOML files and CLI
  overrides with centralized management. Also supports simple initialization with defaults and string overrides via
  `InitWithDefaults`.
- **Automatic File Rotation:** Seamlessly rotates log files when they reach configurable size limits (`max_size_mb`),
  generating timestamped filenames.
- **Comprehensive Disk Management:**
  - Monitors total log directory size against configured limits (`max_total_size_mb`)
  - Enforces minimum free disk space requirements (`min_disk_free_mb`)
  - Automatically prunes oldest log files to maintain space constraints
  - Implements recovery behavior when disk space is exhausted
- **Adaptive Resource Monitoring:** Dynamically adjusts disk check frequency based on logging volume (
  `enable_adaptive_interval`, `min_check_interval_ms`, `max_check_interval_ms`), optimizing performance under varying
  loads.
- **Operational Heartbeats:** Multi-level periodic statistics messages (process, disk, system) that bypass level
  filtering to ensure operational monitoring even with higher log levels.
- **Reliable Buffer Management:** Periodic buffer flushing with configurable intervals (`flush_interval_ms`). Detects
  and reports dropped logs during high-volume scenarios.
- **Automated Log Retention:** Time-based log file cleanup with configurable retention periods (`retention_period_hrs`,
  `retention_check_mins`).
- **Structured Logging:** Support for both human-readable text (`txt`) and machine-parseable (`json`) output formats
  with consistent field handling.
- **Comprehensive Log Levels:** Standard severity levels (Debug, Info, Warn, Error) with numeric values compatible with
  other logging systems.
- **Function Call Tracing:** Optional function call stack traces with configurable depth (`trace_depth`) for debugging
  complex execution flows.
- **Clean API Design:** Straightforward logging methods on the logger instance that don't require `context.Context`
  parameters.
- **Graceful Shutdown:** Managed termination with best-effort flushing to minimize log data loss during application
  shutdown.

## Installation

```bash
go get github.com/LixenWraith/log
go get github.com/LixenWraith/config
```

## Basic Usage

This example shows minimal initialization using defaults with a single override, logging one message, and shutting down.

```go
package main

import (
    "github.com/LixenWraith/log"
)

func main() {
    logger := log.NewLogger()
    _ = logger.InitWithDefaults("directory=/var/log/myapp")
    logger.Info("Application starting", "pid", 12345)
    _ = logger.Shutdown()
}
```

## Configuration

The `log` package can be configured in two ways:

1. Using a `config.Config` instance with the `Init` method
2. Using simple string overrides with the `InitWithDefaults` method

### Configuration via Init

When using `(l *Logger) Init(cfg *config.Config, basePath string)`, the `basePath` argument defines the prefix for all configuration keys in the `config.Config` instance. For example, if `basePath` is set to `"logging"`, the logger will look for configuration keys like `"logging.level"`, `"logging.name"`, etc. This allows embedding logger configuration within a larger application configuration.

```go
// Initialize config
cfg := config.New()
configExists, err := cfg.Load("app_config.toml", os.Args[1:])
if err != nil {
    // Handle error
}

// Initialize logger with config and path prefix
logger := log.NewLogger()
err = logger.Init(cfg, "logging") // Look for keys under "logging."
if err != nil {
    // Handle error
}
```

Example TOML with `basePath = "logging"`:
```toml
[logging]  # This matches the basePath
level = -4 # Debug
directory = "/var/log/my_service"
format = "json"
```

### Configuration via InitWithDefaults

When using `(l *Logger) InitWithDefaults(overrides ...string)`, the configuration keys are provided directly without a prefix, e.g., `"level=-4"`, `"directory=/var/log/my_service"`.

```go
// Simple initialization with specific overrides
logger := log.NewLogger()
err := logger.InitWithDefaults("directory=/var/log/app", "level=-4")
if err != nil {
    // Handle error
}
```

### Configuration Parameters

| Key (`basePath` + Key)     | Type      | Description                                                               | Default Value |
|:---------------------------|:----------|:--------------------------------------------------------------------------|:--------------|
| `level`                    | `int64`   | Minimum log level (-4=Debug, 0=Info, 4=Warn, 8=Error)                     | `0` (Info)    |
| `name`                     | `string`  | Base name for log files                                                   | `"log"`       |
| `directory`                | `string`  | Directory to store log files                                              | `"./logs"`    |
| `format`                   | `string`  | Log file format (`"txt"`, `"json"`)                                       | `"txt"`       |
| `extension`                | `string`  | Log file extension (without dot)                                          | `"log"`       |
| `show_timestamp`           | `bool`    | Show timestamp in log entries                                             | `true`        |
| `show_level`               | `bool`    | Show log level in entries                                                 | `true`        |
| `buffer_size`              | `int64`   | Channel buffer capacity for log records                                   | `1024`        |
| `max_size_mb`              | `int64`   | Max size (MB) per log file before rotation                                | `10`          |
| `max_total_size_mb`        | `int64`   | Max total size (MB) of log directory (0=unlimited)                        | `50`          |
| `min_disk_free_mb`         | `int64`   | Min required free disk space (MB) (0=unlimited)                           | `100`         |
| `flush_interval_ms`        | `int64`   | Interval (ms) to force flush buffer to disk via timer                     | `100`         |
| `trace_depth`              | `int64`   | Function call trace depth (0=disabled, 1-10)                              | `0`           |
| `retention_period_hrs`     | `float64` | Hours to keep log files (0=disabled)                                      | `0.0`         |
| `retention_check_mins`     | `float64` | Minutes between retention checks via timer (if enabled)                   | `60.0`        |
| `disk_check_interval_ms`   | `int64`   | Base interval (ms) for periodic disk space checks via timer               | `5000`        |
| `enable_adaptive_interval` | `bool`    | Adjust disk check interval based on load (within min/max bounds)          | `true`        |
| `enable_periodic_sync`     | `bool`    | Periodic sync with disk based on flush interval                           | `true`        |
| `min_check_interval_ms`    | `int64`   | Minimum interval (ms) for adaptive disk checks                            | `100`         |
| `max_check_interval_ms`    | `int64`   | Maximum interval (ms) for adaptive disk checks                            | `60000`       |
| `heartbeat_level`          | `int64`   | Heartbeat detail level (0=disabled, 1=proc, 2=proc+disk, 3=proc+disk+sys) | `0`           |
| `heartbeat_interval_s`     | `int64`   | Interval (s) between heartbeat messages                                   | `60`          |

## API Reference

**Note:** All logging and control functions are methods on a `*Logger` instance obtained via `NewLogger()`.

### Creation

- **`NewLogger() *Logger`**
  Creates a new, uninitialized logger instance with default configuration parameters registered internally.

### Initialization

- **`(l *Logger) Init(cfg *config.Config, basePath string) error`**
  Initializes the logger instance `l` using settings from the provided `config.Config` instance under `basePath`. Starts
  the background processing goroutine.
- **`(l *Logger) InitWithDefaults(overrides ...string) error`**
  Initializes the logger instance `l` using built-in defaults, applying optional overrides provided as "key=value"
  strings (e.g., `"directory=/tmp/logs"`). Starts the background processing goroutine.

### Logging Functions

These methods accept `...any` arguments, typically used as key-value pairs for structured logging. They are called on an
initialized `*Logger` instance (e.g., `l.Info(...)`).

- **`(l *Logger) Debug(args ...any)`**: Logs at Debug level (-4).
- **`(l *Logger) Info(args ...any)`**: Logs at Info level (0).
- **`(l *Logger) Warn(args ...any)`**: Logs at Warn level (4).
- **`(l *Logger) Error(args ...any)`**: Logs at Error level (8).

### Trace Logging Functions

Temporarily enable function call tracing for a single log entry on an initialized `*Logger` instance.

- **`(l *Logger) DebugTrace(depth int, args ...any)`**: Logs Debug with trace.
- **`(l *Logger) InfoTrace(depth int, args ...any)`**: Logs Info with trace.
- **`(l *Logger) WarnTrace(depth int, args ...any)`**: Logs Warn with trace.
- **`(l *Logger) ErrorTrace(depth int, args ...any)`**: Logs Error with trace.
  (`depth` specifies the number of stack frames, 0-10).

### Other Logging Variants

Called on an initialized `*Logger` instance.

- **`(l *Logger) Log(args ...any)`**: Logs with timestamp only, no level (uses Info internally).
- **`(l *Logger) Message(args ...any)`**: Logs raw message without timestamp or level.
- **`(l *Logger) LogTrace(depth int, args ...any)`**: Logs with timestamp and trace, no level.

### Shutdown and Control

Called on an initialized `*Logger` instance.

- **`(l *Logger) Shutdown(timeout time.Duration) error`**
  Gracefully shuts down the logger instance `l`. Signals the processor to stop, waits briefly for pending logs to flush,
  then closes file handles.

- **`(l *Logger) Flush(timeout time.Duration) error`**
  Explicitly triggers a sync of the current log file buffer to disk for instance `l` and waits for completion or
  timeout.

### Constants

- **`LevelDebug (-4)`, `LevelInfo (0)`, `LevelWarn (4)`, `LevelError (8)` (`int64`)**: Standard log level constants.
- **`LevelProc (12)`, `LevelDisk (16)`, `LevelSys (20)` (`int64`)**: Heartbeat log level constants. These levels bypass
  the configured `level` filter.
- **`FlagShowTimestamp`, `FlagShowLevel`, `FlagDefault`**: Record flag constants controlling output format.

## Implementation Details

- **Lock-Free Hot Path:** Logging methods (`l.Info`, `l.Debug`, etc.) operate without locks, using atomic operations to
  check logger state and non-blocking channel sends. Only initialization, reconfiguration, and shutdown use a mutex
  internally.

- **Channel-Based Architecture:** Log records flow through a buffered channel from producer methods to a single consumer
  goroutine per logger instance, preventing contention and serializing file I/O operations.

- **Adaptive Resource Management:**
  - Disk checks run periodically via timer and reactively when write volume thresholds are crossed.
  - Check frequency automatically adjusts based on logging rate when `enable_adaptive_interval` is enabled.

- **Heartbeat Messages:**
  - Periodic operational statistics that bypass log level filtering.
  - Three levels of detail (`heartbeat_level`):
    - Level 1 (PROC): Logger metrics (uptime, processed/dropped logs)
    - Level 2 (DISK): Adds disk metrics (rotations, deletions, file counts, sizes)
    - Level 3 (SYS): Adds system metrics (memory usage, goroutine count, GC stats)
  - Ensures monitoring data is available regardless of the configured `level`.

- **File Management:**
  - Log files are rotated when `max_size_mb` is exceeded.
  - Oldest files are automatically pruned when space limits (`max_total_size_mb`, `min_disk_free_mb`) are approached.
  - Files older than `retention_period_hrs` are periodically removed.

- **Recovery Behavior:** When disk issues occur, the logger temporarily pauses new logs and attempts recovery on
  subsequent operations, logging one disk warning message to prevent error spam.

- **Graceful Shutdown Flow:**
  1. Sets atomic flags to prevent new logs on the specific instance.
  2. Closes the active log channel to signal processor shutdown for that instance.
  3. Waits briefly for the processor to finish pending records.
  4. Performs final sync and closes the file handle.

## Performance Considerations

- **Non-blocking Design:** The logger is designed to have minimal impact on application performance, with non-blocking
  log operations and buffered processing.

- **Memory Efficiency:** Uses a reusable buffer (`serializer`) per instance for serialization, avoiding unnecessary
  allocations when formatting log entries.

- **Disk I/O Management:** Batches writes and intelligently schedules disk operations to minimize I/O overhead while
  maintaining data safety.

- **Concurrent Safety:** Thread-safe through careful use of atomic operations and channel-based processing, minimizing
  mutex usage to initialization and shutdown paths only. Multiple `*Logger` instances operate independently.

## Heartbeat Usage Example

Heartbeats provide periodic operational statistics even when using higher log levels:

```go
// Enable all heartbeat types with 30-second interval
logger := log.NewLogger()
err := logger.InitWithDefaults(
    "level=4",                // Only show Warn and above for normal logs
    "heartbeat_level=3",      // Enable all heartbeat types 
    "heartbeat_interval_s=30" // 30-second interval
)

// The PROC, DISK, and SYS heartbeat messages will appear every 30 seconds
// even though regular Debug and Info logs are filtered out
```

## Caveats & Limitations

- **Log Loss Scenarios:**
  - **Buffer Saturation:** Under extreme load, logs may be dropped if the internal buffer fills faster than records
    can be processed by the background goroutine. A summary message will be logged once capacity is available again.
  - **Shutdown Race:** The `Shutdown` function provides a best-effort attempt to process remaining logs, but cannot
    guarantee all buffered logs will be written if the application terminates abruptly or the timeout is too short.
  - **Persistent Disk Issues:** If disk space cannot be reclaimed through cleanup, logs will be dropped until the
    condition is resolved.

- **Configuration Dependencies:**
  For full configuration management (TOML file loading, CLI overrides, etc.), the `github.com/LixenWraith/config` package is required when using the `Init` method. For simpler initialization without this external dependency, use `InitWithDefaults`.

- **Retention Accuracy:** Log retention relies on file modification times, which could potentially be affected by
  external file system operations.

## License

BSD-3-Clause