# Log

A high-performance, buffered, rotating file logger for Go applications, configured via the [LixenWraith/config](https://github.com/LixenWraith/config) package. Designed for production-grade reliability with features like disk management, log retention, and lock-free asynchronous processing using atomic operations and channels.

## Features

-   **Lock-free Asynchronous Logging:** Non-blocking log operations with minimal application impact. Logs are sent via a buffered channel, processed by a dedicated background goroutine. Uses atomic operations for state management, avoiding mutexes in the hot path.
-   **External Configuration:** Fully configured using `github.com/LixenWraith/config`, supporting both TOML files and CLI overrides with centralized management.
-   **Automatic File Rotation:** Seamlessly rotates log files when they reach configurable size limits (`max_size_mb`), generating timestamped filenames.
-   **Comprehensive Disk Management:**
    -   Monitors total log directory size against configured limits (`max_total_size_mb`)
    -   Enforces minimum free disk space requirements (`min_disk_free_mb`)
    -   Automatically prunes oldest log files to maintain space constraints
    -   Implements recovery behavior when disk space is exhausted
-   **Adaptive Resource Monitoring:** Dynamically adjusts disk check frequency based on logging volume (`enable_adaptive_interval`, `min_check_interval_ms`, `max_check_interval_ms`), optimizing performance under varying loads.
-   **Reliable Buffer Management:** Periodic buffer flushing with configurable intervals (`flush_interval_ms`). Detects and reports dropped logs during high-volume scenarios.
-   **Automated Log Retention:** Time-based log file cleanup with configurable retention periods (`retention_period_hrs`, `retention_check_mins`).
-   **Structured Logging:** Support for both human-readable text (`txt`) and machine-parseable (`json`) output formats with consistent field handling.
-   **Comprehensive Log Levels:** Standard severity levels (Debug, Info, Warn, Error) with numeric values compatible with other logging systems.
-   **Function Call Tracing:** Optional function call stack traces with configurable depth (`trace_depth`) for debugging complex execution flows.
-   **Clean API Design:** Straightforward logging methods that don't require `context.Context` parameters.
-   **Graceful Shutdown:** Managed termination with best-effort flushing to minimize log data loss during application shutdown.

## Installation

```bash
go get github.com/LixenWraith/log
go get github.com/LixenWraith/config
```

## Basic Usage

```go
package main

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/LixenWraith/config" // External config package
	"github.com/LixenWraith/log"    // This logger package
)

const configFile = "app_config.toml"
const logConfigPath = "logging" // Base path for logger settings in TOML/config

// Example app_config.toml content:
/*
[logging]
  level = 0 # Info Level (0)
  directory = "./app_logs"
  format = "json"
  extension = "log"
  max_size_mb = 50
  flush_interval_ms = 100
  disk_check_interval_ms = 5000 # Check disk every 5s
  enable_adaptive_interval = true
  # Other settings will use defaults registered by log.Init
*/

func main() {
	// 1. Initialize the main config manager
	cfg := config.New()

	// Optional: Create a dummy config file if it doesn't exist
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		content := fmt.Sprintf("[%s]\n  level = 0\n  directory = \"./app_logs\"\n", logConfigPath)
		os.WriteFile(configFile, []byte(content), 0644)
	}

	// 2. Load configuration (e.g., from file and/or CLI)
	_, err := cfg.Load(configFile, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load config file '%s': %v. Using defaults.\n", configFile, err)
	}

	// 3. Initialize the logger, passing the config instance and base path.
	// log.Init registers necessary keys (e.g., "logging.level") with cfg.
	err = log.Init(cfg, logConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Logger initialized.")

	// 4. Optionally save the merged config (defaults + file/CLI overrides)
	err = cfg.Save(configFile) // Save back to the file
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to save config: %v\n", err)
	}

	// 5. Use the logger
	log.Info("Application started", "pid", os.Getpid())
	log.Debug("Debugging info", "value", 42) // Might be filtered by level

	// Example concurrent logging
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			log.Info("Goroutine task started", "goroutine_id", id)
			time.Sleep(time.Duration(id*10) * time.Millisecond)
			log.InfoTrace(1, "Goroutine task finished", "goroutine_id", id)
		}(i)
	}
	wg.Wait()

	// ... application logic ...

	// 6. Shutdown the logger gracefully before exit
	fmt.Println("Shutting down...")
	// Shutdown timeout is used internally for a brief wait, not a hard deadline for flushing.
	shutdownTimeout := 2 * time.Second
	err = log.Shutdown(shutdownTimeout) // Pass timeout (used for internal sleep)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Logger shutdown error: %v\n", err)
	}
	fmt.Println("Shutdown complete.")
}
```

## Configuration

The `log` package is configured via keys registered with the `config.Config` instance passed to `log.Init`. `log.Init` expects these keys relative to the `basePath` argument.

| Key (`basePath` + Key)     | Type      | Description                                                      | Default Value |
|:---------------------------| :-------- |:-----------------------------------------------------------------|:--------------|
| `level`                    | `int64`   | Minimum log level (-4=Debug, 0=Info, 4=Warn, 8=Error)            | `0` (Info)    |
| `name`                     | `string`  | Base name for log files                                          | `"log"`       |
| `directory`                | `string`  | Directory to store log files                                     | `"./logs"`    |
| `format`                   | `string`  | Log file format (`"txt"`, `"json"`)                              | `"txt"`       |
| `extension`                | `string`  | Log file extension (without dot)                                 | `"log"`       |
| `show_timestamp`           | `bool`    | Show timestamp in log entries                                    | `true`        |
| `show_level`               | `bool`    | Show log level in entries                                        | `true`        |
| `buffer_size`              | `int64`   | Channel buffer capacity for log records                          | `1024`        |
| `max_size_mb`              | `int64`   | Max size (MB) per log file before rotation                       | `10`          |
| `max_total_size_mb`        | `int64`   | Max total size (MB) of log directory (0=unlimited)               | `50`          |
| `min_disk_free_mb`         | `int64`   | Min required free disk space (MB) (0=unlimited)                  | `100`         |
| `flush_interval_ms`        | `int64`   | Interval (ms) to force flush buffer to disk via timer            | `100`         |
| `trace_depth`              | `int64`   | Function call trace depth (0=disabled, 1-10)                     | `0`           |
| `retention_period_hrs`     | `float64` | Hours to keep log files (0=disabled)                             | `0.0`         |
| `retention_check_mins`     | `float64` | Minutes between retention checks via timer (if enabled)          | `60.0`        |
| `disk_check_interval_ms`   | `int64`   | Base interval (ms) for periodic disk space checks via timer      | `5000`        |
| `enable_adaptive_interval` | `bool`    | Adjust disk check interval based on load (within min/max bounds) | `true`        |
| `enable_periodic_sync`     | `bool`    | Periodic sync with disk based on flush interval                  | `false`       |
| `min_check_interval_ms`    | `int64`   | Minimum interval (ms) for adaptive disk checks                   | `100`         |
| `max_check_interval_ms`    | `int64`   | Maximum interval (ms) for adaptive disk checks                   | `60000`       |

**Example TOML Configuration (`app_config.toml`)**

```toml
# Main application settings
app_name = "My Service"

# Logger settings under the 'logging' base path
[logging]
  level = -4 # Debug
  directory = "/var/log/my_service"
  format = "json"
  extension = "log"
  max_size_mb = 100
  max_total_size_mb = 1024 # 1 GB total
  min_disk_free_mb = 512 # 512 MB free required
  flush_interval_ms = 100
  trace_depth = 2
  retention_period_hrs = 168.0 # 7 days (7 * 24)
  retention_check_mins = 60.0
  disk_check_interval_ms = 10000 # Check disk every 10 seconds
  enable_adaptive_interval = false # Disable adaptive checks

# Other application settings
[database]
  host = "db.example.com"
```

## API Reference

### Initialization

-   **`Init(cfg *config.Config, basePath string) error`**
    Initializes or reconfigures the logger using settings from the provided `config.Config` instance under `basePath`. Registers required keys with defaults if not present. Thread-safe.
-   **`InitWithDefaults(overrides ...string) error`**
    Initializes the logger using built-in defaults, applying optional overrides provided as "key=value" strings. Thread-safe.

### Logging Functions

These methods accept `...any` arguments, typically used as key-value pairs for structured logging (e.g., `"user_id", 123, "status", "active"`). All logging functions are non-blocking and use atomic operations for state checks.

-   **`Debug(args ...any)`**: Logs at Debug level (-4).
-   **`Info(args ...any)`**: Logs at Info level (0).
-   **`Warn(args ...any)`**: Logs at Warn level (4).
-   **`Error(args ...any)`**: Logs at Error level (8).

### Trace Logging Functions

Temporarily enable function call tracing for a single log entry, regardless of the configured `trace_depth`.

-   **`DebugTrace(depth int, args ...any)`**: Logs Debug with trace.
-   **`InfoTrace(depth int, args ...any)`**: Logs Info with trace.
-   **`WarnTrace(depth int, args ...any)`**: Logs Warn with trace.
-   **`ErrorTrace(depth int, args ...any)`**: Logs Error with trace.
    (`depth` specifies the number of stack frames, 0-10).

### Other Logging Variants

-   **`Log(args ...any)`**: Logs with timestamp only, no level (uses Info internally).
-   **`Message(args ...any)`**: Logs raw message without timestamp or level.
-   **`LogTrace(depth int, args ...any)`**: Logs with timestamp and trace, no level.

### Shutdown and Control

-   **`Shutdown(timeout time.Duration) error`**
    Gracefully shuts down the logger. Signals the processor to stop, waits briefly for pending logs to flush, then closes file handles. Returns error details if closing operations fail.

-   **`Flush(timeout time.Duration) error`**
    Explicitly triggers a sync of the current log file buffer to disk and waits for completion or timeout.

### Constants

-   **`LevelDebug (-4)`, `LevelInfo (0)`, `LevelWarn (4)`, `LevelError (8)` (`int64`)**: Log level constants.
-   **`FlagShowTimestamp`, `FlagShowLevel`, `FlagDefault`**: Record flag constants controlling output format.

## Implementation Details

-   **Lock-Free Hot Path:** Log methods (`Info`, `Debug`, etc.) operate without locks, using atomic operations to check logger state and non-blocking channel sends. Only initialization, reconfiguration, and shutdown use a mutex.

-   **Channel-Based Architecture:** Log records flow through a buffered channel from producer methods to a single consumer goroutine, preventing contention and serializing file I/O operations.

-   **Adaptive Resource Management:**
    - Disk checks run periodically via timer and reactively when write volume thresholds are crossed
    - Check frequency automatically adjusts based on logging rate when `enable_adaptive_interval` is enabled
    - Intelligently backs off during low activity and increases responsiveness during high volume

-   **File Management:**
    - Log files are rotated when `max_size_mb` is exceeded, with new files named using timestamps
    - Oldest files (by modification time) are automatically pruned when space limits are approached
    - Files older than `retention_period_hrs` are periodically removed

-   **Recovery Behavior:** When disk issues occur, the logger temporarily pauses new logs and attempts recovery on subsequent operations, logging one disk warning message to prevent error spam.

-   **Graceful Shutdown Flow:**
    1. Sets atomic flags to prevent new logs
    2. Closes the active log channel to signal processor shutdown
    3. Waits briefly for processor to finish pending records
    4. Performs final sync and closes the file handle

## Performance Considerations

-   **Non-blocking Design:** The logger is designed to have minimal impact on application performance, with non-blocking log operations and buffered processing.

-   **Memory Efficiency:** Uses a reusable buffer for serialization, avoiding unnecessary allocations when formatting log entries.

-   **Disk I/O Management:** Batches writes and intelligently schedules disk operations to minimize I/O overhead while maintaining data safety.

-   **Concurrent Safety:** Thread-safe through careful use of atomic operations, minimizing mutex usage to initialization and shutdown paths only.

## Caveats & Limitations

-   **Log Loss Scenarios:**
    -   **Buffer Saturation:** Under extreme load, logs may be dropped if the internal buffer fills faster than records can be processed. A summary message will be logged once capacity is available again.
    -   **Shutdown Race:** The `Shutdown` function provides a best-effort attempt to process remaining logs, but cannot guarantee all buffered logs will be written if the application terminates quickly.
    -   **Persistent Disk Issues:** If disk space cannot be reclaimed through cleanup, logs will be dropped until the condition is resolved.

-   **Configuration Dependencies:** Requires the `github.com/LixenWraith/config` package for advanced configuration management.

-   **Retention Accuracy:** Log retention relies on file modification times, which could be affected by external file system operations.

-   **Reconfiguration Impact:** Changing buffer size during runtime requires restarting the background processor, which may cause a brief period where logs could be dropped.

## License

BSD-3-Clause