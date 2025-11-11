# Heartbeat Monitoring

Guide to using heartbeat messages for operational monitoring and system health tracking.

## Overview

Heartbeats are periodic log messages that provide operational statistics about the logger and system. They bypass normal log level filtering, ensuring visibility even when running at higher log levels.

### Key Features

- **Always Visible**: Heartbeats use special log levels that bypass filtering
- **Multi-Level Detail**: Choose from process, disk, or system statistics
- **Production Monitoring**: Track logger health without debug logs
- **Metrics Source**: Parse heartbeats for monitoring dashboards

## Heartbeat Levels

### Level 0: Disabled (Default)

No heartbeat messages are generated.

```go
logger.ApplyConfigString(
    "heartbeat_level=0",  // No heartbeats
)
```

### Level 1: Process Statistics (PROC)

Basic logger operation metrics:

```go
logger.ApplyConfigString(
    "heartbeat_level=1",
    "heartbeat_interval_s=300",  // Every 5 minutes
)
```

**Output:**
```
2024-01-15T10:30:00Z PROC type="proc" sequence=1 uptime_hours="24.50" processed_logs=1847293 dropped_logs=0
```

**Fields:**
- `sequence`: Incrementing counter
- `uptime_hours`: Logger uptime
- `processed_logs`: Successfully written logs
- `dropped_logs`: Logs lost due to buffer overflow

### Level 2: Process + Disk Statistics (DISK)

Includes file and disk usage information:

```go
logger.ApplyConfigString(
    "heartbeat_level=2",
    "heartbeat_interval_s=300",
)
```

**Additional Output:**
```
2024-01-15T10:30:00Z DISK type="disk" sequence=1 rotated_files=12 deleted_files=5 total_log_size_mb="487.32" log_file_count=8 current_file_size_mb="23.45" disk_status_ok=true disk_free_mb="5234.67"
```

**Additional Fields:**
- `rotated_files`: Total file rotations
- `deleted_files`: Files removed by cleanup
- `total_log_size_mb`: Size of all log files
- `log_file_count`: Number of log files
- `current_file_size_mb`: Active file size
- `disk_status_ok`: Disk health status
- `disk_free_mb`: Available disk space

### Level 3: Process + Disk + System Statistics (SYS)

Includes runtime and memory metrics:

```go
logger.ApplyConfigString(
    "heartbeat_level=3",
    "heartbeat_interval_s=60",  // Every minute for detailed monitoring
)
```

**Additional Output:**
```
2024-01-15T10:30:00Z SYS type="sys" sequence=1 alloc_mb="45.23" sys_mb="128.45" num_gc=1523 num_goroutine=42
```

**Additional Fields:**
- `alloc_mb`: Allocated memory
- `sys_mb`: System memory reserved
- `num_gc`: Garbage collection runs
- `num_goroutine`: Active goroutines

## Configuration

### Basic Configuration

```go
logger.ApplyConfigString(
    "heartbeat_level=2",         // Process + Disk stats
    "heartbeat_interval_s=300",  // Every 5 minutes
)
```

### Interval Recommendations

| Environment | Level | Interval | Rationale |
|-------------|-------|----------|-----------|
| Development | 3 | 30s | Detailed debugging info |
| Staging | 2 | 300s | Balance detail vs noise |
| Production | 1-2 | 300-600s | Minimize overhead |
| High-Load | 1 | 600s | Reduce I/O impact |

### Dynamic Adjustment

```go
// Start with basic monitoring
logger.ApplyConfigString(
    "heartbeat_level=1",
    "heartbeat_interval_s=600",
)

// During incident, increase detail
logger.ApplyConfigString(
    "heartbeat_level=3",
    "heartbeat_interval_s=60",
)

// After resolution, reduce back
logger.ApplyConfigString(
    "heartbeat_level=1",
    "heartbeat_interval_s=600",
)
```

## Heartbeat Messages

### JSON Format Example

With `format=json`, heartbeats are structured for easy parsing:

```json
{
  "time": "2024-01-15T10:30:00.123456789Z",
  "level": "PROC",
  "fields": [
    "type", "proc",
    "sequence", 42,
    "uptime_hours", "24.50",
    "processed_logs", 1847293,
    "dropped_logs", 0
  ]
}
```

### Text Format Example

With `format=txt`, heartbeats are human-readable:

```
2024-01-15T10:30:00.123456789Z PROC type="proc" sequence=42 uptime_hours="24.50" processed_logs=1847293 dropped_logs=0
```

---
[← Disk Management](disk-management.md) | [← Back to README](../README.md) | [Compatibility Adapters →](compatibility-adapters.md)