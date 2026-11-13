package log

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
		// Archive pattern: log_YYMMDD_HHMMSS[_N].log
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

	c := logger.getConfig()
	dirSize, err := logger.getLogDirSize(c)
	mustNoErr(t, err, "getLogDirSize")
	// The active file is present but empty
	equal(t, dirSize, int64(files*size), "getLogDirSize")

	count, err := logger.getLogFileCount(c)
	mustNoErr(t, err, "getLogFileCount")
	equal(t, count, files+1, "getLogFileCount")

	// Missing directories are not an error condition
	c = c.Clone()
	c.Directory = filepath.Join(tmpDir, "absent")
	dirSize, err = logger.getLogDirSize(c)
	mustNoErr(t, err, "getLogDirSize on missing dir")
	equal(t, dirSize, int64(0), "size of missing dir")

	count, err = logger.getLogFileCount(c)
	mustNoErr(t, err, "getLogFileCount on missing dir")
	equal(t, count, 0, "count of missing dir")
}

// TestArchiveNaming verifies archive names carry the base name and never
// collide with a file already present in the directory.
func TestArchiveNaming(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	equal(t, logger.getStaticLogFilePath(), filepath.Join(tmpDir, "log.log"), "static path")

	ts := time.Now()
	first := logger.generateArchiveLogFileName(ts)
	isTrue(t, strings.HasPrefix(first, "log_"), "archive prefix")
	isTrue(t, strings.HasSuffix(first, ".log"), "archive extension")

	// Names are second-resolution; the counter disambiguates against files
	// already on disk, not against clock resolution
	mustNoErr(t, os.WriteFile(filepath.Join(tmpDir, first), []byte("archived"), 0644), "WriteFile archive")

	second := logger.generateArchiveLogFileName(ts)
	if second == first {
		t.Errorf("archive name collided with existing file: %s", second)
	}
	isTrue(t, strings.HasPrefix(second, "log_"), "archive prefix")
	isTrue(t, strings.HasSuffix(second, ".log"), "archive extension")
}

// With cleanup_own_files, size limits, cleanup and retention see only the
// logger's file and the archives it names: other names, per-run names and
// other programs' files sharing the directory survive.
func TestCleanupOwnFilesSparesOtherFiles(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.Name = "app"
	c.CleanupOwnFiles = true
	c.RetentionPeriodHrs = 1
	mustNoErr(t, l.ApplyConfig(c), "apply") // not started: the test runs cleanup
	defer l.Shutdown()

	old := time.Now().Add(-2 * time.Hour)
	write := func(name string) {
		path := filepath.Join(c.Directory, name)
		mustNoErr(t, os.WriteFile(path, []byte("0123456789"), 0644), name)
		mustNoErr(t, os.Chtimes(path, old, old), name)
	}
	var own []string
	for range 3 { // the second and third archives take a counter
		name := l.generateArchiveLogFileName(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
		write(name)
		own = append(own, name)
	}
	others := []string{"other.log", "other_260102_030405.log", "app-run2.log", "appx_260102_030405.log",
		"app_260102.log", "app_260102_030405_.log", "app_260102_03040x.log", "app_260102_030405.txt"}
	for _, name := range others {
		write(name)
	}

	size, err := l.getLogDirSize(c)
	mustNoErr(t, err, "size")
	equal(t, size, int64(10*len(own)), "size counts the empty active file and the archives")
	mustNoErr(t, l.cleanExpiredLogs(old), "retention")
	for _, name := range own {
		_, err := os.Stat(filepath.Join(c.Directory, name))
		isTrue(t, os.IsNotExist(err), name+" expired")
	}
	errContains(t, l.cleanOldLogs(1), "no old logs", "other files are not candidates")
	for _, name := range others {
		_, err := os.Stat(filepath.Join(c.Directory, name))
		noErr(t, err, name+" kept")
	}
}

// followLogger returns a started txt logger writing dir/app.log, and the
// diagnostics it reported
func followLogger(t *testing.T, maxKB int64) (*Logger, string, func() []string) {
	t.Helper()
	l := NewLogger()
	var mu sync.Mutex
	var diags []string
	l.SetErrorHandler(func(s string) { mu.Lock(); diags = append(diags, s); mu.Unlock() })
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.Name = "app"
	c.Format = "txt"
	c.MaxSizeKB = maxKB
	c.MaxTotalSizeKB = 0
	c.MinDiskFreeKB = 0
	mustNoErr(t, l.ApplyConfig(c), "ApplyConfig")
	mustNoErr(t, l.Start(), "Start")
	t.Cleanup(func() { _ = l.Shutdown() })
	return l, filepath.Join(c.Directory, "app.log"), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(diags)
	}
}

func logFlush(t *testing.T, l *Logger, msgs ...string) {
	t.Helper()
	for _, m := range msgs {
		l.Info(m)
	}
	mustNoErr(t, l.Flush(time.Second), "Flush")
}

// logProcessed logs without Flush, whose forced path check would mask the one under test
func logProcessed(t *testing.T, l *Logger, msgs ...string) {
	t.Helper()
	want := l.state.TotalLogsProcessed.Load() + uint64(len(msgs))
	for _, m := range msgs {
		l.Info(m)
	}
	mustEventually(t, time.Second, "records processed", func() bool { return l.state.TotalLogsProcessed.Load() >= want })
}

// replaceFile saves content the way vim, sed -i and perl -i do: a new file renamed over path
func replaceFile(t *testing.T, path, content string) {
	t.Helper()
	mustNoErr(t, os.WriteFile(path+".tmp", []byte(content), 0644), "write replacement")
	mustNoErr(t, os.Rename(path+".tmp", path), "replace")
}

func TestFollowsReplacedLogFile(t *testing.T) {
	l, path, _ := followLogger(t, 0)
	logFlush(t, l, "before")
	data, err := os.ReadFile(path)
	mustNoErr(t, err, "read")
	replaceFile(t, path, string(data)+"NOTE by user\n")
	time.Sleep(fileCheckInterval + 5*time.Millisecond) // the next record is due a check
	logProcessed(t, l, "after")
	data, err = os.ReadFile(path)
	mustNoErr(t, err, "read")
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	mustEqual(t, len(lines), 3, "lines: "+string(data))
	contains(t, lines[0], "before", "kept by the editor")
	equal(t, lines[1], "NOTE by user", "note")
	contains(t, lines[2], "after", "appended after the note")
	equal(t, l.state.TotalReopens.Load(), uint64(1), "reopens")
}

// The rotation decision follows a replaced file instead of archiving it
func TestRotationFollowsReplacedLogFile(t *testing.T) {
	l, path, _ := followLogger(t, 1) // 1000 bytes
	for l.state.CurrentSize.Load() < 900 {
		logFlush(t, l, "fill-"+strings.Repeat("y", 10))
	}
	replaceFile(t, path, "")        // all content deleted
	pad := strings.Repeat("z", 120) // the first record after the edit crosses the stale size limit
	logProcessed(t, l, "a1"+pad, "a2"+pad, "a3"+pad)
	data, err := os.ReadFile(path)
	mustNoErr(t, err, "read")
	equal(t, strings.Count(string(data), pad), 3, "records after the edit: "+string(data))
	equal(t, l.state.TotalRotations.Load(), uint64(0), "rotations")
}

// Truncation leaves the tracked size stale; rotation must use the real size
func TestRotationAfterTruncateUsesFileSize(t *testing.T) {
	l, path, _ := followLogger(t, 2) // 2000 bytes
	for l.state.CurrentSize.Load() < 1800 {
		logFlush(t, l, "fill-"+strings.Repeat("y", 10))
	}
	mustNoErr(t, os.Truncate(path, 0), "truncate")
	logProcessed(t, l, "a1", "a2", "a3", "a4", "a5", "a6", "a7", "a8", "a9", "a10")
	equal(t, l.state.TotalRotations.Load(), uint64(0), "rotations")
	fi, err := os.Stat(path)
	mustNoErr(t, err, "stat")
	equal(t, l.state.CurrentSize.Load(), fi.Size(), "tracked size")
}

// A file removed under the logger leaves nothing to archive, not a disabled logger
func TestRotateRemovedLogFile(t *testing.T) {
	l, path, _ := followLogger(t, 0)
	mustNoErr(t, l.Stop(5*time.Second), "Stop") // own the file state from the test goroutine
	mustNoErr(t, os.Remove(path), "rm")
	mustNoErr(t, l.rotateLogFile(), "rotate")
	isFalse(t, l.state.LoggerDisabled.Load(), "logger disabled")
	_, err := os.Stat(path)
	mustNoErr(t, err, "new active file")
}

// Flush and Stop reattach a moved log path at once, as after logrotate's create
// mode; no record is processed in between, so only their check can reopen
func TestFlushAndStopReattachMovedLogFile(t *testing.T) {
	for name, barrier := range map[string]func(l *Logger) error{
		"flush": func(l *Logger) error { return l.Flush(time.Second) },
		"stop":  func(l *Logger) error { return l.Stop(5 * time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			l, path, _ := followLogger(t, 0)
			logFlush(t, l, "before")
			mustNoErr(t, os.Rename(path, path+".1"), "mv")
			mustNoErr(t, barrier(l), name)
			_, err := os.Stat(path)
			mustNoErr(t, err, "path recreated")
			equal(t, l.state.TotalReopens.Load(), uint64(1), "reopens")
		})
	}
}

// Cleanup never deletes the file being written, whatever it is now named
func TestCleanupSparesOpenFile(t *testing.T) {
	l, path, _ := followLogger(t, 0)
	logFlush(t, l, "before")
	mustNoErr(t, l.Stop(5*time.Second), "Stop") // own the file state from the test goroutine
	moved := filepath.Join(filepath.Dir(path), "app-keep.log")
	mustNoErr(t, os.Rename(path, moved), "mv")
	old := time.Now().Add(-48 * time.Hour)
	mustNoErr(t, os.Chtimes(moved, old, old), "age")
	c := l.GetConfig()
	c.RetentionPeriodHrs = 1
	mustNoErr(t, l.ApplyConfig(c), "ApplyConfig")

	errContains(t, l.cleanOldLogs(1), "no old logs", "size cleanup")
	mustNoErr(t, l.cleanExpiredLogs(old), "retention cleanup")
	_, err := os.Stat(moved)
	mustNoErr(t, err, "open file kept")
	equal(t, l.state.TotalDeletions.Load(), uint64(0), "deletions")
}

// A file removed to free disk space is followed while records are dropped for
// that low space, so the unlinked inode is released
func TestFollowsRemovedLogFileWhileDiskLow(t *testing.T) {
	l, path, _ := followLogger(t, 0)
	logFlush(t, l, "before")
	l.state.DiskStatusOK.Store(false) // as when min_disk_free_kb is not met
	mustNoErr(t, os.Remove(path), "rm")
	time.Sleep(fileCheckInterval + 5*time.Millisecond)
	l.Info("dropped")
	mustEventually(t, time.Second, "path recreated", func() bool { return l.state.TotalReopens.Load() == 1 })
	_, err := os.Stat(path)
	mustNoErr(t, err, "path")
}
