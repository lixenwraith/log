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

// EnableFile enables file output.
func (b *Builder) EnableFile(enable bool) *Builder {
	b.cfg.EnableFile = enable
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

// ShowTimestamp sets whether to show timestamps in logs.
func (b *Builder) ShowTimestamp(show bool) *Builder {
	b.cfg.ShowTimestamp = show
	return b
}

// ShowLevel sets whether to show log levels.
func (b *Builder) ShowLevel(show bool) *Builder {
	b.cfg.ShowLevel = show
	return b
}

// TimestampFormat sets the timestamp format string.
func (b *Builder) TimestampFormat(format string) *Builder {
	b.cfg.TimestampFormat = format
	return b
}

// MaxTotalSizeKB sets the maximum total size of all log files in KB.
func (b *Builder) MaxTotalSizeKB(size int64) *Builder {
	b.cfg.MaxTotalSizeKB = size
	return b
}

// MaxTotalSizeMB sets the maximum total size of all log files in MB. Convenience.
func (b *Builder) MaxTotalSizeMB(size int64) *Builder {
	b.cfg.MaxTotalSizeKB = size * 1000
	return b
}

// MinDiskFreeKB sets the minimum required free disk space in KB.
func (b *Builder) MinDiskFreeKB(size int64) *Builder {
	b.cfg.MinDiskFreeKB = size
	return b
}

// MinDiskFreeMB sets the minimum required free disk space in MB. Convenience.
func (b *Builder) MinDiskFreeMB(size int64) *Builder {
	b.cfg.MinDiskFreeKB = size * 1000
	return b
}

// FlushIntervalMs sets the flush interval in milliseconds.
func (b *Builder) FlushIntervalMs(interval int64) *Builder {
	b.cfg.FlushIntervalMs = interval
	return b
}

// TraceDepth sets the default trace depth for stack traces.
func (b *Builder) TraceDepth(depth int64) *Builder {
	b.cfg.TraceDepth = depth
	return b
}

// RetentionPeriodHrs sets the log retention period in hours.
func (b *Builder) RetentionPeriodHrs(hours float64) *Builder {
	b.cfg.RetentionPeriodHrs = hours
	return b
}

// RetentionCheckMins sets the retention check interval in minutes.
func (b *Builder) RetentionCheckMins(mins float64) *Builder {
	b.cfg.RetentionCheckMins = mins
	return b
}

// DiskCheckIntervalMs sets the disk check interval in milliseconds.
func (b *Builder) DiskCheckIntervalMs(interval int64) *Builder {
	b.cfg.DiskCheckIntervalMs = interval
	return b
}

// EnableAdaptiveInterval enables adaptive disk check intervals.
func (b *Builder) EnableAdaptiveInterval(enable bool) *Builder {
	b.cfg.EnableAdaptiveInterval = enable
	return b
}

// EnablePeriodicSync enables periodic file sync.
func (b *Builder) EnablePeriodicSync(enable bool) *Builder {
	b.cfg.EnablePeriodicSync = enable
	return b
}

// MinCheckIntervalMs sets the minimum disk check interval in milliseconds.
func (b *Builder) MinCheckIntervalMs(interval int64) *Builder {
	b.cfg.MinCheckIntervalMs = interval
	return b
}

// MaxCheckIntervalMs sets the maximum disk check interval in milliseconds.
func (b *Builder) MaxCheckIntervalMs(interval int64) *Builder {
	b.cfg.MaxCheckIntervalMs = interval
	return b
}

// ConsoleTarget sets the console output target ("stdout", "stderr", or "split").
func (b *Builder) ConsoleTarget(target string) *Builder {
	b.cfg.ConsoleTarget = target
	return b
}

// InternalErrorsToStderr sets whether to write internal errors to stderr.
func (b *Builder) InternalErrorsToStderr(enable bool) *Builder {
	b.cfg.InternalErrorsToStderr = enable
	return b
}

// EnableConsole enables console output.
func (b *Builder) EnableConsole(enable bool) *Builder {
	b.cfg.EnableConsole = enable
	return b
}

// Example usage:
// logger, err := log.NewBuilder().
//
//	Directory("/var/log/app").
//	LevelString("debug").
//	Format("json").
//	BufferSize(4096).
//	EnableConsole(true).
//	Build()
//
// if err == nil {
//
//	 defer logger.Shutdown()
//	 logger.Info("Logger initialized successfully")
//
// }