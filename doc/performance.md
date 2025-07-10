# Performance Guide

[← Heartbeat Monitoring](heartbeat-monitoring.md) | [← Back to README](../README.md) | [Compatibility Adapters →](compatibility-adapters.md)

Architecture overview and performance optimization strategies for the lixenwraith/log package.

## Table of Contents

- [Architecture Overview](#architecture-overview)
- [Performance Characteristics](#performance-characteristics)
- [Optimization Strategies](#optimization-strategies)
- [Benchmarking](#benchmarking)
- [Troubleshooting Performance](#troubleshooting-performance)

## Architecture Overview

### Lock-Free Design

The logger uses a lock-free architecture for maximum performance:

```
┌─────────────┐     Atomic Checks      ┌──────────────┐
│   Logger    │ ──────────────────────→│ State Check  │
│  Methods    │                        │ (No Locks)   │
└─────────────┘                        └──────────────┘
      │                                        │
      │ Non-blocking                          │ Pass
      ↓ Channel Send                          ↓
┌─────────────┐                        ┌──────────────┐
│  Buffered   │←───────────────────────│ Format Data  │
│  Channel    │                        │ (Stack Alloc)│
└─────────────┘                        └──────────────┘
      │
      │ Single Consumer
      ↓ Goroutine
┌─────────────┐     Batch Write        ┌──────────────┐
│  Processor  │ ──────────────────────→│ File System  │
│  Goroutine  │                        │    (OS)      │
└─────────────┘                        └──────────────┘
```

### Key Components

1. **Atomic State Management**: No mutexes in hot path
2. **Buffered Channel**: Decouples producers from I/O
3. **Single Processor**: Eliminates write contention
4. **Reusable Serializer**: Minimizes allocations

## Performance Characteristics

### Throughput

Typical performance on modern hardware:

| Scenario | Logs/Second | Latency (p99) |
|----------|-------------|---------------|
| File only | 500,000+ | < 1μs |
| File + Console | 100,000+ | < 5μs |
| JSON format | 400,000+ | < 2μs |
| With rotation | 450,000+ | < 2μs |

### Memory Usage

- **Per Logger**: ~10KB base overhead
- **Per Log Entry**: 0 allocations (reused buffer)
- **Channel Buffer**: `buffer_size * 24 bytes`

### CPU Impact

- **Logging Thread**: < 0.1% CPU per 100k logs/sec
- **Processor Thread**: 1-5% CPU depending on I/O

## Optimization Strategies

### 1. Buffer Size Tuning

Choose buffer size based on burst patterns:

```go
// Low volume, consistent rate
logger.InitWithDefaults("buffer_size=256")

// Medium volume with bursts
logger.InitWithDefaults("buffer_size=1024")  // Default

// High volume or large bursts
logger.InitWithDefaults("buffer_size=4096")

// Extreme bursts (monitor for drops)
logger.InitWithDefaults(
    "buffer_size=8192",
    "heartbeat_level=1",  // Monitor dropped logs
)
```

### 2. Flush Interval Optimization

Balance latency vs throughput:

```go
// Low latency (more syscalls)
logger.InitWithDefaults("flush_interval_ms=10")

// Balanced (default)
logger.InitWithDefaults("flush_interval_ms=100")

// High throughput (batch writes)
logger.InitWithDefaults(
    "flush_interval_ms=1000",
    "enable_periodic_sync=false",
)
```

### 3. Format Selection

Choose format based on needs:

```go
// Maximum performance
logger.InitWithDefaults(
    "format=txt",
    "show_timestamp=false",  // Skip time formatting
    "show_level=false",      // Skip level string
)

// Balanced features/performance
logger.InitWithDefaults("format=txt")  // Default

// Structured but slower
logger.InitWithDefaults("format=json")
```

### 4. Disk I/O Optimization

Reduce disk operations:

```go
// Minimize disk checks
logger.InitWithDefaults(
    "disk_check_interval_ms=30000",      // 30 seconds
    "enable_adaptive_interval=false",    // Fixed interval
    "enable_periodic_sync=false",        // No periodic sync
)

// Large files to reduce rotations
logger.InitWithDefaults(
    "max_size_mb=1000",                  // 1GB files
)

// Disable unnecessary features
logger.InitWithDefaults(
    "retention_period_hrs=0",            // No retention checks
    "heartbeat_level=0",                 // No heartbeats
)
```

### 5. Console Output Optimization

For development with console output:

```go
// Faster console output
logger.InitWithDefaults(
    "enable_stdout=true",
    "stdout_target=stdout",  // Slightly faster than stderr
    "disable_file=true",     // Skip file I/O entirely
)
```

## Benchmarking

### Basic Benchmark

```go
func BenchmarkLogger(b *testing.B) {
    logger := log.NewLogger()
    logger.InitWithDefaults(
        "directory=./bench_logs",
        "buffer_size=4096",
        "flush_interval_ms=1000",
    )
    defer logger.Shutdown()
    
    b.ResetTimer()
    b.RunParallel(func(pb *testing.PB) {
        for pb.Next() {
            logger.Info("Benchmark log",
                "iteration", 1,
                "thread", runtime.GOID(),
                "timestamp", time.Now(),
            )
        }
    })
}
```

### Throughput Test

```go
func TestThroughput(t *testing.T) {
    logger := log.NewLogger()
    logger.InitWithDefaults("buffer_size=4096")
    defer logger.Shutdown()
    
    start := time.Now()
    count := 1000000
    
    for i := 0; i < count; i++ {
        logger.Info("msg", "seq", i)
    }
    
    logger.Flush(5 * time.Second)
    duration := time.Since(start)
    
    rate := float64(count) / duration.Seconds()
    t.Logf("Throughput: %.0f logs/sec", rate)
}
```

### Memory Profile

```go
func profileMemory() {
    logger := log.NewLogger()
    logger.InitWithDefaults()
    defer logger.Shutdown()
    
    // Force GC for baseline
    runtime.GC()
    var m1 runtime.MemStats
    runtime.ReadMemStats(&m1)
    
    // Log heavily
    for i := 0; i < 100000; i++ {
        logger.Info("Memory test", "index", i)
    }
    
    // Measure again
    runtime.GC()
    var m2 runtime.MemStats
    runtime.ReadMemStats(&m2)
    
    fmt.Printf("Alloc delta: %d bytes\n", m2.Alloc-m1.Alloc)
    fmt.Printf("Total alloc: %d bytes\n", m2.TotalAlloc-m1.TotalAlloc)
}
```

## Troubleshooting Performance

### 1. Detecting Dropped Logs

Monitor heartbeats for drops:

```go
logger.InitWithDefaults(
    "heartbeat_level=1",
    "heartbeat_interval_s=60",
)

// In logs: dropped_logs=1523
```

**Solutions:**
- Increase `buffer_size`
- Reduce log volume
- Optimize log formatting

### 2. High CPU Usage

Check processor goroutine:

```go
// Enable system stats
logger.InitWithDefaults(
    "heartbeat_level=3",
    "heartbeat_interval_s=10",
)

// Monitor: num_goroutine count
// Monitor: CPU usage of process
```

**Solutions:**
- Increase `flush_interval_ms`
- Disable `enable_periodic_sync`
- Reduce `heartbeat_level`

### 3. Memory Growth

```go
// Add memory monitoring
go func() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)
        logger.Info("Memory stats",
            "alloc_mb", m.Alloc/1024/1024,
            "sys_mb", m.Sys/1024/1024,
            "num_gc", m.NumGC,
        )
    }
}()
```

**Solutions:**
- Check for logger reference leaks
- Verify `buffer_size` is reasonable
- Look for infinite log loops

### 4. Slow Disk I/O

Identify I/O bottlenecks:

```bash
# Monitor disk I/O
iostat -x 1

# Check write latency
ioping -c 10 /var/log
```

**Solutions:**
- Use faster storage (SSD)
- Increase `flush_interval_ms`
- Enable write caching
- Use separate log volume

### 5. Lock Contention

The logger is designed to avoid locks, but check for:

```go
// Profile mutex contention
import _ "net/http/pprof"

go func() {
    runtime.SetMutexProfileFraction(1)
    http.ListenAndServe("localhost:6060", nil)
}()

// Check: go tool pprof http://localhost:6060/debug/pprof/mutex
```

### Performance Checklist

Before deploying:

- [ ] Appropriate `buffer_size` for load
- [ ] Reasonable `flush_interval_ms`
- [ ] Correct `format` for use case
- [ ] Heartbeat monitoring enabled
- [ ] Disk space properly configured
- [ ] Retention policies set
- [ ] Load tested with expected volume
- [ ] Drop monitoring in place
- [ ] CPU/memory baseline established

---

[← Heartbeat Monitoring](heartbeat-monitoring.md) | [← Back to README](../README.md) | [Compatibility Adapters →](compatibility-adapters.md)