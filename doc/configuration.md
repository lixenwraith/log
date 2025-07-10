# Configuration Guide

[← Getting Started](getting-started.md) | [← Back to README](../README.md) | [API Reference →](api-reference.md)

This guide covers all configuration options and methods for customizing logger behavior.

## Table of Contents

- [Configuration Methods](#configuration-methods)
- [Configuration Parameters](#configuration-parameters)
- [Configuration Examples](#configuration-examples)
- [Dynamic Reconfiguration](#dynamic-reconfiguration)
- [Configuration Best Practices](#configuration-best-practices)

## Configuration Methods

### Method 1: InitWithDefaults

Simple string-based configuration using key=value pairs:

```go
logger := log.NewLogger()
err := logger.InitWithDefaults(
    "directory=/var/log/myapp",
    "level=-4",
    "format=json",
    "max_size_mb=100",
)
```

### Method 2: Init with config.Config

Integration with external configuration management:

```go
cfg := config.New()
cfg.Load("app.toml", os.Args[1:])

logger := log.NewLogger()
err := logger.Init(cfg, "logging")  // Uses [logging] section
```

Example TOML configuration:

```toml
[logging]
level = -4
directory = "/var/log/myapp"
format = "json"
max_size_mb = 100
buffer_size = 2048
heartbeat_level = 2
heartbeat_interval_s = 300
```

## Configuration Parameters

### Basic Settings

| Parameter | Type | Description | Default    |
|-----------|------|-------------|------------|
| `level` | `int64` | Minimum log level (-4=Debug, 0=Info, 4=Warn, 8=Error) | `0`        |
| `name` | `string` | Base name for log files | `"log"`    |
| `directory` | `string` | Directory to store log files | `"./logs"` |
| `format` | `string` | Output format: `"txt"` or `"json"` | `"txt"`    |
| `extension` | `string` | Log file extension (without dot) | `"log"`    |
| `internal_errors_to_stderr` | `bool` | Write logger's internal errors to stderr | `false`    |

### Output Control

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `show_timestamp` | `bool` | Include timestamps in log entries | `true` |
| `show_level` | `bool` | Include log level in entries | `true` |
| `enable_stdout` | `bool` | Mirror logs to stdout/stderr | `false` |
| `stdout_target` | `string` | Console target: `"stdout"` or `"stderr"` | `"stdout"` |
| `disable_file` | `bool` | Disable file output (console-only) | `false` |

### Performance Tuning

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `buffer_size` | `int64` | Channel buffer size for log records | `1024` |
| `flush_interval_ms` | `int64` | Buffer flush interval (milliseconds) | `100` |
| `enable_periodic_sync` | `bool` | Enable periodic disk sync | `true` |
| `trace_depth` | `int64` | Default function trace depth (0-10) | `0` |

### File Management

| Parameter | Type | Description | Default |
|-----------|------|-------------|---------|
| `max_size_mb` | `int64` | Maximum size per log file (MB) | `10` |
| `max_total_size_mb` | `int64` | Maximum total log directory size (MB) | `50` |
| `min_disk_free_mb` | `int64` | Minimum required free disk space (MB) | `100` |
| `retention_period_hrs` | `float64` | Hours to keep log files (0=disabled) | `0.0` |
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

## Configuration Examples

### Development Configuration

Verbose logging with quick rotation for testing:

```go
logger.InitWithDefaults(
    "directory=./logs",
    "level=-4",              // Debug level
    "format=txt",            // Human-readable
    "max_size_mb=1",         // Small files for testing
    "flush_interval_ms=50",  // Quick flushes
    "trace_depth=3",         // Include call traces
    "enable_stdout=true",    // Also print to console
)
```

### Production Configuration

Optimized for performance with monitoring:

```go
logger.InitWithDefaults(
    "directory=/var/log/app",
    "level=0",                    // Info and above
    "format=json",                // Machine-parseable
    "buffer_size=4096",           // Large buffer
    "max_size_mb=1000",          // 1GB files
    "max_total_size_mb=50000",   // 50GB total
    "retention_period_hrs=168",   // 7 days
    "heartbeat_level=2",          // Process + disk stats
    "heartbeat_interval_s=300",   // 5 minutes
    "enable_periodic_sync=false", // Reduce I/O
)
```

### Container/Cloud Configuration

Console-only with structured output:

```go
logger.InitWithDefaults(
    "enable_stdout=true",
    "disable_file=true",     // No file output
    "format=json",           // Structured for log aggregators
    "level=0",               // Info level
    "show_timestamp=true",   // Include timestamps
    "internal_errors_to_stderr=false", // Suppress internal errors
)
```

### High-Security Configuration

Strict disk limits with frequent cleanup:

```go
logger.InitWithDefaults(
    "directory=/secure/logs",
    "level=4",                    // Warn and Error only
    "max_size_mb=100",           // 100MB files
    "max_total_size_mb=1000",    // 1GB total max
    "min_disk_free_mb=5000",     // 5GB free required
    "retention_period_hrs=24",    // 24 hour retention
    "retention_check_mins=15",    // Check every 15 min
    "flush_interval_ms=10",       // Immediate flush
)
```

## Dynamic Reconfiguration

The logger supports hot reconfiguration without losing data:

```go
// Initial configuration
logger := log.NewLogger()
logger.InitWithDefaults("level=0", "directory=/var/log/app")

// Later, change configuration
logger.InitWithDefaults(
    "level=-4",              // Now debug level
    "enable_stdout=true",    // Add console output
    "heartbeat_level=1",     // Enable monitoring
)
```

During reconfiguration:
- Pending logs are preserved
- Files are rotated if needed
- New settings take effect immediately

## Configuration Best Practices

### 1. Choose Appropriate Buffer Sizes

```go
// Low-volume application
"buffer_size=256"

// Medium-volume application (default)
"buffer_size=1024"

// High-volume application
"buffer_size=4096"

// Extreme volume (with monitoring)
"buffer_size=8192"
"heartbeat_level=1"  // Monitor for dropped logs
```

### 2. Set Sensible Rotation Limits

Consider your disk space and retention needs:

```go
// Development
"max_size_mb=10"
"max_total_size_mb=100"

// Production with archival
"max_size_mb=1000"       // 1GB files
"max_total_size_mb=0"    // No limit (external archival)
"retention_period_hrs=168" // 7 days local

// Space-constrained environment
"max_size_mb=50"
"max_total_size_mb=500"
"min_disk_free_mb=1000"
```

### 3. Use Appropriate Formats

```go
// Development/debugging
"format=txt"
"show_timestamp=true"
"show_level=true"

// Production with log aggregation
"format=json"
"show_timestamp=true"  // Aggregators parse this
"show_level=true"
```

### 4. Configure Monitoring

For production systems, enable heartbeats:

```go
// Basic monitoring
"heartbeat_level=1"      // Process stats only
"heartbeat_interval_s=300" // Every 5 minutes

// Full monitoring
"heartbeat_level=3"      // Process + disk + system
"heartbeat_interval_s=60"  // Every minute
```

### 5. Platform-Specific Paths

```go
// Linux/Unix
"directory=/var/log/myapp"

// Windows
"directory=C:\\Logs\\MyApp"

// Container (ephemeral)
"disable_file=true"
"enable_stdout=true"
```

---

[← Getting Started](getting-started.md) | [← Back to README](../README.md) | [API Reference →](api-reference.md)