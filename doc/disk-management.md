# Disk Management

Comprehensive guide to log file rotation, retention policies, and disk space management.

## File Rotation

### Automatic Rotation

Log files are automatically rotated when they reach the configured size limit:

```go
logger.ApplyOverride(
    "max_size_mb=100",  // Rotate at 100MB
)
```

### Rotation Behavior

1. **Size Check**: Before each write, the logger checks if the file would exceed `max_size_mb`
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
logger.ApplyOverride(
    "max_total_size_mb=1000",   // Total log directory size
    "min_disk_free_mb=5000",    // Minimum free disk space
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
logger.ApplyOverride(
    "max_size_mb=50",          // 50MB files
    "max_total_size_mb=500",   // 500MB total
    "min_disk_free_mb=1000",   // 1GB free required
)

// Generous: Large files, external archival
logger.ApplyOverride(
    "max_size_mb=1000",        // 1GB files
    "max_total_size_mb=0",     // No total limit
    "min_disk_free_mb=100",    // 100MB free required
)

// Balanced: Production defaults
logger.ApplyOverride(
    "max_size_mb=100",         // 100MB files
    "max_total_size_mb=5000",  // 5GB total
    "min_disk_free_mb=500",    // 500MB free required
)
```

## Retention Policies

### Time-Based Retention

Automatically delete logs older than a specified duration:

```go
logger.ApplyOverride(
    "retention_period_hrs=168",    // Keep 7 days
    "retention_check_mins=60",     // Check hourly
)
```

### Retention Examples

```go
// Daily logs, keep 30 days
logger.ApplyOverride(
    "retention_period_hrs=720",    // 30 days
    "retention_check_mins=60",     // Check hourly
    "max_size_mb=1000",           // 1GB daily files
)

// High-frequency logs, keep 24 hours
logger.ApplyOverride(
    "retention_period_hrs=24",     // 1 day
    "retention_check_mins=15",     // Check every 15 min
    "max_size_mb=100",            // 100MB files
)

// Compliance: Keep 90 days
logger.ApplyOverride(
    "retention_period_hrs=2160",   // 90 days
    "retention_check_mins=360",    // Check every 6 hours
    "max_total_size_mb=100000",   // 100GB total
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
logger.ApplyOverride(
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
logger.ApplyOverride(
    "heartbeat_level=2",           // Enable disk stats
    "heartbeat_interval_s=300",    // Every 5 minutes
)
```

Output:
```
2024-01-15T10:30:00Z DISK type="disk" sequence=1 rotated_files=5 deleted_files=2 total_log_size_mb="487.32" log_file_count=8 current_file_size_mb="23.45" disk_status_ok=true disk_free_mb="5234.67"
```

## Recovery Behavior

### Manual Intervention

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

## Best Practices

### 1. Plan for Growth

Estimate log volume and set appropriate limits:

```go
// Calculate required space:
// - Average log entry: 200 bytes
// - Entries per second: 100
// - Daily volume: 200 * 100 * 86400 = 1.7GB

logger.ApplyOverride(
    "max_size_mb=2000",          // 2GB files (~ 1 day)
    "max_total_size_mb=15000",   // 15GB (~ 1 week)
    "retention_period_hrs=168",   // 7 days
)
```

### 2. External Archival

For long-term storage, implement external archival:

```go
// Configure for archival
logger.ApplyOverride(
    "max_size_mb=1000",          // 1GB files for easy transfer
    "max_total_size_mb=10000",   // 10GB local buffer
    "retention_period_hrs=48",    // 2 days local
)

// Archive completed files
func archiveCompletedLogs(archivePath string) error {
    files, _ := filepath.Glob("/var/log/myapp/*.log")
    for _, file := range files {
        if !isCurrentLogFile(file) {
            // Move to archive storage (S3, NFS, etc.)
            if err := archiveFile(file, archivePath); err != nil {
                return err
            }
            os.Remove(file)
        }
    }
    return nil
}
```

### 3. Monitor Disk Health

Set up alerts for disk issues:

```go
// Parse heartbeat logs for monitoring
type DiskStats struct {
    TotalSizeMB    float64
    FileCount      int
    DiskFreeMB     float64
    DiskStatusOK   bool
}

func monitorDiskHealth(logLine string) {
    if strings.Contains(logLine, "type=\"disk\"") {
        stats := parseDiskHeartbeat(logLine)
        
        if !stats.DiskStatusOK {
            alert("Log disk unhealthy")
        }
        
        if stats.DiskFreeMB < 1000 {
            alert("Low disk space: %.0fMB free", stats.DiskFreeMB)
        }
        
        if stats.FileCount > 100 {
            alert("Too many log files: %d", stats.FileCount)
        }
    }
}
```

### 4. Separate Log Volumes

Use dedicated volumes for logs:

```bash
# Create dedicated log volume
mkdir -p /mnt/logs
mount /dev/sdb1 /mnt/logs

# Configure logger
logger.ApplyOverride(
    "directory=/mnt/logs/myapp",
    "max_total_size_mb=50000",   # Use most of volume
    "min_disk_free_mb=1000",     # Leave 1GB free
)
```

### 5. Test Cleanup Behavior

Verify cleanup works before production:

```go
// Test configuration
func TestDiskCleanup(t *testing.T) {
    logger := log.NewLogger()
    logger.ApplyOverride(
        "directory=./test_logs",
        "max_size_mb=1",             // Small files
        "max_total_size_mb=5",       // Low limit
        "retention_period_hrs=0.01", // 36 seconds
        "retention_check_mins=0.5",  // 30 seconds
    )
    
    // Generate logs to trigger cleanup
    for i := 0; i < 1000; i++ {
        logger.Info(strings.Repeat("x", 1000))
    }
    
    time.Sleep(45 * time.Second)
    
    // Verify cleanup occurred
    files, _ := filepath.Glob("./test_logs/*.log")
    if len(files) > 5 {
        t.Errorf("Cleanup failed: %d files remain", len(files))
    }
}
```

---

[← Logging Guide](logging-guide.md) | [← Back to README](../README.md) | [Heartbeat Monitoring →](heartbeat-monitoring.md)