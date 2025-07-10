# Heartbeat Monitoring

[← Disk Management](disk-management.md) | [← Back to README](../README.md) | [Performance →](performance.md)

Guide to using heartbeat messages for operational monitoring and system health tracking.

## Table of Contents

- [Overview](#overview)
- [Heartbeat Levels](#heartbeat-levels)
- [Configuration](#configuration)
- [Heartbeat Messages](#heartbeat-messages)
- [Monitoring Integration](#monitoring-integration)
- [Use Cases](#use-cases)

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
logger.InitWithDefaults(
    "heartbeat_level=0",  // No heartbeats
)
```

### Level 1: Process Statistics (PROC)

Basic logger operation metrics:

```go
logger.InitWithDefaults(
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
logger.InitWithDefaults(
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
logger.InitWithDefaults(
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
logger.InitWithDefaults(
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
logger.InitWithDefaults(
    "heartbeat_level=1",
    "heartbeat_interval_s=600",
)

// During incident, increase detail
logger.InitWithDefaults(
    "heartbeat_level=3",
    "heartbeat_interval_s=60",
)

// After resolution, reduce back
logger.InitWithDefaults(
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

## Monitoring Integration

### Prometheus Exporter

```go
type LoggerMetrics struct {
    logger *log.Logger
    
    uptime          prometheus.Gauge
    processedTotal  prometheus.Counter
    droppedTotal    prometheus.Counter
    diskUsageMB     prometheus.Gauge
    diskFreeSpace   prometheus.Gauge
    fileCount       prometheus.Gauge
}

func (m *LoggerMetrics) ParseHeartbeat(line string) {
    if strings.Contains(line, "type=\"proc\"") {
        // Extract and update process metrics
        if match := regexp.MustCompile(`processed_logs=(\d+)`).FindStringSubmatch(line); match != nil {
            if val, err := strconv.ParseFloat(match[1], 64); err == nil {
                m.processedTotal.Set(val)
            }
        }
    }
    
    if strings.Contains(line, "type=\"disk\"") {
        // Extract and update disk metrics
        if match := regexp.MustCompile(`total_log_size_mb="([0-9.]+)"`).FindStringSubmatch(line); match != nil {
            if val, err := strconv.ParseFloat(match[1], 64); err == nil {
                m.diskUsageMB.Set(val)
            }
        }
    }
}
```

### Grafana Dashboard

Create alerts based on heartbeat metrics:

```yaml
# Dropped logs alert
- alert: HighLogDropRate
  expr: rate(logger_dropped_total[5m]) > 10
  annotations:
    summary: "High log drop rate detected"
    description: "Logger dropping {{ $value }} logs/sec"

# Disk space alert
- alert: LogDiskSpaceLow
  expr: logger_disk_free_mb < 1000
  annotations:
    summary: "Low log disk space"
    description: "Only {{ $value }}MB free on log disk"

# Logger health alert
- alert: LoggerUnhealthy
  expr: logger_disk_status_ok == 0
  annotations:
    summary: "Logger disk status unhealthy"
```

### ELK Stack Integration

Logstash filter for parsing heartbeats:

```ruby
filter {
  if [message] =~ /type="(proc|disk|sys)"/ {
    grok {
      match => {
        "message" => [
          '%{TIMESTAMP_ISO8601:timestamp} %{WORD:level} type="%{WORD:heartbeat_type}" sequence=%{NUMBER:sequence:int} uptime_hours="%{NUMBER:uptime_hours:float}" processed_logs=%{NUMBER:processed_logs:int} dropped_logs=%{NUMBER:dropped_logs:int}',
          '%{TIMESTAMP_ISO8601:timestamp} %{WORD:level} type="%{WORD:heartbeat_type}" sequence=%{NUMBER:sequence:int} rotated_files=%{NUMBER:rotated_files:int} deleted_files=%{NUMBER:deleted_files:int} total_log_size_mb="%{NUMBER:total_log_size_mb:float}"'
        ]
      }
    }
    
    mutate {
      add_tag => [ "heartbeat", "metrics" ]
    }
  }
}
```

## Use Cases

### 1. Production Health Monitoring

```go
// Production configuration
logger.InitWithDefaults(
    "level=4",                   // Warn and Error only
    "heartbeat_level=2",         // But still get disk stats
    "heartbeat_interval_s=300",  // Every 5 minutes
)

// Monitor for:
// - Dropped logs (buffer overflow)
// - Disk space issues
// - File rotation frequency
// - Logger uptime (crash detection)
```

### 2. Performance Tuning

```go
// Detailed monitoring during load test
logger.InitWithDefaults(
    "heartbeat_level=3",        // All stats
    "heartbeat_interval_s=10",  // Frequent updates
)

// Track:
// - Memory usage trends
// - Goroutine leaks
// - GC frequency
// - Log throughput
```

### 3. Capacity Planning

```go
// Long-term trending
logger.InitWithDefaults(
    "heartbeat_level=2",
    "heartbeat_interval_s=3600",  // Hourly
)

// Analyze:
// - Log growth rate
// - Rotation frequency
// - Disk usage trends
// - Seasonal patterns
```

### 4. Debugging Logger Issues

```go
// When investigating logger problems
logger.InitWithDefaults(
    "level=-4",                 // Debug everything
    "heartbeat_level=3",        // All heartbeats
    "heartbeat_interval_s=5",   // Very frequent
    "enable_stdout=true",       // Console output
)
```

### 5. Alerting Script

```bash
#!/bin/bash
# Monitor heartbeats for issues

tail -f /var/log/myapp/*.log | while read line; do
    if [[ $line =~ type=\"proc\" ]]; then
        if [[ $line =~ dropped_logs=([0-9]+) ]] && [[ ${BASH_REMATCH[1]} -gt 0 ]]; then
            alert "Logs being dropped: ${BASH_REMATCH[1]}"
        fi
    fi
    
    if [[ $line =~ type=\"disk\" ]]; then
        if [[ $line =~ disk_status_ok=false ]]; then
            alert "Logger disk unhealthy!"
        fi
        
        if [[ $line =~ disk_free_mb=\"([0-9.]+)\" ]]; then
            free_mb=${BASH_REMATCH[1]}
            if (( $(echo "$free_mb < 500" | bc -l) )); then
                alert "Low disk space: ${free_mb}MB"
            fi
        fi
    fi
done
```

---

[← Disk Management](disk-management.md) | [← Back to README](../README.md) | [Performance →](performance.md)