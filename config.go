// FILE: config.go
package log

import (
	"errors"
	"fmt"
	"github.com/lixenwraith/config"
	"reflect"
	"strings"
	"time"
)

// Config holds all logger configuration values
type Config struct {
	// Basic settings
	Level     int64  `toml:"level"`
	Name      string `toml:"name"` // Base name for log files
	Directory string `toml:"directory"`
	Format    string `toml:"format"` // "txt", "raw", or "json"
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
	copiedConfig := defaultConfig
	return &copiedConfig
}

// NewConfigFromFile loads configuration from a TOML file and returns a validated Config
func NewConfigFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()

	// Use lixenwraith/config as a loader
	loader := config.New()

	// Register the struct to enable proper unmarshaling
	if err := loader.RegisterStruct("log.", *cfg); err != nil {
		return nil, fmt.Errorf("failed to register config struct: %w", err)
	}

	// Load from file (handles file not found gracefully)
	if err := loader.Load(path, nil); err != nil && !errors.Is(err, config.ErrConfigNotFound) {
		return nil, fmt.Errorf("failed to load config from %s: %w", path, err)
	}

	// Extract values into our Config struct
	if err := extractConfig(loader, "log.", cfg); err != nil {
		return nil, fmt.Errorf("failed to extract config values: %w", err)
	}

	// Validate the loaded configuration
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// NewConfigFromDefaults creates a Config with default values and applies overrides
func NewConfigFromDefaults(overrides map[string]any) (*Config, error) {
	cfg := DefaultConfig()

	// Apply overrides using reflection
	if err := applyOverrides(cfg, overrides); err != nil {
		return nil, fmt.Errorf("failed to apply overrides: %w", err)
	}

	// Validate the configuration
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// extractConfig extracts values from lixenwraith/config into our Config struct
func extractConfig(loader *config.Config, prefix string, cfg *Config) error {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Get the toml tag to determine the config key
		tomlTag := field.Tag.Get("toml")
		if tomlTag == "" {
			continue
		}

		key := prefix + tomlTag

		// Get value from loader
		val, found := loader.Get(key)
		if !found {
			continue // Use default value
		}

		// Set the field value with type conversion
		if err := setFieldValue(fieldValue, val); err != nil {
			return fmt.Errorf("failed to set field %s: %w", field.Name, err)
		}
	}

	return nil
}

// applyOverrides applies a map of overrides to the Config struct
func applyOverrides(cfg *Config, overrides map[string]any) error {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	// Create a map of field names to field values for efficient lookup
	fieldMap := make(map[string]reflect.Value)
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tomlTag := field.Tag.Get("toml")
		if tomlTag != "" {
			fieldMap[tomlTag] = v.Field(i)
		}
	}

	for key, value := range overrides {
		fieldValue, exists := fieldMap[key]
		if !exists {
			return fmt.Errorf("unknown config key: %s", key)
		}

		if err := setFieldValue(fieldValue, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}

	return nil
}

// setFieldValue sets a reflect.Value with proper type conversion
func setFieldValue(field reflect.Value, value any) error {
	switch field.Kind() {
	case reflect.String:
		strVal, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
		field.SetString(strVal)

	case reflect.Int64:
		switch v := value.(type) {
		case int64:
			field.SetInt(v)
		case int:
			field.SetInt(int64(v))
		default:
			return fmt.Errorf("expected int64, got %T", value)
		}

	case reflect.Float64:
		floatVal, ok := value.(float64)
		if !ok {
			return fmt.Errorf("expected float64, got %T", value)
		}
		field.SetFloat(floatVal)

	case reflect.Bool:
		boolVal, ok := value.(bool)
		if !ok {
			return fmt.Errorf("expected bool, got %T", value)
		}
		field.SetBool(boolVal)

	default:
		return fmt.Errorf("unsupported field type: %v", field.Kind())
	}

	return nil
}

// validate performs validation on the configuration
func (c *Config) validate() error {
	// String validations
	if strings.TrimSpace(c.Name) == "" {
		return fmtErrorf("log name cannot be empty")
	}

	if c.Format != "txt" && c.Format != "json" && c.Format != "raw" {
		return fmtErrorf("invalid format: '%s' (use txt, json, or raw)", c.Format)
	}

	if strings.HasPrefix(c.Extension, ".") {
		return fmtErrorf("extension should not start with dot: %s", c.Extension)
	}

	if strings.TrimSpace(c.TimestampFormat) == "" {
		return fmtErrorf("timestamp_format cannot be empty")
	}

	if c.StdoutTarget != "stdout" && c.StdoutTarget != "stderr" {
		return fmtErrorf("invalid stdout_target: '%s' (use stdout or stderr)", c.StdoutTarget)
	}

	// Numeric validations
	if c.BufferSize <= 0 {
		return fmtErrorf("buffer_size must be positive: %d", c.BufferSize)
	}

	if c.MaxSizeMB < 0 || c.MaxTotalSizeMB < 0 || c.MinDiskFreeMB < 0 {
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

// Clone creates a deep copy of the configuration
func (c *Config) Clone() *Config {
	copiedConfig := *c
	return &copiedConfig
}