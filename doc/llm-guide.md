# lixenwraith/log LLM Usage Guide

High-performance, thread-safe logging library for Go with file rotation, disk management, and compatibility adapters for popular frameworks.

## Core Types

### Logger
```go
// Primary logger instance. All operations are thread-safe.
type Logger struct {
    // Internal fields - thread-safe logging implementation
}
```

### Config
```go
// Logger configuration with validation support.
type Config struct {
    // Basic settings
    Level     int64  `toml:"level"`
    Name      string `toml:"name"`
    Directory string `toml:"directory"`
    Format    string `toml:"format"`     // "txt", "json", or "raw"
    Extension string `toml:"extension"`

    // Formatting
    ShowTimestamp   bool   `toml:"show_timestamp"`
    ShowLevel       bool   `toml:"show_level"`
    TimestampFormat string `toml:"timestamp_format"`

    // Buffer and size limits
    BufferSize     int64 `toml:"buffer_size"`
    MaxSizeMB      int64 `toml:"max_size_mb"`
    MaxTotalSizeMB int64 `toml:"max_total_size_mb"`
    MinDiskFreeMB  int64 `toml:"min_disk_free_mb"`

    // Timers
    FlushIntervalMs    int64   `toml:"flush_interval_ms"`
    TraceDepth         int64   `toml:"trace_depth"`
    RetentionPeriodHrs float64 `toml:"retention_period_hrs"`
    RetentionCheckMins float64 `toml:"retention_check_mins"`

    // Disk check settings
    DiskCheckIntervalMs    int64 `toml:"disk_check_interval_ms"`
    EnableAdaptiveInterval bool  `toml:"enable_adaptive_interval"`
    EnablePeriodicSync     bool  `toml:"enable_periodic_sync"`
    MinCheckIntervalMs     int64 `toml:"min_check_interval_ms"`
    MaxCheckIntervalMs     int64 `toml:"max_check_interval_ms"`

    // Heartbeat configuration
    HeartbeatLevel     int64 `toml:"heartbeat_level"`
    HeartbeatIntervalS int64 `toml:"heartbeat_interval_s"`

    // Stdout/console output settings
    EnableStdout bool   `toml:"enable_stdout"`
    StdoutTarget string `toml:"stdout_target"` // "stdout", "stderr", or "split"
    DisableFile  bool   `toml:"disable_file"`

    // Internal error handling
    InternalErrorsToStderr bool `toml:"internal_errors_to_stderr"`
}
```

## Constants

### Log Levels
```go
const (
    LevelDebug int64 = -4
    LevelInfo  int64 = 0
    LevelWarn  int64 = 4
    LevelError int64 = 8
)
```

### Heartbeat Levels
```go
const (
    LevelProc int64 = 12  // Process statistics
    LevelDisk int64 = 16  // Disk usage statistics
    LevelSys  int64 = 20  // System statistics
)
```

## Core Methods

### Creation
```go
func NewLogger() *Logger
func DefaultConfig() *Config
```

### Configuration
```go
func (l *Logger) ApplyConfig(cfg *Config) error
func (l *Logger) ApplyOverride(overrides ...string) error
func (l *Logger) GetConfig() *Config
```

### Logging Methods
```go
func (l *Logger) Debug(args ...any)
func (l *Logger) Info(args ...any)
func (l *Logger) Warn(args ...any)
func (l *Logger) Error(args ...any)
func (l *Logger) LogStructured(level int64, message string, fields map[string]any)
func (l *Logger) Write(args ...any)  // Raw output, no formatting
func (l *Logger) Log(args ...any)     // Timestamp only, no level
func (l *Logger) Message(args ...any) // No timestamp or level
```

### Trace Logging
```go
func (l *Logger) DebugTrace(depth int, args ...any)
func (l *Logger) InfoTrace(depth int, args ...any)
func (l *Logger) WarnTrace(depth int, args ...any)
func (l *Logger) ErrorTrace(depth int, args ...any)
func (l *Logger) LogTrace(depth int, args ...any)
```

### Control Methods
```go
func (l *Logger) Shutdown(timeout ...time.Duration) error
func (l *Logger) Flush(timeout time.Duration) error
```

### Utilities
```go
func Level(levelStr string) (int64, error)
```

## Configuration Builder

### ConfigBuilder
```go
type ConfigBuilder struct {
    // Internal builder state
}
```

### Builder Methods
```go
func NewConfigBuilder() *ConfigBuilder
func (b *ConfigBuilder) Build() (*Config, error)
func (b *ConfigBuilder) Level(level int64) *ConfigBuilder
func (b *ConfigBuilder) LevelString(level string) *ConfigBuilder
func (b *ConfigBuilder) Directory(dir string) *ConfigBuilder
func (b *ConfigBuilder) Format(format string) *ConfigBuilder
func (b *ConfigBuilder) BufferSize(size int64) *ConfigBuilder
func (b *ConfigBuilder) MaxSizeMB(size int64) *ConfigBuilder
func (b *ConfigBuilder) EnableStdout(enable bool) *ConfigBuilder
func (b *ConfigBuilder) DisableFile(disable bool) *ConfigBuilder
func (b *ConfigBuilder) HeartbeatLevel(level int64) *ConfigBuilder
func (b *ConfigBuilder) HeartbeatIntervalS(seconds int64) *ConfigBuilder
```

## Compatibility Adapters (log/compat)

### Builder
```go
type Builder struct {
    // Internal adapter builder state
}
```

### Builder Methods
```go
func NewBuilder() *Builder
func (b *Builder) WithLogger(l *log.Logger) *Builder
func (b *Builder) WithConfig(cfg *log.Config) *Builder
func (b *Builder) BuildGnet(opts ...GnetOption) (*GnetAdapter, error)
func (b *Builder) BuildStructuredGnet(opts ...GnetOption) (*StructuredGnetAdapter, error)
func (b *Builder) BuildFastHTTP(opts ...FastHTTPOption) (*FastHTTPAdapter, error)
func (b *Builder) GetLogger() (*log.Logger, error)
```

### gnet Adapters
```go
type GnetAdapter struct {
    // Implements gnet.Logger interface
}

type StructuredGnetAdapter struct {
    *GnetAdapter
    // Enhanced with field extraction
}

type GnetOption func(*GnetAdapter)
func WithFatalHandler(handler func(string)) GnetOption
```

### gnet Interface Implementation
```go
func (a *GnetAdapter) Debugf(format string, args ...any)
func (a *GnetAdapter) Infof(format string, args ...any)
func (a *GnetAdapter) Warnf(format string, args ...any)
func (a *GnetAdapter) Errorf(format string, args ...any)
func (a *GnetAdapter) Fatalf(format string, args ...any)
```

### fasthttp Adapter
```go
type FastHTTPAdapter struct {
    // Implements fasthttp.Logger interface
}

type FastHTTPOption func(*FastHTTPAdapter)
func WithDefaultLevel(level int64) FastHTTPOption
func WithLevelDetector(detector func(string) int64) FastHTTPOption
```

### fasthttp Interface Implementation
```go
func (a *FastHTTPAdapter) Printf(format string, args ...any)
```

### Helper Functions
```go
func NewGnetAdapter(logger *log.Logger, opts ...GnetOption) *GnetAdapter
func NewStructuredGnetAdapter(logger *log.Logger, opts ...GnetOption) *StructuredGnetAdapter
func NewFastHTTPAdapter(logger *log.Logger, opts ...FastHTTPOption) *FastHTTPAdapter
func DetectLogLevel(msg string) int64
```

## File Management

### Rotation
Files rotate automatically when `MaxSizeMB` is reached. Rotated files use naming pattern: `{name}_{YYMMDD}_{HHMMSS}_{nanoseconds}.{extension}`

### Disk Management
- Enforces `MaxTotalSizeMB` for total log directory size
- Maintains `MinDiskFreeMB` free disk space
- Deletes oldest logs when limits exceeded

### Retention
- Time-based cleanup with `RetentionPeriodHrs`
- Periodic checks via `RetentionCheckMins`

## Heartbeat Monitoring

### Levels
- **0**: Disabled (default)
- **1**: Process stats (logs processed, dropped, uptime)
- **2**: + Disk stats (rotations, deletions, sizes, free space)
- **3**: + System stats (memory, GC, goroutines)

### Output
Heartbeats bypass log level filtering and use special levels (PROC, DISK, SYS).

## Output Formats

### Text Format
Human-readable with configurable timestamp and level display.

### JSON Format
Machine-parseable with structured fields array.

### Raw Format
Space-separated values without metadata, triggered by `Write()` method or `format=raw`.

## Thread Safety
All public methods are thread-safe. Concurrent logging from multiple goroutines is supported without external synchronization.

## Configuration Overrides
String key-value pairs for runtime configuration changes:
```
"level=-4"              // Numeric level
"level=debug"           // Named level
"directory=/var/log"    // String value
"buffer_size=2048"      // Integer value
"enable_stdout=true"    // Boolean value
```

## Error Handling
- Configuration errors prefixed with "log: "
- Failed initialization disables logger
- Dropped logs tracked and reported periodically
- Internal errors optionally written to stderr

## Performance Characteristics
- Non-blocking log submission (buffered channel)
- Adaptive disk checking based on load
- Batch file writes with configurable flush interval
- Automatic log dropping under extreme load with tracking