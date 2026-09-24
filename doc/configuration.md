# Configuration Guide

This guide covers all configuration options and methods for customizing logger behavior.

## Initialization

log.NewLogger() creates a new instance of logger with DefaultConfig.

```go
logger := log.NewLogger()
```

## Configuration Methods

### ApplyConfig & ApplyConfigString

Direct struct configuration using the Config struct, or key-value overrides:

```go
logger := log.NewLogger()
cfg := log.DefaultConfig()
cfg.Level = log.LevelDebug
cfg.Name = "myapp"
cfg.Directory = "/var/log/myapp"
cfg.EnableFile = true
cfg.Format = "json"
cfg.MaxSizeKB = 100
if err := logger.ApplyConfig(cfg); err != nil { return err }
if err := logger.Start(); err != nil { return err }
defer logger.Shutdown()
logger.Info("application started")

// Apply overrides to the current configuration atomically.
if err := logger.ApplyConfigString("extension=txt", "format=txt"); err != nil {
    return err
}
```

`ApplyConfig` copies the supplied configuration. The caller may reuse or modify it after the call returns. Failed validation, directory creation, or file opening leaves the active configuration unchanged. Changes requiring a restart drain accepted records using the old file and configuration before publishing the replacement. Calls made while emission is stopped are ignored; a full queue drops records.

Names and extensions must be filename components, without path separators or NUL. The queue size must be between 1 and 1,048,576 records. Numeric settings are checked for byte-count and duration overflow; positive fractional retention settings must resolve to at least one nanosecond. Size settings use decimal units: 1 KB = 1,000 bytes and 1 MB = 1,000 KB.

## Configuration Parameters

### Basic Settings

| Parameter | Type | Description | Default    |
|-----------|------|-------------|------------|
| `level` | `int64` | Minimum log level (-8=Trace, -4=Debug, 0=Info, 4=Warn, 8=Error) | `0` |
| `name` | `string` | Base name for log files | `"log"`    |
| `extension` | `string` | Log file extension (without dot) | `"log"` |
| `directory` | `string` | Directory to store log files | `"./log"` |
| `format` | `string` | Output format: `"txt"`, `"json"`, or `"raw"` | `"raw"` |
| `sanitization` | `string` | Sanitization policy: `"raw"`, `"txt"`, `"json"`, or `"shell"` | `"raw"` |
| `timestamp_format` | `string` | Custom timestamp format (Go time format) | `time.RFC3339Nano` |
| `internal_errors_to_stderr` | `bool` | Write logger's internal errors to stderr | `false` |

### Output Control

| Parameter        | Type | Description                                          | Default    |
|------------------|------|------------------------------------------------------|------------|
| `show_timestamp` | `bool` | Include timestamps in log entries                    | `true`     |
| `show_level`     | `bool` | Include log level in entries                         | `true`     |
| `enable_console` | `bool` | Enable console output (stdout/stderr)                | `true`     |
| `console_target` | `string` | Console target: `"stdout"`, `"stderr"`, or `"split"` | `"stderr"` |
| `enable_file`    | `bool` | Enable file output                    | `false`    |

**Note:** When `console_target="split"`, INFO/DEBUG logs go to stdout while WARN/ERROR logs go to stderr.

### Performance Tuning

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `buffer_size` | `int64` | Channel buffer size for log records | `1024` |
| `flush_interval_ms` | `int64` | Buffer flush interval (milliseconds) | `100` |
| `enable_periodic_sync` | `bool` | Enable periodic disk sync | `true` |
| `trace_depth` | `int64` | Default function trace depth (0-10) | `0` |

### File Management

| Parameter | Type | Description | Default |
|-----------|------|-------------|--------|
| `max_size_kb` | `int64` | Maximum size per log file (KB) | `1000` |
| `max_total_size_kb` | `int64` | Maximum total log directory size (KB) | `5000` |
| `min_disk_free_kb` | `int64` | Minimum required free disk space (KB) | `10000` |
| `retention_period_hrs` | `float64` | Hours to keep log files (0=disabled) | `0.0`  |
| `retention_check_mins` | `float64` | Retention check interval (minutes) | `60.0` |

### Disk Monitoring

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `disk_check_interval_ms` | `int64` | Base disk check interval (ms) | `5000` |
| `enable_adaptive_interval` | `bool` | Adjust check interval based on load | `true` |
| `min_check_interval_ms` | `int64` | Minimum adaptive interval (ms) | `100` |
| `max_check_interval_ms` | `int64` | Maximum adaptive interval (ms) | `60000` |

### Heartbeat Monitoring

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `heartbeat_level` | `int64` | Heartbeat detail (0=off, 1=proc, 2=+disk, 3=+sys) | `0` |
| `heartbeat_interval_s` | `int64` | Heartbeat interval (seconds) | `60` |

---