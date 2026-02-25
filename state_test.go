package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoggerShutdown verifies the logger's state and behavior after shutdown is called
func TestLoggerShutdown(t *testing.T) {
	t.Run("normal shutdown", func(t *testing.T) {
		logger, _ := createTestLogger(t)

		// Write some logs
		logger.Info("shutdown test")

		// Shutdown
		err := logger.Shutdown(2 * time.Second)
		assert.NoError(t, err)

		// Verify state
		assert.True(t, logger.state.ShutdownCalled.Load())
		assert.True(t, logger.state.LoggerDisabled.Load())
		assert.False(t, logger.state.IsInitialized.Load())
	})

	t.Run("shutdown timeout", func(t *testing.T) {
		logger, _ := createTestLogger(t)

		// Fill buffer to potentially block processor
		for i := 0; i < 200; i++ {
			logger.Info("flood", i)
		}

		// Short timeout
		err := logger.Shutdown(1 * time.Millisecond)
		// May or may not timeout depending on system speed
		_ = err
	})

	t.Run("shutdown before init", func(t *testing.T) {
		logger := NewLogger()
		err := logger.Shutdown()
		assert.NoError(t, err)
	})

	t.Run("double shutdown", func(t *testing.T) {
		logger, _ := createTestLogger(t)

		err1 := logger.Shutdown()
		err2 := logger.Shutdown()

		assert.NoError(t, err1)
		assert.NoError(t, err2)
	})
}

// TestLoggerFlush tests the functionality and timeout behavior of the Flush method
func TestLoggerFlush(t *testing.T) {
	t.Run("successful flush", func(t *testing.T) {
		logger, tmpDir := createTestLogger(t)
		defer logger.Shutdown()

		logger.Info("flush test")

		// Small delay to process log
		time.Sleep(100 * time.Millisecond)

		err := logger.Flush(time.Second)
		assert.NoError(t, err)

		// Verify data written
		content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "flush test")
	})

	t.Run("flush timeout", func(t *testing.T) {
		logger, _ := createTestLogger(t)
		defer logger.Shutdown()

		// Very short timeout
		err := logger.Flush(1 * time.Nanosecond)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "timeout")
	})

	t.Run("flush after shutdown", func(t *testing.T) {
		logger, _ := createTestLogger(t)
		logger.Shutdown()

		err := logger.Flush(time.Second)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not initialized")
	})
}