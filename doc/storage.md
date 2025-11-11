# Disk Management

Comprehensive guide to log file rotation, retention policies, and disk space management.

## File Rotation

### Automatic Rotation

Log files are automatically rotated when they reach the configured size limit:

```go
logger.ApplyConfigString(
    "max_size_kb=100",  // Rotate at 100MB
)
```

### Rotation Behavior

1. **Size Check**: Before each write, the logger checks if the file would exceed `max_size_kb`
2. **New File Creation**: Creates a new file with timestamp: `appname_240115_103045_123456789.log`
3. **Seamless Transition**: No logs are lost during rotation
4. **Old File Closure**: Previous file is properly closed and synced

### File Naming Convention

```
{name}_{YYMMDD}_{HHMMSS}_{nanoseconds}.{extension}

Example: myapp_240115_143022_987654321.log
```

Components:
- `name`: Configured log name
- `YYMMDD`: Date (year, month, day)
- `HHMMSS`: Time (hour, minute, second)
- `nanoseconds`: For uniqueness
- `extension`: Configured extension

## Disk Space Management

### Space Limits

The logger enforces two types of space limits:

```go
logger.ApplyConfigString(
    "max_total_size_kb=1000",   // Total log directory size
    "min_disk_free_kb=5000",    // Minimum free disk space
)
```

### Automatic Cleanup

When limits are exceeded, the logger:
1. Identifies oldest log files
2. Deletes them until space requirements are met
3. Preserves the current active log file
4. Logs cleanup actions for audit

### Example Configuration

```go
// Conservative: Strict limits
logger.ApplyConfigString(
    "max_size_kb=500",          // 500KB files
    "max_total_size_kb=5000",   // 5MB total
    "min_disk_free_kb=1000000",   // 1GB free required
)

// Generous: Large files, external archival
logger.ApplyConfigString(
    "max_size_kb=100000",        // 100MB files
    "max_total_size_kb=0",     // No total limit
    "min_disk_free_kb=10000",    // 10MB free required
)

// Balanced: Production defaults
logger.ApplyConfigString(
    "max_size_kb=100000",         // 100MB files
    "max_total_size_kb=5000000",  // 5GB total
    "min_disk_free_kb=500000",    // 500MB free required
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
2024-01-15T10:30:00Z DISK type="disk" sequence=1 rotated_files=5 deleted_files=2 total_log_size_kb="487.32" log_file_count=8 current_file_size_kb="23.45" disk_status_ok=true disk_free_kb="5234.67"
```

## Manual Recovery

If automatic cleanup fails:

```bash
# Check disk usage
df -h /var/log

# Find large log files
find /var/log/myapp -name "*.log" -size +100M

# Manual cleanup (oldest first)
ls -t /var/log/myapp/*.log | tail -n 20 | xargs rm

# Verify space
df -h /var/log
```

---
[← Logging Guide](logging-guide.md) | [← Back to README](../README.md) | [Heartbeat Monitoring →](heartbeat-monitoring.md)