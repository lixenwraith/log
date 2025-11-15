// FILE: lixenwraith/log/config.go
package log

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/lixenwraith/log/sanitizer"
)

// Config holds all logger configuration values
type Config struct {
	// File and Console output settings
	EnableConsole bool   `toml:"enable_console"` // Enable console output (stdout/stderr)
	ConsoleTarget string `toml:"console_target"` // "stdout", "stderr", or "split"
	EnableFile    bool   `toml:"enable_file"`    // Enable file output

	// Basic settings
	Level     int64  `toml:"level"`     // Log records at or above this Level will be logged
	Name      string `toml:"name"`      // Base name for log files
	Directory string `toml:"directory"` // Directory for log files
	Extension string `toml:"extension"` // Log file extension

	// Formatting
	Format          string         `toml:"format"`           // "txt", "raw", or "json"
	ShowTimestamp   bool           `toml:"show_timestamp"`   // Add timestamp to log records
	ShowLevel       bool           `toml:"show_level"`       // Add level to log record
	TimestampFormat string         `toml:"timestamp_format"` // Time format for log timestamps
	Sanitization    sanitizer.Mode `toml:"sanitization"`     // 0=None, 1=HexEncode, 2=Strip, 3=Escape

	// Buffer and size limits
	BufferSize     int64 `toml:"buffer_size"`       // Channel buffer size
	MaxSizeKB      int64 `toml:"max_size_kb"`       // Max size per log file
	MaxTotalSizeKB int64 `toml:"max_total_size_kb"` // Max total size of all logs in dir
	MinDiskFreeKB  int64 `toml:"min_disk_free_kb"`  // Minimum free disk space required

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

	// Internal error handling
	InternalErrorsToStderr bool `toml:"internal_errors_to_stderr"` // Write internal errors to stderr
}

// defaultConfig is the single source for all configurable default values
var defaultConfig = Config{
	// Output settings
	EnableConsole: true,
	ConsoleTarget: "stdout",
	EnableFile:    true,

	// File settings
	Level:     LevelInfo,
	Name:      "log",
	Directory: "./log",
	Extension: "log",

	// Formatting
	Format:          "txt",
	ShowTimestamp:   true,
	ShowLevel:       true,
	TimestampFormat: time.RFC3339Nano,
	Sanitization:    sanitizer.HexEncode,

	// Buffer and size limits
	BufferSize:     1024,
	MaxSizeKB:      1000,
	MaxTotalSizeKB: 5000,
	MinDiskFreeKB:  10000,

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

	// Internal error handling
	InternalErrorsToStderr: false,
}

// DefaultConfig returns a copy of the default configuration
func DefaultConfig() *Config {
	// Create a copy to prevent modifications to the original
	return defaultConfig.Clone()
}

// Clone creates a deep copy of the configuration
func (c *Config) Clone() *Config {
	copiedConfig := *c
	return &copiedConfig
}

// Validate performs validation on the configuration
func (c *Config) Validate() error {
	// String validations
	if strings.TrimSpace(c.Name) == "" {
		return fmtErrorf("log name cannot be empty")
	}

	if c.Format != "txt" && c.Format != "json" && c.Format != "raw" {
		return fmtErrorf("invalid format: '%s' (use txt, json, or raw)", c.Format)
	}

	// TODO: better bound check, implement validator in `sanitizer`
	if c.Sanitization < 0 || c.Sanitization > sanitizer.Escape {
		return fmtErrorf("invalid sanitization mode: '%d' (use 0=None, 1=HexEncode, 2=Strip, 3=Escape)", c.Sanitization)
	}

	if strings.HasPrefix(c.Extension, ".") {
		return fmtErrorf("extension should not start with dot: %s", c.Extension)
	}

	if strings.TrimSpace(c.TimestampFormat) == "" {
		return fmtErrorf("timestamp_format cannot be empty")
	}

	if c.ConsoleTarget != "stdout" && c.ConsoleTarget != "stderr" && c.ConsoleTarget != "split" {
		return fmtErrorf("invalid console_target: '%s' (use stdout, stderr, or split)", c.ConsoleTarget)
	}

	// Numeric validations
	if c.BufferSize <= 0 {
		return fmtErrorf("buffer_size must be positive: %d", c.BufferSize)
	}

	if c.MaxSizeKB < 0 || c.MaxTotalSizeKB < 0 || c.MinDiskFreeKB < 0 {
		return fmtErrorf("size limits cannot be negative")
	}

	if c.FlushIntervalMs <= 0 || c.DiskCheckIntervalMs <= 0 ||
		c.MinCheckIntervalMs <= 0 || c.MaxCheckIntervalMs <= 0 {
		return fmtErrorf("interval settings must be positive")
	}

	if c.TraceDepth < 0 || c.TraceDepth > 10 {
		return fmtErrorf("trace_depth must be between 0 and 10: %d", c.TraceDepth)
	}

	if c.RetentionPeriodHrs < 0 || c.RetentionCheckMins < 0 {
		return fmtErrorf("retention settings cannot be negative")
	}

	if c.HeartbeatLevel < 0 || c.HeartbeatLevel > 3 {
		return fmtErrorf("heartbeat_level must be between 0 and 3: %d", c.HeartbeatLevel)
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

// applyConfigField applies a single key-value override to a Config
// This is the core field mapping logic for string overrides
func applyConfigField(cfg *Config, key, value string) error {
	switch key {
	// Basic settings
	case "level":
		// Special handling: accept both numeric and named values
		if numVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			cfg.Level = numVal
		} else {
			// Try parsing as named level
			levelVal, err := Level(value)
			if err != nil {
				return fmtErrorf("invalid level value '%s': %w", value, err)
			}
			cfg.Level = levelVal
		}
	case "name":
		cfg.Name = value
	case "directory":
		cfg.Directory = value
	case "extension":
		cfg.Extension = value

	// Formatting
	case "format":
		cfg.Format = value
	case "show_timestamp":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for show_timestamp '%s': %w", value, err)
		}
		cfg.ShowTimestamp = boolVal
	case "show_level":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for show_level '%s': %w", value, err)
		}
		cfg.ShowLevel = boolVal
	case "timestamp_format":
		cfg.TimestampFormat = value
	case "sanitization":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for sanitization '%s': %w", value, err)
		}
		cfg.Sanitization = sanitizer.Mode(intVal)

	// Buffer and size limits
	case "buffer_size":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for buffer_size '%s': %w", value, err)
		}
		cfg.BufferSize = intVal
	case "max_size_kb":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for max_size_kb '%s': %w", value, err)
		}
		cfg.MaxSizeKB = intVal
	case "max_total_size_kb":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for max_total_size_kb '%s': %w", value, err)
		}
		cfg.MaxTotalSizeKB = intVal
	case "min_disk_free_kb":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for min_disk_free_kb '%s': %w", value, err)
		}
		cfg.MinDiskFreeKB = intVal

	// Timers
	case "flush_interval_ms":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for flush_interval_ms '%s': %w", value, err)
		}
		cfg.FlushIntervalMs = intVal
	case "trace_depth":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for trace_depth '%s': %w", value, err)
		}
		cfg.TraceDepth = intVal
	case "retention_period_hrs":
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmtErrorf("invalid float value for retention_period_hrs '%s': %w", value, err)
		}
		cfg.RetentionPeriodHrs = floatVal
	case "retention_check_mins":
		floatVal, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmtErrorf("invalid float value for retention_check_mins '%s': %w", value, err)
		}
		cfg.RetentionCheckMins = floatVal

	// Disk check settings
	case "disk_check_interval_ms":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for disk_check_interval_ms '%s': %w", value, err)
		}
		cfg.DiskCheckIntervalMs = intVal
	case "enable_adaptive_interval":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for enable_adaptive_interval '%s': %w", value, err)
		}
		cfg.EnableAdaptiveInterval = boolVal
	case "enable_periodic_sync":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for enable_periodic_sync '%s': %w", value, err)
		}
		cfg.EnablePeriodicSync = boolVal
	case "min_check_interval_ms":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for min_check_interval_ms '%s': %w", value, err)
		}
		cfg.MinCheckIntervalMs = intVal
	case "max_check_interval_ms":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for max_check_interval_ms '%s': %w", value, err)
		}
		cfg.MaxCheckIntervalMs = intVal

	// Heartbeat configuration
	case "heartbeat_level":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for heartbeat_level '%s': %w", value, err)
		}
		cfg.HeartbeatLevel = intVal
	case "heartbeat_interval_s":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for heartbeat_interval_s '%s': %w", value, err)
		}
		cfg.HeartbeatIntervalS = intVal

	// Console output settings
	case "enable_console":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for enable_console '%s': %w", value, err)
		}
		cfg.EnableConsole = boolVal
	case "console_target":
		cfg.ConsoleTarget = value
	case "enable_file":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for enable_file '%s': %w", value, err)
		}
		cfg.EnableFile = boolVal

	// Internal error handling
	case "internal_errors_to_stderr":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for internal_errors_to_stderr '%s': %w", value, err)
		}
		cfg.InternalErrorsToStderr = boolVal

	default:
		return fmtErrorf("unknown configuration key '%s'", key)
	}

	return nil
}

// configRequiresRestart checks if config changes require processor restart
func configRequiresRestart(oldCfg, newCfg *Config) bool {
	// Channel size change requires restart
	if oldCfg.BufferSize != newCfg.BufferSize {
		return true
	}

	// File output changes require restart
	if oldCfg.EnableFile != newCfg.EnableFile {
		return true
	}

	// Directory or file naming changes require restart
	if oldCfg.Directory != newCfg.Directory ||
		oldCfg.Name != newCfg.Name ||
		oldCfg.Extension != newCfg.Extension {
		return true
	}

	// Timer changes require restart
	if oldCfg.FlushIntervalMs != newCfg.FlushIntervalMs ||
		oldCfg.DiskCheckIntervalMs != newCfg.DiskCheckIntervalMs ||
		oldCfg.EnableAdaptiveInterval != newCfg.EnableAdaptiveInterval ||
		oldCfg.HeartbeatIntervalS != newCfg.HeartbeatIntervalS ||
		oldCfg.HeartbeatLevel != newCfg.HeartbeatLevel ||
		oldCfg.RetentionCheckMins != newCfg.RetentionCheckMins ||
		oldCfg.RetentionPeriodHrs != newCfg.RetentionPeriodHrs {
		return true
	}

	return false
}

// combineConfigErrors combines multiple configuration errors into a single error.
func combineConfigErrors(errors []error) error {
	if len(errors) == 0 {
		return nil
	}
	if len(errors) == 1 {
		return errors[0]
	}

	var sb strings.Builder
	sb.WriteString("log: multiple configuration errors:")
	for i, err := range errors {
		errMsg := err.Error()
		// Remove "log: " prefix from individual errors to avoid duplication
		if strings.HasPrefix(errMsg, "log: ") {
			errMsg = errMsg[5:]
		}
		sb.WriteString(fmt.Sprintf("\n  %d. %s", i+1, errMsg))
	}
	return fmt.Errorf("%s", sb.String())
}