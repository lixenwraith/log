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
	c := l.getConfig()
	// Skip sync if file output is disabled
	disableFile := c.DisableFile
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
	c := l.getConfig()
	// Skip all disk checks if file output is disabled
	disableFile := c.DisableFile
	if disableFile {
		// Always return OK status when file output is disabled
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}

	dir := c.Directory
	ext := c.Extension
	maxTotalMB := c.MaxTotalSizeMB
	minDiskFreeMB := c.MinDiskFreeMB
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
		l.internalLog("warning - failed to check free disk space for '%s': %v\n", dir, err)
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
			l.internalLog("warning - failed to check log directory size for '%s': %v\n", dir, err)
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
func (l *Logger) getLogDirSize(dir, ext string) (int64, error) {
	var size int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmtErrorf("failed to read log directory '%s': %w", dir, err)
	}

	targetExt := "." + ext
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
	c := l.getConfig()
	dir := c.Directory
	ext := c.Extension
	name := c.Name

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmtErrorf("failed to read log directory '%s' for cleanup: %w", dir, err)
	}

	// Get the static log filename to exclude from deletion
	staticLogName := name
	if ext != "" {
		staticLogName = name + "." + ext
	}

	type logFileMeta struct {
		name    string
		modTime time.Time
		size    int64
	}
	var logs []logFileMeta
	targetExt := "." + ext
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == staticLogName {
			continue
		}
		if ext != "" && filepath.Ext(entry.Name()) != targetExt {
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
			l.internalLog("failed to remove old log file '%s': %v\n", filePath, err)
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
	c := l.getConfig()
	dir := c.Directory
	ext := c.Extension
	name := c.Name

	entries, err := os.ReadDir(dir)
	if err != nil {
		l.state.EarliestFileTime.Store(time.Time{})
		return
	}

	var earliest time.Time
	// Get the active log filename to exclude from timestamp tracking
	staticLogName := name
	if ext != "" {
		staticLogName = name + "." + ext
	}

	targetExt := "." + ext
	prefix := name + "_"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fname := entry.Name()
		// Skip the active log file
		if fname == staticLogName {
			continue
		}
		if !strings.HasPrefix(fname, prefix) || (ext != "" && filepath.Ext(fname) != targetExt) {
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
	c := l.getConfig()
	dir := c.Directory
	ext := c.Extension
	name := c.Name
	retentionPeriodHrs := c.RetentionPeriodHrs
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

	// Get the active log filename to exclude from deletion
	staticLogName := name
	if ext != "" {
		staticLogName = name + "." + ext
	}

	targetExt := "." + ext
	var deletedCount int
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == staticLogName {
			continue
		}
		// Only consider files with correct extension
		if ext != "" && filepath.Ext(entry.Name()) != targetExt {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		if info.ModTime().Before(cutoffTime) {
			filePath := filepath.Join(dir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				l.internalLog("failed to remove expired log file '%s': %v\n", filePath, err)
			} else {
				deletedCount++
				l.state.TotalDeletions.Add(1)
			}
		}
	}

	return nil
}

// getStaticLogFilePath returns the full path to the active log file
func (l *Logger) getStaticLogFilePath() string {
	c := l.getConfig()
	dir := c.Directory
	ext := c.Extension
	name := c.Name

	// Handle extension with or without dot
	filename := name
	if ext != "" {
		filename = name + "." + ext
	}

	return filepath.Join(dir, filename)
}

// generateArchiveLogFileName creates a timestamped filename for archived logs during rotation
func (l *Logger) generateArchiveLogFileName(timestamp time.Time) string {
	c := l.getConfig()
	ext := c.Extension
	name := c.Name

	tsFormat := timestamp.Format("060102_150405")
	nano := timestamp.Nanosecond()

	if ext != "" {
		return fmt.Sprintf("%s_%s_%d.%s", name, tsFormat, nano, ext)
	}
	return fmt.Sprintf("%s_%s_%d", name, tsFormat, nano)
}

// createNewLogFile generates a unique name and opens a new log file
func (l *Logger) createNewLogFile() (*os.File, error) {
	fullPath := l.getStaticLogFilePath()

	file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmtErrorf("failed to open/create log file '%s': %w", fullPath, err)
	}
	return file, nil
}

// rotateLogFile implements the rename-on-rotate strategy
// Closes current file, renames it with timestamp, creates new static file
func (l *Logger) rotateLogFile() error {
	c := l.getConfig()

	// Get current file handle
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr == nil {
		// No current file, just create a new one
		newFile, err := l.createNewLogFile()
		if err != nil {
			return fmtErrorf("failed to create log file during rotation: %w", err)
		}
		l.state.CurrentFile.Store(newFile)
		l.state.CurrentSize.Store(0)
		l.state.TotalRotations.Add(1)
		return nil
	}

	currentFile, ok := cfPtr.(*os.File)
	if !ok || currentFile == nil {
		// Invalid file handle, create new one
		newFile, err := l.createNewLogFile()
		if err != nil {
			return fmtErrorf("failed to create log file during rotation: %w", err)
		}
		l.state.CurrentFile.Store(newFile)
		l.state.CurrentSize.Store(0)
		l.state.TotalRotations.Add(1)
		return nil
	}

	// Close current file before renaming
	if err := currentFile.Close(); err != nil {
		l.internalLog("failed to close log file before rotation: %v\n", err)
		// Continue with rotation anyway
	}

	// Generate archive filename with current timestamp
	dir := c.Directory
	archiveName := l.generateArchiveLogFileName(time.Now())
	archivePath := filepath.Join(dir, archiveName)

	// Rename current file to archive name
	currentPath := l.getStaticLogFilePath()
	if err := os.Rename(currentPath, archivePath); err != nil {
		// The original file is closed and couldn't be renamed. This is a terminal state for file logging.
		l.internalLog("failed to rename log file from '%s' to '%s': %v. file logging disabled.",
			currentPath, archivePath, err)
		l.state.LoggerDisabled.Store(true)
		return fmtErrorf("failed to rotate log file, logging is disabled: %w", err)
	}

	// Create new log file at static path
	newFile, err := l.createNewLogFile()
	if err != nil {
		return fmtErrorf("failed to create new log file after rotation: %w", err)
	}

	// Update state
	l.state.CurrentFile.Store(newFile)
	l.state.CurrentSize.Store(0)
	l.state.TotalRotations.Add(1)

	// Update earliest file time after successful rotation
	l.updateEarliestFileTime()

	return nil
}

// getLogFileCount calculates the number of log files matching the current extension
func (l *Logger) getLogFileCount(dir, ext string) (int, error) {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return -1, fmtErrorf("failed to read log directory '%s': %w", dir, err)
	}

	targetExt := "." + ext
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