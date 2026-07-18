package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogRotation verifies size-triggered rotation, archive naming, and counters.
func TestLogRotation(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.MaxSizeKB = 100
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	const messageSize = 5000
	const overhead = 100 // timestamp + level + framing
	largeData := strings.Repeat("x", messageSize)

	// Enough volume for at least two rotations
	messages := int((2 * sizeMultiplier * cfg.MaxSizeKB) / (messageSize + overhead))
	for i := range messages {
		logger.Info(fmt.Sprintf("msg%d:", i), largeData)
	}
	mustNoErr(t, logger.Flush(2*time.Second), "Flush")

	mustEventually(t, 2*time.Second, "rotation performed", func() bool {
		return logger.state.TotalRotations.Load() > 0
	})

	entries, err := os.ReadDir(tmpDir)
	mustNoErr(t, err, "ReadDir")

	archives := 0
	hasActive := false
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == "log.log":
			hasActive = true
		// Archive pattern: log_YYMMDD_HHMMSS_<nano>.log
		case strings.HasPrefix(name, "log_") && strings.HasSuffix(name, ".log"):
			archives++
		default:
			t.Errorf("unexpected file in log directory: %s", name)
		}
	}

	isTrue(t, hasActive, "active log file must exist after rotation")
	if archives == 0 {
		t.Error("no archive files produced")
	}
	// Rotation resets the size counter, so the active file must be below the limit
	if size := logger.state.CurrentSize.Load(); size > cfg.MaxSizeKB*sizeMultiplier {
		t.Errorf("active file exceeds MaxSizeKB: %d bytes", size)
	}
}

// TestRotationDisabled verifies MaxSizeKB=0 suppresses rotation entirely.
func TestRotationDisabled(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.MaxSizeKB = 0
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	data := strings.Repeat("y", 5000)
	for range 20 {
		logger.Info(data)
	}
	mustNoErr(t, logger.Flush(2*time.Second), "Flush")

	equal(t, logger.state.TotalRotations.Load(), uint64(0), "TotalRotations")
	equal(t, countLogFiles(t, tmpDir), 1, "log file count")
}

// TestDiskSpaceManagement verifies total-size enforcement deletes oldest archives first.
func TestDiskSpaceManagement(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	// Five archives, 2000 bytes each, oldest last
	const archives = 5
	for i := range archives {
		path := filepath.Join(tmpDir, fmt.Sprintf("log_old_%d.log", i))
		mustNoErr(t, os.WriteFile(path, []byte(strings.Repeat("a", 2000)), 0644), "WriteFile")
		old := time.Now().Add(-time.Duration(i+1) * 24 * time.Hour)
		mustNoErr(t, os.Chtimes(path, old, old), "Chtimes")
	}

	cfg := logger.GetConfig()
	cfg.MaxTotalSizeKB = 1 // 1000 bytes; 10000 present
	cfg.MinDiskFreeKB = 0  // isolate the total-size branch
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	isTrue(t, logger.performDiskCheck(true), "performDiskCheck must succeed after cleanup")
	isTrue(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK")

	// Freeing 9000 bytes requires all five archives; the active file is never eligible
	equal(t, countLogFiles(t, tmpDir), 1, "remaining log files")
	equal(t, logger.state.TotalDeletions.Load(), uint64(archives), "TotalDeletions")

	entries, err := os.ReadDir(tmpDir)
	mustNoErr(t, err, "ReadDir")
	equal(t, entries[0].Name(), "log.log", "surviving file")
}

// TestCleanOldLogsInsufficient verifies the error path when nothing can be freed.
func TestCleanOldLogsInsufficient(t *testing.T) {
	logger, _ := newTestLogger(t)

	// Only the active file exists, and it is excluded from deletion
	errContains(t, logger.cleanOldLogs(1000), "no old logs available to delete", "cleanOldLogs")
	noErr(t, logger.cleanOldLogs(0), "cleanOldLogs with no requirement")
}

// TestRetentionPolicy verifies age-based deletion spares recent and active files.
func TestRetentionPolicy(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	expired := filepath.Join(tmpDir, "log_expired.log")
	mustNoErr(t, os.WriteFile(expired, []byte("old data"), 0644), "WriteFile expired")
	oldTime := time.Now().Add(-2 * time.Hour)
	mustNoErr(t, os.Chtimes(expired, oldTime, oldTime), "Chtimes")

	fresh := filepath.Join(tmpDir, "log_fresh.log")
	mustNoErr(t, os.WriteFile(fresh, []byte("new data"), 0644), "WriteFile fresh")

	cfg := logger.GetConfig()
	cfg.RetentionPeriodHrs = 1.0
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	mustNoErr(t, logger.cleanExpiredLogs(oldTime), "cleanExpiredLogs")

	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Errorf("expired file must be deleted, stat err: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("recent file must survive: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "log.log")); err != nil {
		t.Errorf("active file must survive: %v", err)
	}
	equal(t, logger.state.TotalDeletions.Load(), uint64(1), "TotalDeletions")
}

// TestRetentionDisabled verifies a zero retention period is a no-op.
func TestRetentionDisabled(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	archive := filepath.Join(tmpDir, "log_ancient.log")
	mustNoErr(t, os.WriteFile(archive, []byte("data"), 0644), "WriteFile")
	old := time.Now().Add(-1000 * time.Hour)
	mustNoErr(t, os.Chtimes(archive, old, old), "Chtimes")

	// RetentionPeriodHrs defaults to 0
	mustNoErr(t, logger.cleanExpiredLogs(old), "cleanExpiredLogs")
	if _, err := os.Stat(archive); err != nil {
		t.Errorf("file must survive with retention disabled: %v", err)
	}
}

// TestLogDirAccounting verifies size and count helpers filter on extension.
func TestLogDirAccounting(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	const files, size = 3, 500
	for i := range files {
		path := filepath.Join(tmpDir, fmt.Sprintf("log_%d.log", i))
		mustNoErr(t, os.WriteFile(path, []byte(strings.Repeat("z", size)), 0644), "WriteFile")
	}
	// Non-matching extension must be excluded from both helpers
	mustNoErr(t, os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("ignored"), 0644), "WriteFile txt")

	dirSize, err := logger.getLogDirSize(tmpDir, "log")
	mustNoErr(t, err, "getLogDirSize")
	// The active file is present but empty
	equal(t, dirSize, int64(files*size), "getLogDirSize")

	count, err := logger.getLogFileCount(tmpDir, "log")
	mustNoErr(t, err, "getLogFileCount")
	equal(t, count, files+1, "getLogFileCount")

	// Missing directories are not an error condition
	missing := filepath.Join(tmpDir, "absent")
	dirSize, err = logger.getLogDirSize(missing, "log")
	mustNoErr(t, err, "getLogDirSize on missing dir")
	equal(t, dirSize, int64(0), "size of missing dir")

	count, err = logger.getLogFileCount(missing, "log")
	mustNoErr(t, err, "getLogFileCount on missing dir")
	equal(t, count, 0, "count of missing dir")
}

// TestArchiveNaming verifies archive names are unique and carry the base name.
func TestArchiveNaming(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	equal(t, logger.getStaticLogFilePath(), filepath.Join(tmpDir, "log.log"), "static path")

	ts := time.Now()
	first := logger.generateArchiveLogFileName(ts)
	second := logger.generateArchiveLogFileName(ts.Add(time.Nanosecond))

	isTrue(t, strings.HasPrefix(first, "log_"), "archive prefix")
	isTrue(t, strings.HasSuffix(first, ".log"), "archive extension")
	if first == second {
		t.Errorf("archive names must be unique at nanosecond resolution: %s", first)
	}
}

