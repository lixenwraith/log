// FILE: lixenwraith/log/config_test.go
package log

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultConfig verifies that the default configuration is created with expected values
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotNil(t, cfg)
	assert.Equal(t, LevelInfo, cfg.Level)
	assert.Equal(t, "log", cfg.Name)
	assert.Equal(t, "log", cfg.Extension)
	assert.Equal(t, "./log", cfg.Directory)
	assert.Equal(t, "raw", cfg.Format)
	assert.Equal(t, PolicyRaw, cfg.Sanitization)
	assert.True(t, cfg.ShowTimestamp)
	assert.True(t, cfg.ShowLevel)
	assert.Equal(t, time.RFC3339Nano, cfg.TimestampFormat)
	assert.Equal(t, int64(1024), cfg.BufferSize)
}

// TestConfigClone verifies that cloning a config creates a deep copy
func TestConfigClone(t *testing.T) {
	cfg1 := DefaultConfig()
	cfg1.Level = LevelDebug
	cfg1.Directory = "/custom/path"

	cfg2 := cfg1.Clone()

	// Verify deep copy
	assert.Equal(t, cfg1.Level, cfg2.Level)
	assert.Equal(t, cfg1.Directory, cfg2.Directory)

	// Modify original
	cfg1.Level = LevelError

	// Verify clone unchanged
	assert.Equal(t, LevelDebug, cfg2.Level)
}

// TestConfigValidate checks various invalid configuration scenarios to ensure they produce errors
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*Config)
		wantError string
	}{
		{
			name:      "valid config",
			modify:    func(c *Config) {},
			wantError: "",
		},
		{
			name:      "empty name",
			modify:    func(c *Config) { c.Name = "" },
			wantError: "log name cannot be empty",
		},
		{
			name:      "invalid format",
			modify:    func(c *Config) { c.Format = "invalid" },
			wantError: "invalid format",
		},
		{
			name:      "extension with dot",
			modify:    func(c *Config) { c.Extension = ".log" },
			wantError: "extension should not start with dot",
		},
		{
			name:      "negative buffer size",
			modify:    func(c *Config) { c.BufferSize = -1 },
			wantError: "buffer_size must be positive",
		},
		{
			name:      "invalid trace depth",
			modify:    func(c *Config) { c.TraceDepth = 11 },
			wantError: "trace_depth must be between 0 and 10",
		},
		{
			name:      "invalid heartbeat level",
			modify:    func(c *Config) { c.HeartbeatLevel = 4 },
			wantError: "heartbeat_level must be between 0 and 3",
		},
		{
			name:      "invalid stdout target",
			modify:    func(c *Config) { c.ConsoleTarget = "invalid" },
			wantError: "invalid console_target",
		},
		{
			name: "min > max check interval",
			modify: func(c *Config) {
				c.MinCheckIntervalMs = 1000
				c.MaxCheckIntervalMs = 500
			},
			wantError: "min_check_interval_ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()

			if tt.wantError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantError)
			}
		})
	}
}

// TestConcurrentApplyConfig verifies that applying configurations concurrently does not cause race conditions or panics
func TestConcurrentApplyConfig(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	var wg sync.WaitGroup

	// Concurrent config applications
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			cfg := logger.GetConfig()
			// Vary settings
			if id%2 == 0 {
				cfg.Level = LevelDebug
				cfg.Format = "json"
			} else {
				cfg.Level = LevelInfo
				cfg.Format = "txt"
			}
			cfg.TraceDepth = int64(id % 5)

			err := logger.ApplyConfig(cfg)
			assert.NoError(t, err)

			// Log with new config
			logger.Info("config test", id)
		}(i)
	}

	wg.Wait()

	// Verify logger still functional
	logger.Info("after concurrent config")
	err := logger.Flush(time.Second)
	assert.NoError(t, err)

	// Check log file exists and has content
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(files), 1)
}