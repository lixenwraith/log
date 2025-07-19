// FILE: override.go
package log

import (
	"fmt"
	"strconv"
	"strings"
)

// ApplyOverride applies string key-value overrides to the logger's current configuration.
// Each override should be in the format "key=value".
// The configuration is cloned before modification to ensure thread safety.
//
// Example:
//
//	logger := log.NewLogger()
//	err := logger.ApplyOverride(
//	    "directory=/var/log/app",
//	    "level=-4",
//	    "format=json",
//	)
func (l *Logger) ApplyOverride(overrides ...string) error {
	cfg := l.getConfig().Clone()

	var errors []error

	for _, override := range overrides {
		key, value, err := parseKeyValue(override)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		if err := applyConfigField(cfg, key, value); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return combineConfigErrors(errors)
	}

	return l.ApplyConfig(cfg)
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

// applyConfigField applies a single key-value override to a Config.
// This is the core field mapping logic for string overrides.
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
	case "format":
		cfg.Format = value
	case "extension":
		cfg.Extension = value

	// Formatting
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

	// Buffer and size limits
	case "buffer_size":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for buffer_size '%s': %w", value, err)
		}
		cfg.BufferSize = intVal
	case "max_size_mb":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for max_size_mb '%s': %w", value, err)
		}
		cfg.MaxSizeMB = intVal
	case "max_total_size_mb":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for max_total_size_mb '%s': %w", value, err)
		}
		cfg.MaxTotalSizeMB = intVal
	case "min_disk_free_mb":
		intVal, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmtErrorf("invalid integer value for min_disk_free_mb '%s': %w", value, err)
		}
		cfg.MinDiskFreeMB = intVal

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

	// Stdout/console output settings
	case "enable_stdout":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for enable_stdout '%s': %w", value, err)
		}
		cfg.EnableStdout = boolVal
	case "stdout_target":
		cfg.StdoutTarget = value
	case "disable_file":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmtErrorf("invalid boolean value for disable_file '%s': %w", value, err)
		}
		cfg.DisableFile = boolVal

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