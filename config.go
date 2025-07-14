// FILE: config.go
package log

import (
	"time"
)

// Config holds all logger configuration values
type Config struct {
	// Basic settings
	Level     int64  `toml:"level"`
	Name      string `toml:"name"` // Base name for log files
	Directory string `toml:"directory"`
	Format    string `toml:"format"` // "txt" or "json"
	Extension string `toml:"extension"`

	// Formatting
	ShowTimestamp   bool   `toml:"show_timestamp"`
	ShowLevel       bool   `toml:"show_level"`
	TimestampFormat string `toml:"timestamp_format"` // Time format for log timestamps

	// Buffer and size limits
	BufferSize     int64 `toml:"buffer_size"`       // Channel buffer size
	MaxSizeMB      int64 `toml:"max_size_mb"`       // Max size per log file
	MaxTotalSizeMB int64 `toml:"max_total_size_mb"` // Max total size of all logs in dir
	MinDiskFreeMB  int64 `toml:"min_disk_free_mb"`  // Minimum free disk space required

	// Timers
	FlushIntervalMs    int64   `toml:"flush_interval_ms"`    // Interval for flushing file buffer
	TraceDepth         int64   `toml:"trace_depth"`          // Default trace depth (0-10)
	RetentionPeriodHrs float64 `toml:"retention_period_hrs"` // Hours to keep logs (0=disabled)
	RetentionCheckMins float64 `toml:"retention_check_mins"` // How often to check retention

	// Disk check settings
	DiskCheckIntervalMs    int64 `toml:"disk_check_interval_ms"`   // Base interval for disk checks
	EnableAdaptiveInterval bool  `toml:"enable_adaptive_interval"` // Adjust interval based on log rate
	EnablePeriodicSync     bool  `toml:"enable_periodic_sync"`     // Periodic sync with disk
	MinCheckIntervalMs     int64 `toml:"min_check_interval_ms"`    // Minimum adaptive interval
	MaxCheckIntervalMs     int64 `toml:"max_check_interval_ms"`    // Maximum adaptive interval

	// Heartbeat configuration
	HeartbeatLevel     int64 `toml:"heartbeat_level"`      // 0=disabled, 1=proc only, 2=proc+disk, 3=proc+disk+sys
	HeartbeatIntervalS int64 `toml:"heartbeat_interval_s"` // Interval seconds for heartbeat

	// Stdout/console output settings
	EnableStdout bool   `toml:"enable_stdout"` // Mirror logs to stdout/stderr
	StdoutTarget string `toml:"stdout_target"` // "stdout" or "stderr"
	DisableFile  bool   `toml:"disable_file"`  // Disable file output entirely

	// Internal error handling
	InternalErrorsToStderr bool `toml:"internal_errors_to_stderr"` // Write internal errors to stderr
}

// defaultConfig is the single source for all configurable default values
var defaultConfig = Config{
	// Basic settings
	Level:     LevelInfo,
	Name:      "log",
	Directory: "./logs",
	Format:    "txt",
	Extension: "log",

	// Formatting
	ShowTimestamp:   true,
	ShowLevel:       true,
	TimestampFormat: time.RFC3339Nano,

	// Buffer and size limits
	BufferSize:     1024,
	MaxSizeMB:      10,
	MaxTotalSizeMB: 50,
	MinDiskFreeMB:  100,

	// Timers
	FlushIntervalMs:    100,
	TraceDepth:         0,
	RetentionPeriodHrs: 0.0,
	RetentionCheckMins: 60.0,

	// Disk check settings
	DiskCheckIntervalMs:    5000,
	EnableAdaptiveInterval: true,
	EnablePeriodicSync:     true,
	MinCheckIntervalMs:     100,
	MaxCheckIntervalMs:     60000,

	// Heartbeat settings
	HeartbeatLevel:     0,
	HeartbeatIntervalS: 60,

	// Stdout settings
	EnableStdout: false,
	StdoutTarget: "stdout",
	DisableFile:  false,

	// Internal error handling
	InternalErrorsToStderr: false,
}

// DefaultConfig returns a copy of the default configuration
func DefaultConfig() *Config {
	// Create a copy to prevent modifications to the original
	config := defaultConfig
	return &config
}

// validate performs basic sanity checks on the configuration values.
func (c *Config) validate() error {
	// Individual field validations
	fields := map[string]any{
		"name":                   c.Name,
		"format":                 c.Format,
		"extension":              c.Extension,
		"timestamp_format":       c.TimestampFormat,
		"buffer_size":            c.BufferSize,
		"max_size_mb":            c.MaxSizeMB,
		"max_total_size_mb":      c.MaxTotalSizeMB,
		"min_disk_free_mb":       c.MinDiskFreeMB,
		"flush_interval_ms":      c.FlushIntervalMs,
		"disk_check_interval_ms": c.DiskCheckIntervalMs,
		"min_check_interval_ms":  c.MinCheckIntervalMs,
		"max_check_interval_ms":  c.MaxCheckIntervalMs,
		"trace_depth":            c.TraceDepth,
		"retention_period_hrs":   c.RetentionPeriodHrs,
		"retention_check_mins":   c.RetentionCheckMins,
		"heartbeat_level":        c.HeartbeatLevel,
		"heartbeat_interval_s":   c.HeartbeatIntervalS,
		"stdout_target":          c.StdoutTarget,
		"level":                  c.Level,
	}

	for key, value := range fields {
		if err := validateConfigValue(key, value); err != nil {
			return err
		}
	}

	// Cross-field validations
	if c.MinCheckIntervalMs > c.MaxCheckIntervalMs {
		return fmtErrorf("min_check_interval_ms (%d) cannot be greater than max_check_interval_ms (%d)",
			c.MinCheckIntervalMs, c.MaxCheckIntervalMs)
	}

	if c.HeartbeatLevel > 0 && c.HeartbeatIntervalS <= 0 {
		return fmtErrorf("heartbeat_interval_s must be positive when heartbeat is enabled: %d",
			c.HeartbeatIntervalS)
	}

	return nil
}