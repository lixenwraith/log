# Log

A robust, buffered, rotating file logger for Go applications, configured via the [LixenWraith/config](https://github.com/LixenWraith/config) package. Designed for performance and reliability with features like disk management, log retention, and asynchronous processing using atomic operations and channels.

## Features

-   **Buffered Asynchronous Logging:** Logs are sent non-blockingly to a buffered channel, processed by a dedicated background goroutine for minimal application impact. Uses atomic operations for state management, avoiding mutexes in the logging hot path.
-   **External Configuration:** Fully configured using `github.com/LixenWraith/config`, allowing settings via TOML files and CLI overrides managed centrally.
-   **Automatic File Rotation:** Rotates log files when they reach a configurable size (`max_size_mb`).
-   **Disk Space Management:**
    -   Monitors total log directory size against a limit (`max_total_size_mb`).
    -   Monitors available disk space against a minimum requirement (`min_disk_free_mb`).
    -   Automatically attempts to delete the oldest log files (by modification time) to stay within limits during periodic checks or when writes fail.
    -   Temporarily pauses logging if space cannot be freed, logging an error message.
-   **Adaptive Disk Check Interval:** Optionally adjusts the frequency of disk space checks based on logging load (`enable_adaptive_interval`, `disk_check_interval_ms`, `min_check_interval_ms`, `max_check_interval_ms`) to balance performance and responsiveness.
-   **Periodic Flushing:** Automatically flushes the log buffer to disk at a configured interval (`flush_interval_ms`) using a timer.
-   **Log Retention:** Automatically deletes log files older than a configured duration (`retention_period_hrs`), checked periodically via a timer (`retention_check_mins`). Relies on file modification time.
-   **Dropped Log Detection:** If the internal buffer fills under high load, logs are dropped, and a summary message indicating the number of drops is logged later.
-   **Structured Logging:** Supports both plain text (`txt`) and `json` output formats.
-   **Standard Log Levels:** Provides `Debug`, `Info`, `Warn`, `Error` levels (values match `slog`).
-   **Function Call Tracing:** Optionally include function call traces in logs with configurable depth (`trace_depth`) or enable temporarily via `*Trace` functions.
-   **Simplified API:** Public logging functions (`log.Info`, `log.Debug`, etc.) do not require `context.Context`.
-   **Graceful Shutdown:** `log.Shutdown` signals the background processor to stop by closing the log channel. It then waits for a *brief, fixed duration* (best-effort) before closing the file handle. Note: This is a best-effort flush; logs might be lost if flushing takes longer than the internal wait or if the application exits abruptly.

## Installation

```bash
go get github.com/LixenWraith/log
go get github.com/LixenWraith/config
```

The `config` package has its own dependencies which will be fetched automatically.

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
  disk_check_interval_ms = 5000 # Example: Check disk every 5s
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

| Key (`basePath` + Key)          | Type      | Description                                                          | Default Value (Registered by `log.Init`) |
| :------------------------------ | :-------- | :------------------------------------------------------------------- | :--------------------------------------- |
| `level`                         | `int64`   | Minimum log level (-4=Debug, 0=Info, 4=Warn, 8=Error)                | `0` (LevelInfo)                          |
| `name`                          | `string`  | Base name for log files                                              | `"log"`                                  |
| `directory`                     | `string`  | Directory to store log files                                         | `"./logs"`                               |
| `format`                        | `string`  | Log file format (`"txt"`, `"json"`)                                  | `"txt"`                                  |
| `extension`                     | `string`  | Log file extension (e.g., `"log"`, `"app"`)                          | `"log"`                                  |
| `show_timestamp`                | `bool`    | Show timestamp in log entries                                        | `true`                                   |
| `show_level`                    | `bool`    | Show log level in entries                                            | `true`                                   |
| `buffer_size`                   | `int64`   | Channel buffer capacity for log records                              | `1024`                                   |
| `max_size_mb`                   | `int64`   | Max size (MB) per log file before rotation                           | `10`                                     |
| `max_total_size_mb`             | `int64`   | Max total size (MB) of log directory (0=unlimited)                   | `50`                                     |
| `min_disk_free_mb`              | `int64`   | Min required free disk space (MB) (0=unlimited)                      | `100`                                    |
| `flush_interval_ms`             | `int64`   | Interval (ms) to force flush buffer to disk via timer                | `100`                                    |
| `trace_depth`                   | `int64`   | Function call trace depth (0=disabled, 1-10)                         | `0`                                      |
| `retention_period_hrs`          | `float64` | Hours to keep log files (0=disabled)                                 | `0.0`                                    |
| `retention_check_mins`          | `float64` | Minutes between retention checks via timer (if enabled)              | `60.0`                                   |
| `disk_check_interval_ms`        | `int64`   | Base interval (ms) for periodic disk space checks via timer          | `5000`                                   |
| `enable_adaptive_interval`      | `bool`    | Adjust disk check interval based on load (within min/max bounds)     | `true`                                   |
| `min_check_interval_ms`         | `int64`   | Minimum interval (ms) for adaptive disk checks                       | `100`                                    |
| `max_check_interval_ms`         | `int64`   | Maximum interval (ms) for adaptive disk checks                       | `60000`                                  |

**Example TOML (`config.toml`)**

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

Your application would then initialize the logger like this:

```go
cfg := config.New()
cfg.Load("config.toml", os.Args[1:]) // Load from file & CLI
log.Init(cfg, "logging")             // Use "logging" as base path
cfg.Save("config.toml")              // Save merged config
```

## API Reference

### Initialization

-   **`Init(cfg *config.Config, basePath string) error`**
    Initializes or reconfigures the logger using settings from the provided `config.Config` instance under `basePath`. Registers required keys with defaults if not present. Handles reconfiguration safely, potentially restarting the background processor goroutine (e.g., if `buffer_size` changes). Must be called before logging. Thread-safe.
-   **`InitWithDefaults(overrides ...string) error`**
    Initializes or reconfigures the logger using built-in defaults, applying optional overrides provided as "key=value" strings. Useful for simple setups without a config file. Thread-safe.

### Logging Functions

These functions accept `...any` arguments, typically used as key-value pairs for structured logging (e.g., `"user_id", 123, "status", "active"`). They are non-blocking and read configuration/state using atomic operations.

-   **`Debug(args ...any)`**: Logs at Debug level.
-   **`Info(args ...any)`**: Logs at Info level.
-   **`Warn(args ...any)`**: Logs at Warn level.
-   **`Error(args ...any)`**: Logs at Error level.

### Trace Logging Functions

Temporarily enable function call tracing for a single log entry.

-   **`DebugTrace(depth int, args ...any)`**: Logs Debug with trace.
-   **`InfoTrace(depth int, args ...any)`**: Logs Info with trace.
-   **`WarnTrace(depth int, args ...any)`**: Logs Warn with trace.
-   **`ErrorTrace(depth int, args ...any)`**: Logs Error with trace.
    (`depth` specifies the number of stack frames, 0-10).

### Other Logging Variants

-   **`Log(args ...any)`**: Logs with timestamp, no level (uses Info internally), no trace.
-   **`Message(args ...any)`**: Logs raw message, no timestamp, no level, no trace.
-   **`LogTrace(depth int, args ...any)`**: Logs with timestamp and trace, no level.

### Shutdown

-   **`Shutdown(timeout time.Duration) error`**
    Attempts to gracefully shut down the logger. Sets atomic flags to prevent new logs, closes the internal log channel to signal the background processor, waits for a *brief fixed duration* (currently using the `flush_interval_ms` configuration value, `timeout` argument is used as a default if the interval is <= 0), and then closes the current log file. Returns `nil` on success or an error if file operations fail. Note: This provides a *best-effort* flush; logs might be lost if disk I/O is slow or the application exits too quickly after calling Shutdown.

### Constants

-   **`LevelDebug`, `LevelInfo`, `LevelWarn`, `LevelError` (`int64`)**: Log level constants.

## Implementation Details & Behavior

-   **Asynchronous Processing:** Log calls (`log.Info`, etc.) are non-blocking. They format a `logRecord` and attempt a non-blocking send to an internal buffered channel (`ActiveLogChannel`). A single background goroutine (`processLogs`) reads from this channel, serializes the record (to TXT or JSON using a reusable buffer), and writes it to the current log file.
-   **Configuration Source:** Relies on an initialized `github.com/LixenWraith/config.Config` instance passed to `log.Init` or uses internal defaults with `InitWithDefaults`. It registers expected keys with "log." prefix and retrieves values using the config package's type-specific accessors (Int64, String, Bool, Float64).
-   **State Management:** Uses `sync.Mutex` (`initMu`) *only* to protect initialization and reconfiguration logic. Uses `sync/atomic` variables extensively for runtime state (`IsInitialized`, `CurrentFile`, `CurrentSize`, `DroppedLogs`), allowing lock-free reads in logging functions and the processor loop.
-   **Timers:** Uses `time.Ticker` internally for:
    *   Periodic buffer flushing (`flush_interval_ms`).
    *   Periodic log retention checks (`retention_check_mins`).
    *   Periodic and potentially adaptive disk space checks (`disk_check_interval_ms`, etc.).
-   **File Rotation:** Triggered synchronously within `processLogs` when writing a record would exceed `max_size_mb`. The old file is closed, a new one is created with a timestamped name, and the atomic `CurrentFile` pointer and `CurrentSize` are updated.
-   **Disk/Retention Checks:**
    *   `performDiskCheck` is called periodically by a timer and reactively if writes fail or a byte threshold is crossed. It checks total size and free space limits. If limits are exceeded *and* `forceCleanup` is true (for periodic checks), it calls `cleanOldLogs`. If checks fail, `DiskStatusOK` is set to false, causing subsequent logs to be dropped until the condition resolves.
    *   `cleanOldLogs` deletes the oldest files (by modification time, skipping the current file) until enough space is freed or no more files can be deleted.
    *   `cleanExpiredLogs` is called periodically by a timer based on `retention_check_mins`. It deletes files whose modification time is older than `retention_period_hrs`.
-   **Shutdown Process:**
    1.  `Shutdown` sets atomic flags (`ShutdownCalled`, `LoggerDisabled`) to prevent new logs.
    2.  It closes the current `ActiveLogChannel` (obtained via atomic load).
    3.  It performs a *fixed short sleep* based on the configured `flush_interval_ms` as a best-effort attempt to allow the processor goroutine time to process remaining items in the channel buffer before the file is closed.
    4.  The `processLogs` goroutine detects the closed channel, performs a final file sync, and exits.
    5.  `Shutdown` performs final `Sync` and `Close` on the log file handle after the sleep.

## Limitations, Caveats & Failure Modes

-   **Dependency:** Requires `github.com/LixenWraith/config` for configuration via `log.Init`.
-   **Log Loss Scenarios:**
    -   **Buffer Full:** If the application generates logs faster than they can be written to disk, `ActiveLogChannel` fills up. Subsequent log calls will drop messages until space becomes available. A `"Logs were dropped"` message will be logged later. Increase `buffer_size` or reduce logging volume.
    -   **Shutdown:** The `Shutdown` function uses a brief, fixed wait, not a guarantee that all logs are flushed. Logs remaining in the buffer or OS buffers after `Shutdown` returns might be lost, especially under heavy load or slow disk I/O. Ensure critical logs are flushed before shutdown if necessary (though this logger doesn't provide an explicit flush mechanism).
    -   **Application Exit:** If the application exits abruptly *before* or *during* `log.Shutdown`, buffered logs will likely be lost.
    -   **Disk Full (Unrecoverable):** If `performDiskCheck` detects low space and `cleanOldLogs` *cannot* free enough space (e.g., no old files to delete, permissions issues), `DiskStatusOK` is set to false. Subsequent logs are dropped until the condition resolves. An error message is logged to stderr *once* when this state is entered.
-   **Configuration Errors:** `log.Init` or `InitWithDefaults` will return an error and fail if configuration values are invalid (e.g., negative `max_size_mb`, invalid `format`, bad override string) or if the `config.Config` instance is `nil` (for `Init`). The application must handle these errors.
-   **Cleanup Race Conditions:** Under high load with frequent rotation/cleanup, benign `"failed to remove old log file ... no such file or directory"` errors might appear in stderr if multiple cleanup attempts target the same file.
-   **Retention Accuracy:** Log retention is based on file **modification time**. External actions modifying old log files could interfere with accurate retention.
-   **Reconfiguration:** Changing `buffer_size` restarts the background processor, involving closing the old channel and creating a new one. Logs sent during this brief transition might be dropped. Other configuration changes are applied live where possible via atomic updates.

## License

BSD-3-Clause