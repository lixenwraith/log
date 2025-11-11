// FILE: lixenwraith/log/processor_test.go
package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoggerHeartbeat verifies that heartbeat messages are logged correctly
func TestLoggerHeartbeat(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.HeartbeatLevel = 3 // All heartbeats
	cfg.HeartbeatIntervalS = 1
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

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

// TestDroppedLogs confirms that the logger correctly tracks dropped logs when the buffer is full
func TestDroppedLogs(t *testing.T) {
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = t.TempDir()
	cfg.BufferSize = 1         // Very small buffer
	cfg.FlushIntervalMs = 10   // Fast processing
	cfg.HeartbeatLevel = 1     // Enable proc heartbeat
	cfg.HeartbeatIntervalS = 1 // Fast heartbeat

	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	err = logger.Start()
	require.NoError(t, err)
	defer logger.Shutdown()

	// Flood to guarantee drops
	for i := 0; i < 100; i++ {
		logger.Info("flood", i)
	}

	// Wait for first heartbeat
	time.Sleep(1500 * time.Millisecond)

	// Flood again
	for i := 0; i < 50; i++ {
		logger.Info("flood2", i)
	}

	// Wait for second heartbeat
	time.Sleep(1000 * time.Millisecond)
	logger.Flush(time.Second)

	// Read log file and verify heartbeats
	content, err := os.ReadFile(filepath.Join(cfg.Directory, "log.log"))
	require.NoError(t, err)

	lines := strings.Split(string(content), "\n")
	foundTotal := false
	foundInterval := false

	for _, line := range lines {
		if strings.Contains(line, "PROC") {
			if strings.Contains(line, "total_dropped_logs") {
				foundTotal = true
			}
			if strings.Contains(line, "dropped_since_last") {
				foundInterval = true
			}
		}
	}

	assert.True(t, foundTotal, "Expected PROC heartbeat with total_dropped_logs")
	assert.True(t, foundInterval, "Expected PROC heartbeat with dropped_since_last")
}

// TestAdaptiveDiskCheck ensures the adaptive disk check mechanism functions without panicking
func TestAdaptiveDiskCheck(t *testing.T) {
	logger, _ := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.EnableAdaptiveInterval = true
	cfg.DiskCheckIntervalMs = 100
	cfg.MinCheckIntervalMs = 50
	cfg.MaxCheckIntervalMs = 500
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

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

// TestDroppedLogRecoveryOnDroppedHeartbeat verifies the total drop count remains accurate even if a heartbeat is dropped
func TestDroppedLogRecoveryOnDroppedHeartbeat(t *testing.T) {
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = t.TempDir()
	cfg.BufferSize = 10        // Small buffer
	cfg.HeartbeatLevel = 1     // Enable proc heartbeat
	cfg.HeartbeatIntervalS = 1 // Fast heartbeat
	cfg.Format = "json"        // Use JSON for easy parsing

	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	err = logger.Start()
	require.NoError(t, err)
	defer logger.Shutdown()

	// 1. Flood the logger to guarantee drops, aiming to drop exactly 50 logs
	const floodCount = 50
	for i := 0; i < int(cfg.BufferSize)+floodCount; i++ {
		logger.Info("flood", i)
	}

	// Wait for the first heartbeat to be generated and report ~50 drops
	time.Sleep(1100 * time.Millisecond)

	// 2. Immediately put the logger into a "disk full" state, causing processor to drop the first heartbeat
	diskFullCfg := logger.GetConfig()
	diskFullCfg.MinDiskFreeKB = 9999999999
	err = logger.ApplyConfig(diskFullCfg)
	require.NoError(t, err)
	// Force a disk check to ensure the state is updated to not OK
	logger.performDiskCheck(true)
	assert.False(t, logger.state.DiskStatusOK.Load(), "Disk status should be not OK")

	// 3. Now, "fix" the disk so the next heartbeat can be written successfully
	diskOKCfg := logger.GetConfig()
	diskOKCfg.MinDiskFreeKB = 0
	err = logger.ApplyConfig(diskOKCfg)
	require.NoError(t, err)
	logger.performDiskCheck(true) // Ensure state is updated back to OK
	assert.True(t, logger.state.DiskStatusOK.Load(), "Disk status should be OK")

	// 4. Wait for the second heartbeat to be generated and written to the file
	time.Sleep(1100 * time.Millisecond)
	logger.Flush(time.Second)

	// 5. Verify the log file content
	content, err := os.ReadFile(filepath.Join(cfg.Directory, "log.log"))
	require.NoError(t, err)

	var foundHeartbeat bool
	var intervalDropCount, totalDropCount float64
	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		// Find the last valid heartbeat with drop stats
		if strings.Contains(line, `"level":"PROC"`) && strings.Contains(line, "dropped_since_last") {
			foundHeartbeat = true
			var entry map[string]any
			err := json.Unmarshal([]byte(line), &entry)
			require.NoError(t, err, "Failed to parse heartbeat log line: %s", line)

			fields := entry["fields"].([]any)
			for i := 0; i < len(fields)-1; i += 2 {
				if key, ok := fields[i].(string); ok {
					if key == "dropped_since_last" {
						intervalDropCount, _ = fields[i+1].(float64)
					}
					if key == "total_dropped_logs" {
						totalDropCount, _ = fields[i+1].(float64)
					}
				}
			}
		}
	}

	require.True(t, foundHeartbeat, "Did not find the final heartbeat with drop stats")

	// ASSERT THE CURRENT BEHAVIOR:
	// The 'dropped_since_last' count from the first heartbeat (~50) was lost when that heartbeat was dropped
	// The only new drop in the next interval was the heartbeat record itself
	assert.Equal(t, float64(1), intervalDropCount, "The interval drop count should only reflect the single dropped heartbeat from the previous interval.")

	// The 'total_dropped_logs' counter should be accurate, reflecting the initial flood (~50) + the one dropped heartbeat
	assert.True(t, totalDropCount >= float64(floodCount), "Total drop count should be at least the number of flooded logs plus the dropped heartbeat.")
}