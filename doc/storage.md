# Disk Management

Comprehensive guide to log file rotation, retention policies, and disk space management.

## File Rotation

### Automatic Rotation

Log files are automatically rotated when they reach the configured size limit in Kilobytes:

```go
logger.ApplyConfigString(
    "max_size_kb=100000",  // Rotate at 100 MB (100,000 KB)
)
```

### Rotation Behavior

1. Before each write, the logger checks whether the active file would exceed `max_size_kb`.
2. It closes the active `{name}.{extension}` file and renames it to `{name}_{YYMMDD}_{HHMMSS}[_N].{extension}`.
3. It opens a new active file. A counter suffix avoids existing names when rotations share a second.

For example: `myapp.log` becomes `myapp_240115_143022.log`, then `myapp_240115_143022_1.log` on the next collision. Rotation does not split a record, so a single oversized record can exceed the size limit. File errors can drop records and are counted. Periodic sync, explicit `Flush`, and shutdown provide sync points; closing a file alone does not guarantee crash durability.

Use a directory owned exclusively by this logger. Cleanup and retention select regular files by extension, including files from earlier runs with different base names. They exclude the active filename, directories, and symlinks. With an empty extension, only extensionless files match. Console-only configurations never perform retention deletion. Total-size and free-space limits are checked periodically and can be exceeded between checks.

## Disk Space Management

### Space Limits

The logger enforces two types of space limits:

```go
logger.ApplyConfigString(
    "max_total_size_kb=1000",   // Total log directory size
    "min_disk_free_mb=5000",    // Minimum free disk space
)
```

### Automatic Cleanup

When limits are exceeded, the logger:
1. Identifies oldest log files
2. Deletes them until space requirements are met
3. Preserves the current active log file
4. Increments deletion counters reported in disk heartbeats

### Example Configuration

```go
// Conservative: Strict limits
logger.ApplyConfigString(
    "max_size_kb=500",            // 500 KB files
    "max_total_size_kb=5000",     // 5 MB total log directory limit
    "min_disk_free_mb=1000000",   // 1 GB free space required on disk
)

// Generous: Large files, external archival
logger.ApplyConfigString(
    "max_size_kb=100000",         // 100 MB files
    "max_total_size_kb=0",        // No total limit
    "min_disk_free_mb=10000",     // 10 MB free required
)

// Balanced: Production defaults
logger.ApplyConfigString(
    "max_size_kb=100000",         // 100 MB files
    "max_total_size_kb=5000000",  // 5 GB total limit
    "min_disk_free_mb=500000",    // 500 MB free required
)
```

## Retention Policies

### Time-Based Retention

Automatically delete logs older than a specified duration:

```go
logger.ApplyConfigString(
    "retention_period_hrs=168",    // Keep 7 days
    "retention_check_mins=60",     // Check hourly
)
```

### Retention Examples

```go
// Daily logs, keep 30 days
logger.ApplyConfigString(
    "retention_period_hrs=720",    // 30 days
    "retention_check_mins=60",     // Check hourly
    "max_size_kb=1000000",           // 1GB daily files
)

// High-frequency logs, keep 24 hours
logger.ApplyConfigString(
    "retention_period_hrs=24",     // 1 day
    "retention_check_mins=15",     // Check every 15 min
    "max_size_kb=100000",            // 100MB files
)

// Compliance: Keep 90 days
logger.ApplyConfigString(
    "retention_period_hrs=2160",   // 90 days
    "retention_check_mins=360",    // Check every 6 hours
    "max_total_size_kb=100000000",   // 100GB total
)
```

### Retention Priority

When multiple policies conflict, cleanup priority is:
1. **Disk free space** (highest priority)
2. **Total size limit**
3. **Retention period** (lowest priority)

## Adaptive Monitoring

### Adaptive Disk Checks

The logger adjusts disk check frequency based on logging volume:

```go
logger.ApplyConfigString(
    "enable_adaptive_interval=true",
    "disk_check_interval_ms=5000",    // Base: 5 seconds
    "min_check_interval_ms=100",      // Minimum: 100ms
    "max_check_interval_ms=60000",    // Maximum: 1 minute
)
```

### How It Works

1. **Low Activity**: Interval increases (up to max)
2. **High Activity**: Interval decreases (down to min)
3. **Reactive Checks**: Immediate check after 10MB written

### Monitoring Disk Usage

Check disk-related heartbeat messages:

```go
logger.ApplyConfigString(
    "heartbeat_level=2",           // Enable disk stats
    "heartbeat_interval_s=300",    // Every 5 minutes
)
```

Output:
```
2024-01-15T10:30:00Z DISK type="disk" sequence=1 rotated_files=5 deleted_files=2 total_log_size_mb="487.32" log_file_count=8 current_file_size_mb="23.45" disk_status_ok=true disk_free_mb="5234.67"
```

## Manual Recovery

If automatic cleanup fails:

```bash
# Check disk usage
df -h /var/log

# Find large log files
find /var/log/myapp -name "*.log" -size +100M

# Manual cleanup (oldest first)
# Review archive paths before deleting; preserve the active log file.
find /var/log/myapp -maxdepth 1 -type f -name 'myapp_*.log' -print

# Verify space
df -h /var/log
```
