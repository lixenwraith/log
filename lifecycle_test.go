// FILE: lixenwraith/log/lifecycle_test.go
package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStartStopLifecycle verifies the logger can be started, stopped, and restarted
func TestStartStopLifecycle(t *testing.T) {
	logger, _ := createTestLogger(t) // Starts the logger by default

	assert.True(t, logger.state.Started.Load(), "Logger should be in a started state")

	// Stop the logger
	err := logger.Stop()
	require.NoError(t, err)
	assert.False(t, logger.state.Started.Load(), "Logger should be in a stopped state after Stop()")

	// Start it again
	err = logger.Start()
	require.NoError(t, err)
	assert.True(t, logger.state.Started.Load(), "Logger should be in a started state after restart")

	logger.Shutdown()
}

// TestStartAlreadyStarted verifies that starting an already started logger is a safe no-op
func TestStartAlreadyStarted(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	assert.True(t, logger.state.Started.Load())

	// Calling Start() on an already started logger should be a no-op and return no error
	err := logger.Start()
	assert.NoError(t, err)
	assert.True(t, logger.state.Started.Load())
}

// TestStopAlreadyStopped verifies that stopping an already stopped logger is a safe no-op
func TestStopAlreadyStopped(t *testing.T) {
	logger, _ := createTestLogger(t)

	// Stop it once
	err := logger.Stop()
	require.NoError(t, err)
	assert.False(t, logger.state.Started.Load())

	// Calling Stop() on an already stopped logger should be a no-op and return no error
	err = logger.Stop()
	assert.NoError(t, err)
	assert.False(t, logger.state.Started.Load())

	logger.Shutdown()
}

// TestStopReconfigureRestart tests reconfiguring a logger while it is stopped
func TestStopReconfigureRestart(t *testing.T) {
	tmpDir := t.TempDir()
	logger := NewLogger()

	// Initial config: txt format
	cfg1 := DefaultConfig()
	cfg1.Directory = tmpDir
	cfg1.Format = "txt"
	cfg1.ShowTimestamp = false
	err := logger.ApplyConfig(cfg1)
	require.NoError(t, err)

	// Start and log
	err = logger.Start()
	require.NoError(t, err)
	logger.Info("first message")
	logger.Flush(time.Second)

	// Stop the logger
	err = logger.Stop()
	require.NoError(t, err)

	// Reconfigure: json format
	cfg2 := logger.GetConfig()
	cfg2.Format = "json"
	err = logger.ApplyConfig(cfg2)
	require.NoError(t, err)

	// Restart and log
	err = logger.Start()
	require.NoError(t, err)
	logger.Info("second message")
	logger.Shutdown(time.Second)

	// Verify content
	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)
	strContent := string(content)

	assert.Contains(t, strContent, "INFO first message", "Should contain the log from the first configuration")
	assert.Contains(t, strContent, `"fields":["second message"]`, "Should contain the log from the second (JSON) configuration")
}

// TestLoggingOnStoppedLogger ensures that log entries are dropped when the logger is stopped
func TestLoggingOnStoppedLogger(t *testing.T) {
	logger, tmpDir := createTestLogger(t)

	// Log something while running
	logger.Info("this should be logged")
	logger.Flush(time.Second)

	// Stop the logger
	err := logger.Stop()
	require.NoError(t, err)

	// Attempt to log while stopped
	logger.Warn("this should NOT be logged")

	// Shutdown (which flushes)
	logger.Shutdown(time.Second)

	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)

	assert.Contains(t, string(content), "this should be logged")
	assert.NotContains(t, string(content), "this should NOT be logged")
}

// TestFlushOnStoppedLogger verifies that Flush returns an error on a stopped logger
func TestFlushOnStoppedLogger(t *testing.T) {
	logger, _ := createTestLogger(t)

	// Stop the logger
	err := logger.Stop()
	require.NoError(t, err)

	// Flush should return an error
	err = logger.Flush(time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger not started")

	logger.Shutdown()
}

// TestShutdownLifecycle checks the terminal state of the logger after shutdown
func TestShutdownLifecycle(t *testing.T) {
	logger, _ := createTestLogger(t)

	assert.True(t, logger.state.Started.Load())
	assert.True(t, logger.state.IsInitialized.Load())

	// Shutdown is a terminal state
	err := logger.Shutdown()
	require.NoError(t, err)

	assert.True(t, logger.state.ShutdownCalled.Load())
	assert.False(t, logger.state.IsInitialized.Load(), "Shutdown should de-initialize the logger")
	assert.False(t, logger.state.Started.Load(), "Shutdown should stop the logger")

	// Attempting to start again should fail because it's no longer initialized
	err = logger.Start()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger not initialized")

	// Logging should be a silent no-op
	logger.Info("this will not be logged")

	// Flush should fail
	err = logger.Flush(time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}