// FILE: lixenwraith/log/integration_test.go
package log

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	// Create logger with builder using the new streamlined interface
	logger, err := NewBuilder().
		Directory(tmpDir).
		LevelString("debug").
		Format("json").
		MaxSizeKB(1).
		BufferSize(1000).
		EnableStdout(false).
		HeartbeatLevel(1).
		HeartbeatIntervalS(2).
		Build()

	require.NoError(t, err, "Logger creation with builder should succeed")
	require.NotNil(t, logger)

	// Defer shutdown right after successful creation
	defer func() {
		err := logger.Shutdown(2 * time.Second)
		assert.NoError(t, err, "Logger shutdown should be clean")
	}()

	// Log at various levels
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warning message")
	logger.Error("error message")

	// Structured logging
	logger.LogStructured(LevelInfo, "structured log", map[string]any{
		"user_id": 123,
		"action":  "login",
		"success": true,
	})

	// Raw write
	logger.Write("raw data write")

	// Trace logging
	logger.InfoTrace(2, "trace info")

	// Apply runtime override
	err = logger.ApplyConfigString("enable_stdout=true", "stdout_target=stderr")
	require.NoError(t, err)

	// More logging after reconfiguration
	logger.Info("after reconfiguration")

	// Wait for heartbeat
	time.Sleep(2500 * time.Millisecond)

	// Flush and check
	err = logger.Flush(time.Second)
	assert.NoError(t, err)

	// Verify log content
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(files), 1, "At least one log file should be created")
}

func TestConcurrentOperations(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	var wg sync.WaitGroup

	// Concurrent logging
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				logger.Info("worker", id, "log", j)
			}
		}(i)
	}

	// Concurrent configuration changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			err := logger.ApplyConfigString(fmt.Sprintf("buffer_size=%d", 100+i*100))
			assert.NoError(t, err)
			time.Sleep(50 * time.Millisecond)
		}
	}()

	// Concurrent flushes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			err := logger.Flush(100 * time.Millisecond)
			assert.NoError(t, err)
			time.Sleep(30 * time.Millisecond)
		}
	}()

	wg.Wait()
}

func TestErrorRecovery(t *testing.T) {
	t.Run("invalid directory", func(t *testing.T) {
		// Use the builder to attempt creation with an invalid directory
		logger, err := NewBuilder().
			Directory("/root/cannot_write_here_without_sudo").
			Build()

		assert.Error(t, err, "Should get an error for an invalid directory")
		assert.Nil(t, logger, "Logger should be nil on creation failure")
	})

	t.Run("disk full simulation", func(t *testing.T) {
		logger, _ := createTestLogger(t)
		defer logger.Shutdown()

		cfg := logger.GetConfig()
		cfg.MinDiskFreeKB = 9999999999 // A very large number to simulate a full disk
		err := logger.ApplyConfig(cfg)
		require.NoError(t, err)

		// Should detect disk space issue during the check
		isOK := logger.performDiskCheck(true)
		assert.False(t, isOK, "Disk check should fail when min free space is not met")
		assert.False(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK state should be false")

		// Small delay to ensure the processor has time to react if needed
		time.Sleep(100 * time.Millisecond)

		// Logs should be dropped when disk status is not OK
		preDropped := logger.state.DroppedLogs.Load()
		logger.Info("this log entry should be dropped")

		// Small delay to let the log processor attempt to process the record
		time.Sleep(100 * time.Millisecond)

		postDropped := logger.state.DroppedLogs.Load()
		assert.Greater(t, postDropped, preDropped, "Dropped log count should increase")
	})
}