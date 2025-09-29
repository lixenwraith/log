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
logger := log.NewLogger() // logger instance created with DefaultConfig (using default values)

logger.Info("info txt log record written to ./log/log.log")

// Directly change config struct
cfg := log.GetConfig()
cfg.Level = log.LevelDebug
cfg.Name = "myapp"
cfg.Directory = "/var/log/myapp"
cfg.Format = "json"
cfg.MaxSizeKB = 100
err := logger.ApplyConfig(cfg)

logger.Info("info json log record written to /var/log/myapp/myapp.log")

// Override values with key-value string
err = logger.ApplyConfigString(
    "directory=/var/log/",
	"extension=txt"
    "format=txt")

logger.Info("info txt log record written to /var/log/myapp.txt")
```

## Configuration Parameters

### Basic Settings

| Parameter | Type | Description | Default    |
|-----------|------|-------------|------------|
| `level` | `int64` | Minimum log level (-4=Debug, 0=Info, 4=Warn, 8=Error) | `0` |
| `name` | `string` | Base name for log files | `"log"`    |
| `directory` | `string` | Directory to store log files | `"./log"` |
| `format` | `string` | Output format: `"txt"` or `"json"` | `"txt"` |
| `extension` | `string` | Log file extension (without dot) | `"log"` |
| `internal_errors_to_stderr` | `bool` | Write logger's internal errors to stderr | `false` |

### Output Control

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `show_timestamp` | `bool` | Include timestamps in log entries | `true` |
| `show_level` | `bool` | Include log level in entries | `true` |
| `enable_console` | `bool` | Mirror logs to stdout/stderr | `false` |
| `console_target` | `string` | Console target: `"stdout"`, `"stderr"`, or `"split"` | `"stdout"` |
| `disable_file` | `bool` | Disable file output (console-only) | `false` |

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

[← Getting Started](getting-started.md) | [← Back to README](../README.md) | [Configuration Builder →](config-builder.md)