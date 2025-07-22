// FILE: lixenwraith/log/builder.go
package log

// Builder provides a fluent API for building logger configurations.
// It wraps a Config instance and provides chainable methods for setting values.
type Builder struct {
	cfg *Config
	err error // Accumulate errors for deferred handling
}

// NewBuilder creates a new configuration builder with default values.
func NewBuilder() *Builder {
	return &Builder{
		cfg: DefaultConfig(),
	}
}

// Build creates a new Logger instance with the specified configuration.
func (b *Builder) Build() (*Logger, error) {
	if b.err != nil {
		return nil, b.err
	}

	// Create a new logger.
	logger := NewLogger()

	// Apply the built configuration. ApplyConfig handles all initialization and validation.
	if err := logger.ApplyConfig(b.cfg); err != nil {
		return nil, err
	}

	return logger, nil
}

// Level sets the log level.
func (b *Builder) Level(level int64) *Builder {
	b.cfg.Level = level
	return b
}

// LevelString sets the log level from a string.
func (b *Builder) LevelString(level string) *Builder {
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

// Name sets the log level.
func (b *Builder) Name(name string) *Builder {
	b.cfg.Name = name
	return b
}

// Directory sets the log directory.
func (b *Builder) Directory(dir string) *Builder {
	b.cfg.Directory = dir
	return b
}

// Format sets the output format.
func (b *Builder) Format(format string) *Builder {
	b.cfg.Format = format
	return b
}

// Extension sets the log level.
func (b *Builder) Extension(ext string) *Builder {
	b.cfg.Extension = ext
	return b
}

// BufferSize sets the channel buffer size.
func (b *Builder) BufferSize(size int64) *Builder {
	b.cfg.BufferSize = size
	return b
}

// MaxSizeKB sets the maximum log file size in KB.
func (b *Builder) MaxSizeKB(size int64) *Builder {
	b.cfg.MaxSizeKB = size
	return b
}

// MaxSizeMB sets the maximum log file size in MB. Convenience.
func (b *Builder) MaxSizeMB(size int64) *Builder {
	b.cfg.MaxSizeKB = size * 1000
	return b
}

// EnableStdout enables mirroring logs to stdout/stderr.
func (b *Builder) EnableStdout(enable bool) *Builder {
	b.cfg.EnableStdout = enable
	return b
}

// DisableFile disables file output entirely.
func (b *Builder) DisableFile(disable bool) *Builder {
	b.cfg.DisableFile = disable
	return b
}

// HeartbeatLevel sets the heartbeat monitoring level.
func (b *Builder) HeartbeatLevel(level int64) *Builder {
	b.cfg.HeartbeatLevel = level
	return b
}

// HeartbeatIntervalS sets the heartbeat monitoring level.
func (b *Builder) HeartbeatIntervalS(interval int64) *Builder {
	b.cfg.HeartbeatIntervalS = interval
	return b
}

// Example usage:
// logger, err := log.NewBuilder().
//
//	Directory("/var/log/app").
//	LevelString("debug").
//	Format("json").
//	BufferSize(4096).
//	EnableStdout(true).
//	Build()
//
// if err == nil {
//
//	 defer logger.Shutdown()
//	 logger.Info("Logger initialized successfully")
//
// }