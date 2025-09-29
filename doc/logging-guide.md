# Logging Guide

Best practices and patterns for effective logging with the lixenwraith/log package.

## Log Levels

### Understanding Log Levels

The logger uses numeric levels for efficient filtering:

| Level | Name | Value | Use Case |
|-------|------|-------|----------|
| Debug | `LevelDebug` | -4 | Detailed information for debugging |
| Info | `LevelInfo` | 0 | General informational messages |
| Warn | `LevelWarn` | 4 | Warning conditions |
| Error | `LevelError` | 8 | Error conditions |

### Level Selection Guidelines

```go
logger.Debug("Cache lookup", "key", cacheKey, "found", found)

logger.Info("Order processed", "order_id", orderID, "amount", 99.99)

logger.Warn("Retry attempt", "service", "payment", "attempt", 3)

logger.Error("Database query failed", "query", query, "error", err)
```

### Setting Log Level

```go
// Development: See everything
logger.ApplyConfigString("level=-4")  // Debug and above

// Production: Reduce noise
logger.ApplyConfigString("level=0")   // Info and above

// Critical systems: Errors only
logger.ApplyConfigString("level=8")   // Error only
```

## Structured Logging

### Key-Value Pairs

Use structured key-value pairs for machine-parseable logs:

```go
logger.Info("User login",
    "user_id", user.ID,
    "email", user.Email,
    "ip", request.RemoteAddr,
    "timestamp", time.Now(),
)

// Works, but not recommended:
logger.Info(fmt.Sprintf("User %s logged in from %s", user.Email, request.RemoteAddr))
```

### Structured JSON Fields

For complex structured data with proper JSON marshaling:

```go
// Use LogStructured for nested objects
logger.LogStructured(log.LevelInfo, "API request", map[string]any{
    "endpoint": "/api/users",
    "method": "POST",
    "headers": req.Header,
    "duration_ms": elapsed.Milliseconds(),
     })
```

### Raw Output

Outputs raw, unformatted data regardless of configured format:

```go
// Write raw metrics data
logger.Write("METRIC", name, value, "ts", time.Now().Unix())
```

### Consistent Field Names

Use consistent field names across your application:

```go
// Define common fields
const (
    FieldUserID    = "user_id"
    FieldRequestID = "request_id"
    FieldDuration  = "duration_ms"
    FieldError     = "error"
)

// Use consistently
logger.Info("API call",
    FieldRequestID, reqID,
    FieldUserID, userID,
    FieldDuration, elapsed.Milliseconds(),
)
```

### Context Propagation

```go
type contextKey string

const requestIDKey contextKey = "request_id"

func logWithContext(ctx context.Context, logger *log.Logger, level string, msg string, fields ...any) {
    // Extract common fields from context
    if reqID := ctx.Value(requestIDKey); reqID != nil {
        fields = append([]any{"request_id", reqID}, fields...)
    }
    
    switch level {
    case "info":
        logger.Info(msg, fields...)
    case "error":
        logger.Error(msg, fields...)
    }
}
```

## Output Formats

### Text Format (Human-Readable)

Default format for development and debugging:

```
2024-01-15T10:30:45.123456789Z INFO User login user_id=42 email="user@example.com" ip="192.168.1.100"
2024-01-15T10:30:45.234567890Z WARN Rate limit approaching user_id=42 requests=95 limit=100
```

Note: The text format does not add quotes around string values containing spaces. This ensures predictability for simple, space-delimited parsing tools. For logs where maintaining the integrity of such values is critical, `json` format is recommended.

Configuration:
```go
logger.ApplyConfigString(
    "format=txt",
    "show_timestamp=true",
    "show_level=true",
)
```

### JSON Format (Machine-Parseable)

Ideal for log aggregation and analysis:

```json
{"time":"2024-01-15T10:30:45.123456789Z","level":"INFO","fields":["User login","user_id",42,"email","user@example.com","ip","192.168.1.100"]}
{"time":"2024-01-15T10:30:45.234567890Z","level":"WARN","fields":["Rate limit approaching","user_id",42,"requests",95,"limit",100]}
```

Configuration:
```go
logger.ApplyConfigString(
    "format=json",
    "show_timestamp=true",
    "show_level=true",
)
```

## Function Tracing

### Using Trace Methods

Include call stack information for debugging:

```go
func processPayment(amount float64) error {
    logger.InfoTrace(1, "Processing payment", "amount", amount)
    
    if err := validateAmount(amount); err != nil {
        logger.ErrorTrace(3, "Payment validation failed", 
            "amount", amount, 
            "error", err,
        )
        return err
    }
    
    return nil
}
```

Output includes function names:
```
2024-01-15T10:30:45.123456789Z INFO processPayment Processing payment amount=99.99
2024-01-15T10:30:45.234567890Z ERROR validateAmount -> processPayment -> main Payment validation failed amount=-10 error="negative amount"
```

### Trace Depth Guidelines

- `1`: Current function only
- `2-3`: Typical for error paths
- `4-5`: Deep debugging
- `10`: Maximum supported depth

## Error Handling

### Logging Errors

Always include error details in structured fields:

```go
if err := db.Query(sql); err != nil {
    logger.Error("Database query failed",
        "query", sql,
        "error", err.Error(),  // Convert to string
        "error_type", fmt.Sprintf("%T", err),
    )
    return fmt.Errorf("query failed: %w", err)
}
```

### Error Context Pattern

```go
func (s *Service) ProcessOrder(orderID string) error {
    logger := s.logger  // Use service logger
    
    logger.Info("Processing order", "order_id", orderID)
    
    order, err := s.db.GetOrder(orderID)
    if err != nil {
        logger.Error("Failed to fetch order",
            "order_id", orderID,
            "error", err,
            "step", "fetch",
        )
        return fmt.Errorf("fetch order %s: %w", orderID, err)
    }
    
    if err := s.validateOrder(order); err != nil {
        logger.Warn("Order validation failed",
            "order_id", orderID,
            "error", err,
            "step", "validate",
        )
        return fmt.Errorf("validate order %s: %w", orderID, err)
    }
    
    // ... more processing
    
    logger.Info("Order processed successfully", "order_id", orderID)
    return nil
}
```

## Internal Error Handling

The logger may encounter internal errors during operation (e.g., file rotation failures, disk space issues). By default, writing these errors to stderr is disabled, but can be enabled ("internal_errors_to_stderr=true") in configuration for diagnostic purposes.

## Sample Logging Patterns

### Request Lifecycle

```go
func handleRequest(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    reqID := generateRequestID()
    
    logger.Info("Request started",
        "request_id", reqID,
        "method", r.Method,
        "path", r.URL.Path,
        "remote_addr", r.RemoteAddr,
    )
    
    defer func() {
        duration := time.Since(start)
        logger.Info("Request completed",
            "request_id", reqID,
            "duration_ms", duration.Milliseconds(),
        )
    }()
    
    // Handle request...
}
```

### Background Job Pattern

```go
func (w *Worker) processJob(job Job) {
    logger := w.logger
    
    logger.Info("Job started",
        "job_id", job.ID,
        "type", job.Type,
        "scheduled_at", job.ScheduledAt,
    )
    
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()
    
    if err := w.execute(ctx, job); err != nil {
        logger.Error("Job failed",
            "job_id", job.ID,
            "error", err,
            "duration_ms", time.Since(job.StartedAt).Milliseconds(),
        )
        return
    }
    
    logger.Info("Job completed",
        "job_id", job.ID,
        "duration_ms", time.Since(job.StartedAt).Milliseconds(),
    )
}
```

### Metrics Logging

```go
func (m *MetricsCollector) logMetrics() {
    ticker := time.NewTicker(1 * time.Minute)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := m.collect()
        
        m.logger.Info("Metrics snapshot",
            "requests_per_sec", stats.RequestRate,
            "error_rate", stats.ErrorRate,
            "p50_latency_ms", stats.P50Latency,
            "p99_latency_ms", stats.P99Latency,
            "active_connections", stats.ActiveConns,
            "memory_mb", stats.MemoryMB,
        )
    }
}
```

---

[← API Reference](api-reference.md) | [← Back to README](../README.md) | [Disk Management →](disk-management.md)