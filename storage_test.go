// FILE: lixenwraith/log/storage_test.go
package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogRotation(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.MaxSizeKB = 1000     // 1MB
	cfg.FlushIntervalMs = 10 // Fast flush for testing
	logger.ApplyConfig(cfg)

	// Create a message that's large enough to trigger rotation
	// Account for timestamp, level, and other formatting overhead
	// A typical log line overhead is ~50-100 bytes
	const overhead = 100
	const targetMessageSize = 50000 // 50KB per message
	largeData := strings.Repeat("x", targetMessageSize)

	// Write enough to exceed 1MB twice (should cause at least one rotation)
	messagesNeeded := (2 * sizeMultiplier * 1000) / (targetMessageSize + overhead) // ~40 messages

	for i := 0; i < messagesNeeded; i++ {
		logger.Info(fmt.Sprintf("msg%d:", i), largeData)
		// Small delay to ensure processing
		if i%10 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Ensure all logs are written and rotated
	time.Sleep(100 * time.Millisecond)
	logger.Flush(time.Second)

	// Check for rotated files
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	// Count log files
	logFileCount := 0
	hasRotated := false
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".log") {
			logFileCount++
			// Check for rotated file pattern: log_YYMMDD_HHMMSS_*.log
			if strings.HasPrefix(f.Name(), "log_") && strings.Contains(f.Name(), "_") {
				hasRotated = true
			}
		}
	}

	// Should have at least 2 log files (current + at least one rotated)
	assert.GreaterOrEqual(t, logFileCount, 2, "Expected at least 2 log files (current + rotated)")
	assert.True(t, hasRotated, "Expected to find rotated log files with timestamp pattern")
}

func TestDiskSpaceManagement(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	// Create some old log files to be cleaned up
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("log_old_%d.log", i)
		path := filepath.Join(tmpDir, name)
		// Write more than 1KB of data to ensure total size exceeds the new limit
		err := os.WriteFile(path, []byte(strings.Repeat("a", 2000)), 0644)
		require.NoError(t, err)

		// Make files appear old
		oldTime := time.Now().Add(-time.Hour * 24 * time.Duration(i+1))
		os.Chtimes(path, oldTime, oldTime)
	}

	cfg := logger.GetConfig()
	// Set a small limit to trigger cleanup. 0 disables the check.
	cfg.MaxTotalSizeKB = 1
	// Disable free disk space check to isolate the total size check
	cfg.MinDiskFreeKB = 0
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	// Trigger disk check and cleanup
	logger.performDiskCheck(true)

	// Small delay to let the check complete
	time.Sleep(100 * time.Millisecond)

	// Verify cleanup occurred. All old logs should be deleted.
	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	// Only the active log.log should remain
	assert.Equal(t, 1, len(files), "Expected only the active log file to remain after cleanup")
	assert.Equal(t, "log.log", files[0].Name())
}

func TestRetentionPolicy(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	// Create an old log file
	oldFile := filepath.Join(tmpDir, "log_old.log")
	err := os.WriteFile(oldFile, []byte("old data"), 0644)
	require.NoError(t, err)

	// Set modification time to 2 hours ago
	oldTime := time.Now().Add(-2 * time.Hour)
	os.Chtimes(oldFile, oldTime, oldTime)

	cfg := logger.GetConfig()
	cfg.RetentionPeriodHrs = 1.0 // 1 hour retention
	logger.ApplyConfig(cfg)

	// Manually trigger retention check
	logger.cleanExpiredLogs(oldTime)

	// Verify old file was deleted
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))
}