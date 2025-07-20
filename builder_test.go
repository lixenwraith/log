// FILE: lixenwraith/log/builder_test.go
package log

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilder_Build(t *testing.T) {
	t.Run("successful build returns configured logger", func(t *testing.T) {
		// Create a temporary directory for the test
		tmpDir := t.TempDir()

		// Use the builder to create a logger with custom settings
		logger, err := NewBuilder().
			Directory(tmpDir).
			LevelString("debug").
			Format("json").
			BufferSize(2048).
			EnableStdout(true).
			MaxSizeMB(10).
			HeartbeatLevel(2).
			Build()

		// Ensure the logger is cleaned up
		if logger != nil {
			defer logger.Shutdown()
		}

		// Check for build errors
		require.NoError(t, err, "Builder.Build() should not return an error on valid config")
		require.NotNil(t, logger, "Builder.Build() should return a non-nil logger")

		// Retrieve the configuration from the logger to verify it was applied correctly
		cfg := logger.GetConfig()
		require.NotNil(t, cfg, "Logger.GetConfig() should return a non-nil config")

		// Assert that the configuration values match what was set
		assert.Equal(t, tmpDir, cfg.Directory)
		assert.Equal(t, LevelDebug, cfg.Level)
		assert.Equal(t, "json", cfg.Format)
		assert.Equal(t, int64(2048), cfg.BufferSize)
		assert.True(t, cfg.EnableStdout, "EnableStdout should be true")
		assert.Equal(t, int64(10*1000), cfg.MaxSizeKB)
		assert.Equal(t, int64(2), cfg.HeartbeatLevel)
	})

	t.Run("builder error accumulation", func(t *testing.T) {
		// Use an invalid level string to trigger an error within the builder
		logger, err := NewBuilder().
			LevelString("invalid-level-string").
			Directory("/some/dir"). // This should not be evaluated
			Build()

		// Assert that an error is returned and it's the one we expect
		require.Error(t, err, "Build should fail with an invalid level string")
		assert.Contains(t, err.Error(), "invalid level string", "Error message should indicate invalid level")

		// Assert that the logger is nil because the build failed
		assert.Nil(t, logger, "A nil logger should be returned on build error")
	})

	t.Run("apply config validation error", func(t *testing.T) {
		// Use a configuration that will fail validation inside ApplyConfig,
		// e.g., an invalid directory path that cannot be created.
		// Note: on linux /root is not writable by non-root users.
		invalidDir := filepath.Join("/root", "unwritable-log-test-dir")
		logger, err := NewBuilder().
			Directory(invalidDir).
			Build()

		// Assert that ApplyConfig (called by Build) failed
		require.Error(t, err, "Build should fail with an unwritable directory")
		assert.Contains(t, err.Error(), "failed to create log directory", "Error message should indicate directory creation failure")

		// Assert that the logger is nil
		assert.Nil(t, logger, "A nil logger should be returned on apply config error")
	})
}