// FILE: lixenwraith/log/processor_test.go
package log

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoggerHeartbeat(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.HeartbeatLevel = 3 // All heartbeats
	cfg.HeartbeatIntervalS = 1
	logger.ApplyConfig(cfg)

	// Wait for heartbeats
	time.Sleep(1500 * time.Millisecond)
	logger.Flush(time.Second)

	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)

	// Check for heartbeat content
	assert.Contains(t, string(content), "PROC")
	assert.Contains(t, string(content), "DISK")
	assert.Contains(t, string(content), "SYS")
	assert.Contains(t, string(content), "uptime_hours")
	assert.Contains(t, string(content), "processed_logs")
	assert.Contains(t, string(content), "num_goroutine")
}

func TestDroppedLogs(t *testing.T) {
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = t.TempDir()
	cfg.BufferSize = 1         // Very small buffer
	cfg.FlushIntervalMs = 1000 // Slow flush

	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)
	defer logger.Shutdown()

	// Flood the logger
	for i := 0; i < 100; i++ {
		logger.Info("flood", i)
	}

	// Let it process
	time.Sleep(100 * time.Millisecond)

	// Check drop counter
	dropped := logger.state.DroppedLogs.Load()
	// Some logs should have been dropped with buffer size 1
	assert.Greater(t, dropped, uint64(0))
}

func TestAdaptiveDiskCheck(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.EnableAdaptiveInterval = true
	cfg.DiskCheckIntervalMs = 100
	cfg.MinCheckIntervalMs = 50
	cfg.MaxCheckIntervalMs = 500
	logger.ApplyConfig(cfg)

	// Generate varying log rates and verify no panic
	for i := 0; i < 10; i++ {
		logger.Info("adaptive test", i)
		time.Sleep(10 * time.Millisecond)
	}

	// Burst
	for i := 0; i < 100; i++ {
		logger.Info("burst", i)
	}

	logger.Flush(time.Second)
}