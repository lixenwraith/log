// FILE: builder.go
package log

// ConfigBuilder provides a fluent API for building logger configurations.
// It wraps a Config instance and provides chainable methods for setting values.
type ConfigBuilder struct {
	cfg *Config
	err error // Accumulate errors for deferred handling
}

// NewConfigBuilder creates a new configuration builder with default values.
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		cfg: DefaultConfig(),
	}
}

// Build returns the built configuration and any accumulated errors.
func (b *ConfigBuilder) Build() (*Config, error) {
	if b.err != nil {
		return nil, b.err
	}
	// Validate the final configuration
	if err := b.cfg.Validate(); err != nil {
		return nil, err
	}
	return b.cfg.Clone(), nil
}

// Level sets the log level.
func (b *ConfigBuilder) Level(level int64) *ConfigBuilder {
	b.cfg.Level = level
	return b
}

// LevelString sets the log level from a string.
func (b *ConfigBuilder) LevelString(level string) *ConfigBuilder {
	if b.err != nil {
		return b
	}
	levelVal, err := Level(level)
	if err != nil {
		b.err = err
		return b
	}
	b.cfg.Level = levelVal
	return b
}

// Directory sets the log directory.
func (b *ConfigBuilder) Directory(dir string) *ConfigBuilder {
	b.cfg.Directory = dir
	return b
}

// Format sets the output format.
func (b *ConfigBuilder) Format(format string) *ConfigBuilder {
	b.cfg.Format = format
	return b
}

// BufferSize sets the channel buffer size.
func (b *ConfigBuilder) BufferSize(size int64) *ConfigBuilder {
	b.cfg.BufferSize = size
	return b
}

// MaxSizeMB sets the maximum log file size in MB.
func (b *ConfigBuilder) MaxSizeMB(size int64) *ConfigBuilder {
	b.cfg.MaxSizeMB = size
	return b
}

// EnableStdout enables mirroring logs to stdout/stderr.
func (b *ConfigBuilder) EnableStdout(enable bool) *ConfigBuilder {
	b.cfg.EnableStdout = enable
	return b
}

// DisableFile disables file output entirely.
func (b *ConfigBuilder) DisableFile(disable bool) *ConfigBuilder {
	b.cfg.DisableFile = disable
	return b
}

// HeartbeatLevel sets the heartbeat monitoring level.
func (b *ConfigBuilder) HeartbeatLevel(level int64) *ConfigBuilder {
	b.cfg.HeartbeatLevel = level
	return b
}

// Example usage:
// cfg, err := log.NewConfigBuilder().
//     Directory("/var/log/app").
//     LevelString("debug").
//     Format("json").
//     BufferSize(4096).
//     EnableStdout(true).
//     Build()