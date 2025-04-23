// --- File: config.go ---
package log

import (
	"strings"
)

// Config holds all logger configuration values, populated via config.UnmarshalSubtree
type Config struct {
	// Basic settings
	Level     int64  `toml:"level"`
	Name      string `toml:"name"`
	Directory string `toml:"directory"`
	Format    string `toml:"format"` // "txt" or "json"
	Extension string `toml:"extension"`

	// Formatting
	ShowTimestamp bool `toml:"show_timestamp"`
	ShowLevel     bool `toml:"show_level"`

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
}

// defaultConfig is the single source of truth for all default values
var defaultConfig = Config{
	// Basic settings
	Level:     LevelInfo,
	Name:      "log",
	Directory: "./logs",
	Format:    "txt",
	Extension: "log",

	// Formatting
	ShowTimestamp: true,
	ShowLevel:     true,

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
	HeartbeatLevel:     0,  // Disabled by default
	HeartbeatIntervalS: 60, // Default to 60 seconds if enabled
}

// DefaultConfig returns a copy of the default configuration
func DefaultConfig() *Config {
	// Create a copy to prevent modifications to the original
	config := defaultConfig
	return &config
}

// validate performs basic sanity checks on the configuration values.
func (c *Config) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmtErrorf("log name cannot be empty")
	}
	if c.Format != "txt" && c.Format != "json" {
		return fmtErrorf("invalid format: '%s' (use txt or json)", c.Format)
	}
	if strings.HasPrefix(c.Extension, ".") {
		return fmtErrorf("extension should not start with dot: %s", c.Extension)
	}
	if c.BufferSize <= 0 {
		return fmtErrorf("buffer_size must be positive: %d", c.BufferSize)
	}
	if c.MaxSizeMB < 0 {
		return fmtErrorf("max_size_mb cannot be negative: %d", c.MaxSizeMB)
	}
	if c.MaxTotalSizeMB < 0 {
		return fmtErrorf("max_total_size_mb cannot be negative: %d", c.MaxTotalSizeMB)
	}
	if c.MinDiskFreeMB < 0 {
		return fmtErrorf("min_disk_free_mb cannot be negative: %d", c.MinDiskFreeMB)
	}
	if c.FlushIntervalMs <= 0 {
		return fmtErrorf("flush_interval_ms must be positive milliseconds: %d", c.FlushIntervalMs)
	}
	if c.DiskCheckIntervalMs <= 0 {
		return fmtErrorf("disk_check_interval_ms must be positive milliseconds: %d", c.DiskCheckIntervalMs)
	}
	if c.MinCheckIntervalMs <= 0 {
		return fmtErrorf("min_check_interval_ms must be positive milliseconds: %d", c.MinCheckIntervalMs)
	}
	if c.MaxCheckIntervalMs <= 0 {
		return fmtErrorf("max_check_interval_ms must be positive milliseconds: %d", c.MaxCheckIntervalMs)
	}
	if c.MinCheckIntervalMs > c.MaxCheckIntervalMs {
		return fmtErrorf("min_check_interval_ms (%d) cannot be greater than max_check_interval_ms (%d)", c.MinCheckIntervalMs, c.MaxCheckIntervalMs)
	}
	if c.TraceDepth < 0 || c.TraceDepth > 10 {
		return fmtErrorf("trace_depth must be between 0 and 10: %d", c.TraceDepth)
	}
	if c.RetentionPeriodHrs < 0 {
		return fmtErrorf("retention_period_hrs cannot be negative: %f", c.RetentionPeriodHrs)
	}
	if c.RetentionCheckMins < 0 {
		return fmtErrorf("retention_check_mins cannot be negative: %f", c.RetentionCheckMins)
	}
	if c.HeartbeatLevel < 0 || c.HeartbeatLevel > 3 {
		return fmtErrorf("heartbeat_level must be between 0 and 3: %d", c.HeartbeatLevel)
	}
	if c.HeartbeatLevel > 0 && c.HeartbeatIntervalS <= 0 {
		return fmtErrorf("heartbeat_interval_s must be positive when heartbeat is enabled: %d", c.HeartbeatIntervalS)
	}
	return nil
}