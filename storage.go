// FILE: storage.go
package log

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// performSync syncs the current log file
func (l *Logger) performSync() {
	// Skip sync if file output is disabled
	disableFile, _ := l.config.Bool("log.disable_file")
	if disableFile {
		return
	}

	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if currentLogFile, isFile := cfPtr.(*os.File); isFile && currentLogFile != nil {
			if err := currentLogFile.Sync(); err != nil {
				// Log sync error
				syncErrRecord := logRecord{
					Flags:     FlagDefault,
					TimeStamp: time.Now(),
					Level:     LevelWarn,
					Args:      []any{"Log file sync failed", "file", currentLogFile.Name(), "error", err.Error()},
				}
				l.sendLogRecord(syncErrRecord)
			}
		}
	}
}

// performDiskCheck checks disk space, triggers cleanup if needed, and updates status
// Returns true if disk is OK, false otherwise
func (l *Logger) performDiskCheck(forceCleanup bool) bool {
	// Skip all disk checks if file output is disabled
	disableFile, _ := l.config.Bool("log.disable_file")
	if disableFile {
		// Always return OK status when file output is disabled
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}

	dir, _ := l.config.String("log.directory")
	ext, _ := l.config.String("log.extension")
	maxTotalMB, _ := l.config.Int64("log.max_total_size_mb")
	minDiskFreeMB, _ := l.config.Int64("log.min_disk_free_mb")
	maxTotal := maxTotalMB * 1024 * 1024
	minFreeRequired := minDiskFreeMB * 1024 * 1024

	if maxTotal <= 0 && minFreeRequired <= 0 {
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}

	freeSpace, err := l.getDiskFreeSpace(dir)
	if err != nil {
		fmtFprintf(os.Stderr, "log: warning - failed to check free disk space for '%s': %v\n", dir, err)
		if l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(false)
		}
		return false
	}

	needsCleanupCheck := false
	spaceToFree := int64(0)
	if minFreeRequired > 0 && freeSpace < minFreeRequired {
		needsCleanupCheck = true
		spaceToFree = minFreeRequired - freeSpace
	}

	if maxTotal > 0 {
		dirSize, err := l.getLogDirSize(dir, ext)
		if err != nil {
			fmtFprintf(os.Stderr, "log: warning - failed to check log directory size for '%s': %v\n", dir, err)
			if l.state.DiskStatusOK.Load() {
				l.state.DiskStatusOK.Store(false)
			}
			return false
		}
		if dirSize > maxTotal {
			needsCleanupCheck = true
			amountOver := dirSize - maxTotal
			if amountOver > spaceToFree {
				spaceToFree = amountOver
			}
		}
	}

	if needsCleanupCheck && forceCleanup {
		if err := l.cleanOldLogs(spaceToFree); err != nil {
			if !l.state.DiskFullLogged.Swap(true) {
				diskFullRecord := logRecord{
					Flags: FlagDefault, TimeStamp: time.Now(), Level: LevelError,
					Args: []any{"Log directory full or disk space low, cleanup failed", "error", err.Error()},
				}
				l.sendLogRecord(diskFullRecord)
			}
			if l.state.DiskStatusOK.Load() {
				l.state.DiskStatusOK.Store(false)
			}
			return false
		}
		// Cleanup succeeded
		l.state.DiskFullLogged.Store(false)
		l.state.DiskStatusOK.Store(true)
		l.updateEarliestFileTime()
		return true
	} else if needsCleanupCheck {
		// Limits exceeded, but not forcing cleanup now
		if l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(false)
		}
		return false
	} else {
		// Limits OK
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}
}

// getDiskFreeSpace retrieves available disk space for the given path
func (l *Logger) getDiskFreeSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmtErrorf("log directory '%s' does not exist for disk check: %w", path, err)
		}
		return 0, fmtErrorf("failed to stat log directory '%s': %w", path, err)
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
	}

	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmtErrorf("failed to get disk stats for '%s': %w", path, err)
	}
	availableBytes := int64(stat.Bavail) * int64(stat.Bsize)
	return availableBytes, nil
}

// getLogDirSize calculates total size of log files matching the current extension
func (l *Logger) getLogDirSize(dir, fileExt string) (int64, error) {
	var size int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmtErrorf("failed to read log directory '%s': %w", dir, err)
	}

	targetExt := "." + fileExt
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == targetExt {
			info, errInfo := entry.Info()
			if errInfo != nil {
				continue
			}
			size += info.Size()
		}
	}
	return size, nil
}

// cleanOldLogs removes oldest log files until required space is freed
func (l *Logger) cleanOldLogs(required int64) error {
	dir, _ := l.config.String("log.directory")
	fileExt, _ := l.config.String("log.extension")

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmtErrorf("failed to read log directory '%s' for cleanup: %w", dir, err)
	}

	currentLogFileName := ""
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if clf, ok := cfPtr.(*os.File); ok && clf != nil {
			currentLogFileName = filepath.Base(clf.Name())
		}
	}

	type logFileMeta struct {
		name    string
		modTime time.Time
		size    int64
	}
	var logs []logFileMeta
	targetExt := "." + fileExt
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != targetExt || entry.Name() == currentLogFileName {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		logs = append(logs, logFileMeta{name: entry.Name(), modTime: info.ModTime(), size: info.Size()})
	}

	if len(logs) == 0 {
		if required > 0 {
			return fmtErrorf("no old logs available to delete in '%s', needed %d bytes", dir, required)
		}
		return nil
	}

	sort.Slice(logs, func(i, j int) bool { return logs[i].modTime.Before(logs[j].modTime) })

	var freedSpace int64
	for _, log := range logs {
		if required > 0 && freedSpace >= required {
			break
		}
		filePath := filepath.Join(dir, log.name)
		if err := os.Remove(filePath); err != nil {
			fmtFprintf(os.Stderr, "log: failed to remove old log file '%s': %v\n", filePath, err)
			continue
		}
		freedSpace += log.size
		l.state.TotalDeletions.Add(1)
	}

	if required > 0 && freedSpace < required {
		return fmtErrorf("could not free enough space in '%s': freed %d bytes, needed %d bytes", dir, freedSpace, required)
	}
	return nil
}

// updateEarliestFileTime scans the log directory for the oldest log file
func (l *Logger) updateEarliestFileTime() {
	dir, _ := l.config.String("log.directory")
	fileExt, _ := l.config.String("log.extension")
	baseName, _ := l.config.String("log.name")

	entries, err := os.ReadDir(dir)
	if err != nil {
		l.state.EarliestFileTime.Store(time.Time{})
		return
	}

	var earliest time.Time
	currentLogFileName := ""
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if clf, ok := cfPtr.(*os.File); ok && clf != nil {
			currentLogFileName = filepath.Base(clf.Name())
		}
	}

	targetExt := "." + fileExt
	prefix := baseName + "_"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fname := entry.Name()
		if !strings.HasPrefix(fname, prefix) || filepath.Ext(fname) != targetExt || fname == currentLogFileName {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		if earliest.IsZero() || info.ModTime().Before(earliest) {
			earliest = info.ModTime()
		}
	}
	l.state.EarliestFileTime.Store(earliest)
}

// cleanExpiredLogs removes log files older than the retention period
func (l *Logger) cleanExpiredLogs(oldest time.Time) error {
	dir, _ := l.config.String("log.directory")
	fileExt, _ := l.config.String("log.extension")
	retentionPeriodHrs, _ := l.config.Float64("log.retention_period_hrs")
	rpDuration := time.Duration(retentionPeriodHrs * float64(time.Hour))

	if rpDuration <= 0 {
		return nil
	}
	cutoffTime := time.Now().Add(-rpDuration)
	if oldest.IsZero() || !oldest.Before(cutoffTime) {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmtErrorf("failed to read log directory '%s' for retention cleanup: %w", dir, err)
	}

	currentLogFileName := ""
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if clf, ok := cfPtr.(*os.File); ok && clf != nil {
			currentLogFileName = filepath.Base(clf.Name())
		}
	}

	targetExt := "." + fileExt
	var deletedCount int
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != targetExt || entry.Name() == currentLogFileName {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(dir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				fmtFprintf(os.Stderr, "log: failed to remove expired log file '%s': %v\n", filePath, err)
			} else {
				deletedCount++
				l.state.TotalDeletions.Add(1)
			}
		}
	}

	if deletedCount == 0 && err != nil {
		return err
	}
	return nil
}

// generateLogFileName creates a unique log filename using a timestamp
func (l *Logger) generateLogFileName(timestamp time.Time) string {
	name, _ := l.config.String("log.name")
	ext, _ := l.config.String("log.extension")
	tsFormat := timestamp.Format("060102_150405")
	nano := timestamp.Nanosecond()
	return fmt.Sprintf("%s_%s_%d.%s", name, tsFormat, nano, ext)
}

// createNewLogFile generates a unique name and opens a new log file
func (l *Logger) createNewLogFile() (*os.File, error) {
	dir, _ := l.config.String("log.directory")
	filename := l.generateLogFileName(time.Now())
	fullPath := filepath.Join(dir, filename)

	for i := 0; i < 5; i++ {
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			break
		}
		time.Sleep(1 * time.Millisecond)
		filename := l.generateLogFileName(time.Now())
		fullPath = filepath.Join(dir, filename)
	}

	file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmtErrorf("failed to open/create log file '%s': %w", fullPath, err)
	}
	return file, nil
}

// rotateLogFile handles closing the current log file and opening a new one
func (l *Logger) rotateLogFile() error {
	newFile, err := l.createNewLogFile()
	if err != nil {
		return fmtErrorf("failed to create new log file for rotation: %w", err)
	}

	oldFilePtr := l.state.CurrentFile.Swap(newFile)
	l.state.CurrentSize.Store(0)

	if oldFilePtr != nil {
		if oldFile, ok := oldFilePtr.(*os.File); ok && oldFile != nil {
			if err := oldFile.Close(); err != nil {
				fmtFprintf(os.Stderr, "log: failed to close old log file '%s': %v\n", oldFile.Name(), err)
				// Continue with new file anyway
			}
		}
	}

	l.updateEarliestFileTime()
	l.state.TotalRotations.Add(1)
	return nil
}

// getLogFileCount calculates the number of log files matching the current extension
func (l *Logger) getLogFileCount(dir, fileExt string) (int, error) {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return -1, fmtErrorf("failed to read log directory '%s': %w", dir, err)
	}

	targetExt := "." + fileExt
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Count all files matching the extension, including the current one if present
		if filepath.Ext(entry.Name()) == targetExt {
			count++
		}
	}
	return count, nil
}