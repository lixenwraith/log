# Troubleshooting

[← Examples](examples.md) | [← Back to README](../README.md)

Common issues and solutions when using the lixenwraith/log package.

## Table of Contents

- [Common Issues](#common-issues)
- [Diagnostic Tools](#diagnostic-tools)
- [Error Messages](#error-messages)
- [Performance Issues](#performance-issues)
- [Platform-Specific Issues](#platform-specific-issues)
- [FAQ](#faq)

## Common Issues

### Logger Not Writing to File

**Symptoms:**
- No log files created
- Empty log directory
- No error messages

**Solutions:**

1. **Check initialization**
   ```go
   logger := log.NewLogger()
   err := logger.InitWithDefaults()
   if err != nil {
       fmt.Printf("Init failed: %v\n", err)
   }
   ```

2. **Verify directory permissions**
   ```bash
   # Check directory exists and is writable
   ls -la /var/log/myapp
   touch /var/log/myapp/test.log
   ```

3. **Check if file output is disabled**
   ```go
   // Ensure file output is enabled
   logger.InitWithDefaults(
       "disable_file=false",  // Default, but be explicit
       "directory=/var/log/myapp",
   )
   ```

4. **Enable console output for debugging**
   ```go
   logger.InitWithDefaults(
       "enable_stdout=true",
       "level=-4",  // Debug level
   )
   ```

### Logs Being Dropped

**Symptoms:**
- "Logs were dropped" messages
- Missing log entries
- `dropped_logs` count in heartbeats

**Solutions:**

1. **Increase buffer size**
   ```go
   logger.InitWithDefaults(
       "buffer_size=4096",  // Increase from default 1024
   )
   ```

2. **Monitor with heartbeats**
   ```go
   logger.InitWithDefaults(
       "heartbeat_level=1",
       "heartbeat_interval_s=60",
   )
   // Watch for: dropped_logs=N
   ```

3. **Reduce log volume**
   ```go
   // Increase log level
   logger.InitWithDefaults("level=0")  // Info and above only
   
   // Or batch operations
   logger.Info("Batch processed", "count", 1000)  // Not 1000 individual logs
   ```

4. **Optimize flush interval**
   ```go
   logger.InitWithDefaults(
       "flush_interval_ms=500",  // Less frequent flushes
   )
   ```

### Disk Full Errors

**Symptoms:**
- "Log directory full or disk space low" messages
- `disk_status_ok=false` in heartbeats
- No new logs being written

**Solutions:**

1. **Configure automatic cleanup**
   ```go
   logger.InitWithDefaults(
       "max_total_size_mb=1000",    // 1GB total limit
       "min_disk_free_mb=500",      // 500MB free required
       "retention_period_hrs=24",    // Keep only 24 hours
   )
   ```

2. **Manual cleanup**
   ```bash
   # Find and remove old logs
   find /var/log/myapp -name "*.log" -mtime +7 -delete
   
   # Or keep only recent files
   ls -t /var/log/myapp/*.log | tail -n +11 | xargs rm
   ```

3. **Monitor disk usage**
   ```bash
   # Set up monitoring
   df -h /var/log
   du -sh /var/log/myapp
   ```

### Logger Initialization Failures

**Symptoms:**
- Init returns error
- "logger previously failed to initialize" errors
- Application won't start

**Common Errors and Solutions:**

1. **Invalid configuration**
   ```go
   // Error: "invalid format: 'xml' (use txt or json)"
   logger.InitWithDefaults("format=json")  // Use valid format
   
   // Error: "buffer_size must be positive"
   logger.InitWithDefaults("buffer_size=1024")  // Use positive value
   ```

2. **Directory creation failure**
   ```go
   // Error: "failed to create log directory: permission denied"
   // Solution: Check permissions or use accessible directory
   logger.InitWithDefaults("directory=/tmp/logs")
   ```

3. **Configuration conflicts**
   ```go
   // Error: "min_check_interval > max_check_interval"
   logger.InitWithDefaults(
       "min_check_interval_ms=100",
       "max_check_interval_ms=60000",  // Max must be >= min
   )
   ```

## Diagnostic Tools

### Enable Debug Logging

```go
// Temporary debug configuration
logger.InitWithDefaults(
    "level=-4",              // Debug everything
    "enable_stdout=true",    // See logs immediately
    "trace_depth=3",         // Include call stacks
    "heartbeat_level=3",     // All statistics
    "heartbeat_interval_s=10", // Frequent updates
)
```

### Check Logger State

```go
// Add diagnostic helper
func diagnoseLogger(logger *log.Logger) {
    // Try logging at all levels
    logger.Debug("Debug test")
    logger.Info("Info test")
    logger.Warn("Warn test")
    logger.Error("Error test")
    
    // Force flush
    if err := logger.Flush(1 * time.Second); err != nil {
        fmt.Printf("Flush failed: %v\n", err)
    }
    
    // Check for output
    time.Sleep(100 * time.Millisecond)
}
```

### Monitor Resource Usage

```go
// Add resource monitoring
func monitorResources(logger *log.Logger) {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)
        
        logger.Info("Resource usage",
            "goroutines", runtime.NumGoroutine(),
            "memory_mb", m.Alloc/1024/1024,
            "gc_runs", m.NumGC,
        )
    }
}
```

## Error Messages

### Configuration Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `log name cannot be empty` | Empty name parameter | Provide valid name or use default |
| `invalid format: 'X' (use txt or json)` | Invalid format value | Use "txt" or "json" |
| `extension should not start with dot` | Extension has leading dot | Use "log" not ".log" |
| `buffer_size must be positive` | Zero or negative buffer | Use positive value (default: 1024) |
| `trace_depth must be between 0 and 10` | Invalid trace depth | Use 0-10 range |

### Runtime Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `logger not initialized or already shut down` | Using closed logger | Check initialization order |
| `timeout waiting for flush confirmation` | Flush timeout | Increase timeout or check I/O |
| `failed to create log file: permission denied` | Directory permissions | Check directory access rights |
| `failed to write to log file: no space left` | Disk full | Free space or configure cleanup |

### Recovery Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `no old logs available to delete` | Can't free space | Manual intervention needed |
| `could not free enough space` | Cleanup insufficient | Reduce limits or add storage |
| `disk check failed` | Can't check disk space | Check filesystem health |

## Performance Issues

### High CPU Usage

**Diagnosis:**
```bash
# Check process CPU
top -p $(pgrep yourapp)

# Profile application
go tool pprof http://localhost:6060/debug/pprof/profile
```

**Solutions:**
1. Increase flush interval
2. Disable periodic sync
3. Reduce heartbeat level
4. Use text format instead of JSON

### Memory Growth

**Diagnosis:**
```go
// Add to application
import _ "net/http/pprof"
go http.ListenAndServe("localhost:6060", nil)

// Check heap
go tool pprof http://localhost:6060/debug/pprof/heap
```

**Solutions:**
1. Check for logger reference leaks
2. Verify reasonable buffer size
3. Look for logging loops

### Slow Disk I/O

**Diagnosis:**
```bash
# Check disk latency
iostat -x 1
ioping -c 10 /var/log
```

**Solutions:**
1. Use SSD storage
2. Increase flush interval
3. Disable periodic sync
4. Use separate log volume

## Platform-Specific Issues

### Linux

**File Handle Limits:**
```bash
# Check limits
ulimit -n

# Increase if needed
ulimit -n 65536
```

**SELinux Issues:**
```bash
# Check SELinux denials
ausearch -m avc -ts recent

# Set context for log directory
semanage fcontext -a -t var_log_t "/var/log/myapp(/.*)?"
restorecon -R /var/log/myapp
```

### FreeBSD

**Directory Permissions:**
```bash
# Ensure log directory ownership
chown appuser:appgroup /var/log/myapp
chmod 755 /var/log/myapp
```

**Jails Configuration:**
```bash
# Allow log directory access in jail
jail -m jid=1 allow.mount.devfs=1 path=/var/log/myapp
```

### Windows

**Path Format:**
```go
// Use proper Windows paths
logger.InitWithDefaults(
    "directory=C:\\Logs\\MyApp",  // Escaped backslashes
    // or
    "directory=C:/Logs/MyApp",    // Forward slashes work too
)
```

**Permissions:**
- Run as Administrator for system directories
- Use user-writable locations like `%APPDATA%`

## FAQ

### Q: Can I use the logger before initialization?

No, always initialize first:
```go
logger := log.NewLogger()
logger.InitWithDefaults()  // Must call before logging
logger.Info("Now safe to log")
```

### Q: How do I rotate logs manually?

The logger handles rotation automatically. To force rotation:
```go
// Set small size limit temporarily
logger.InitWithDefaults("max_size_mb=0.001")
logger.Info("This will trigger rotation")
```

### Q: Can I change log directory at runtime?

Yes, through reconfiguration:
```go
// Change directory
logger.InitWithDefaults("directory=/new/path")
```

### Q: How do I completely disable logging?

Several options:
```go
// Option 1: Disable file output, no console
logger.InitWithDefaults(
    "disable_file=true",
    "enable_stdout=false",
)

// Option 2: Set very high log level
logger.InitWithDefaults("level=100")  // Nothing will log

// Option 3: Don't initialize (logs are dropped)
logger := log.NewLogger()  // Don't call Init
```

### Q: Why are my logs not appearing immediately?

Logs are buffered for performance:
```go
// For immediate output
logger.InitWithDefaults(
    "flush_interval_ms=10",   // Quick flushes
    "enable_stdout=true",     // Also to console
)

// Or force flush
logger.Flush(1 * time.Second)
```

### Q: Can multiple processes write to the same log file?

No, each process should use its own log file:
```go
// Include process ID in name
logger.InitWithDefaults(
    fmt.Sprintf("name=myapp_%d", os.Getpid()),
)
```

### Q: How do I parse JSON logs?

Use any JSON parser:
```go
type LogEntry struct {
    Time   string        `json:"time"`
    Level  string        `json:"level"`
    Fields []interface{} `json:"fields"`
}

// Parse line
var entry LogEntry
json.Unmarshal([]byte(logLine), &entry)
```

### Getting Help

If you encounter issues not covered here:

1. Check the [examples](examples.md) for working code
2. Enable debug logging and heartbeats
3. Review error messages carefully
4. Check system logs for permission/disk issues
5. File an issue with:
    - Go version
    - OS/Platform
    - Minimal reproduction code
    - Error messages
    - Heartbeat output if available

---

[← Examples](examples.md) | [← Back to README](../README.md)