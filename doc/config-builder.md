# Builder Pattern Guide

The ConfigBuilder provides a fluent API for constructing logger configurations with compile-time safety and deferred validation.

## Creating a Builder

NewConfigBuilder creates a new configuration builder initialized with default values.

```go
func NewConfigBuilder() *ConfigBuilder
```

```go
builder := log.NewConfigBuilder()
```

## Builder Methods

All builder methods return `*ConfigBuilder` for chaining. Errors are accumulated and returned by `Build()`.

### Common Methods

| Method | Parameters | Description |
|--------|------------|-------------|
| `Level(level int64)` | `level`: Numeric log level | Sets log level (-4 to 8) |
| `LevelString(level string)` | `level`: Named level | Sets level by name ("debug", "info", etc.) |
| `Directory(dir string)` | `dir`: Path | Sets log directory |
| `Format(format string)` | `format`: Output format | Sets format ("txt", "json", "raw") |
| `BufferSize(size int64)` | `size`: Buffer size | Sets channel buffer size |
| `MaxSizeKB(size int64)` | `size`: Size in MB | Sets max file size |
| `EnableStdout(enable bool)` | `enable`: Boolean | Enables console output |
| `DisableFile(disable bool)` | `disable`: Boolean | Disables file output |
| `HeartbeatLevel(level int64)` | `level`: 0-3 | Sets monitoring level |

## Build

```go
func (b *ConfigBuilder) Build() (*Config, error)
```

Validates builder configuration and returns logger config.
Returns accumulated errors if any builder operations failed.

```go
cfg, err := builder.Build()
if err != nil {
    // Handle validation or conversion errors
}
```

## Usage pattern

```go
logger := log.NewLogger()

cfg, err := log.NewConfigBuilder().
    Directory("/var/log/app").
    Format("json").
    LevelString("debug").
    Build()

if err != nil {
    return err
}

err = logger.ApplyConfig(cfg)
```

---

[← Configuration](configuration.md) | [← Back to README](../README.md) | [API Reference →](api-reference.md)