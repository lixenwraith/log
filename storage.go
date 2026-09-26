package log

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// performSync syncs the current log file
func (l *Logger) performSync() error {
	if !l.getConfig().EnableFile {
		return nil
	}
	if file, _ := l.state.CurrentFile.Load().(*os.File); file != nil {
		if err := file.Sync(); err != nil {
			return fmtErrorf("failed to sync log file '%s': %w", file.Name(), err)
		}
		return nil
	}
	return fmtErrorf("file output enabled without an open log file")
}

// performDiskCheck checks disk space, triggers cleanup if needed, and updates status
// Returns true if disk is OK, false otherwise
func (l *Logger) performDiskCheck(forceCleanup bool) bool {
	c := l.getConfig()
	// Skip all disk checks if file output is disabled
	enableFile := c.EnableFile
	if !enableFile {
		// Always return OK status when file output is disabled
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}

	dir := c.Directory
	ext := c.Extension
	maxTotalKB := c.MaxTotalSizeKB
	minDiskFreeKB := c.MinDiskFreeKB
	maxTotal := maxTotalKB * sizeMultiplier
	minFreeRequired := minDiskFreeKB * sizeMultiplier

	// If no limits are set, the disk is considered OK
	if maxTotal <= 0 && minFreeRequired <= 0 {
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}

	// Check available disk space
	freeSpace, err := l.getDiskFreeSpace(dir)
	if err != nil {
		l.internalLog("warning - failed to check free disk space for '%s': %v\n", dir, err)
		l.state.DiskStatusOK.Store(false)
		return false
	}

	// Determine if cleanup is needed based on disk space and total log size
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

	// Trigger cleanup if needed and allowed by the 'forceCleanup' flag
	if needsCleanupCheck && forceCleanup {
		if err := l.cleanOldLogs(spaceToFree); err != nil {
			if !l.state.DiskFullLogged.Swap(true) {
				diskFullRecord := logRecord{
					Flags: FlagDefault | FlagKV, TimeStamp: time.Now(), Level: LevelError,
					Args: []any{
						"msg", "log directory full or disk space low, cleanup failed",
						"error", err.Error(),
					},
				}
				l.sendLogRecord(diskFullRecord)
			}
			l.state.DiskStatusOK.Store(false)
			return false
		}
		// Cleanup succeeded, reset flags
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
		// Limits OK, reset flags
		if !l.state.DiskStatusOK.Load() {
			l.state.DiskStatusOK.Store(true)
			l.state.DiskFullLogged.Store(false)
		}
		return true
	}
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

	for _, entry := range entries {
		if !matchesLogEntry(entry, ext) {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		size += info.Size()
	}
	return size, nil
}

// cleanOldLogs removes oldest log files until required space is freed
func (l *Logger) cleanOldLogs(required int64) error {
	c := l.getConfig()
	if !c.EnableFile || required <= 0 {
		return nil
	}
	dir := c.Directory
	ext := c.Extension
	name := c.Name

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmtErrorf("failed to read log directory '%s' for cleanup: %w", dir, err)
	}

	// Build a list of log files eligible for deletion, excluding the active log file
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
	for _, entry := range entries {
		if !matchesLogEntry(entry, ext) || entry.Name() == staticLogName {
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

	// Sort logs by modification time to delete the oldest ones first
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].modTime.Equal(logs[j].modTime) {
			return logs[i].name < logs[j].name
		}
		return logs[i].modTime.Before(logs[j].modTime)
	})

	// Iterate and remove files until enough space has been freed
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

// updateEarliestFileTime scans the log directory for the oldest log file.
// Matches by extension only: a name-prefix filter would hide files written by
// earlier runs when the active name carries a per-run timestamp.
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

	for _, entry := range entries {
		if !matchesLogEntry(entry, ext) {
			continue
		}
		fname := entry.Name()
		if fname == staticLogName {
			continue // Skip the active log file
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

	if !c.EnableFile || rpDuration <= 0 {
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

	for _, entry := range entries {
		if !matchesLogEntry(entry, ext) || entry.Name() == staticLogName {
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

	// Extension was validated without a leading dot
	filename := name
	if ext != "" {
		filename = name + "." + ext
	}

	return filepath.Join(dir, filename)
}

// fileExists includes directories and symlinks, which also occupy archive names
func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// generateArchiveLogFileName creates a second-resolution name for a rotated
// log, disambiguating with a counter when that name is already taken
func (l *Logger) generateArchiveLogFileName(timestamp time.Time) string {
	c := l.getConfig()

	suffix := ""
	if c.Extension != "" {
		suffix = "." + c.Extension
	}
	base := fmt.Sprintf("%s_%s", c.Name, timestamp.Format("060102_150405"))

	name := base + suffix
	for i := 1; fileExists(filepath.Join(c.Directory, name)); i++ {
		name = fmt.Sprintf("%s_%d%s", base, i, suffix)
	}
	return name
}

// createNewLogFile opens the active log file in append mode
func (l *Logger) createNewLogFile() (*os.File, error) {
	return openLogFile(l.getConfig())
}

func openLogFile(c *Config) (*os.File, error) {
	name := c.Name
	if c.Extension != "" {
		name += "." + c.Extension
	}
	fullPath := filepath.Join(c.Directory, name)
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
		// This can happen if file logging was disabled and re-enabled
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
		// Invalid file handle in state, treat as if there's no file
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

	l.state.CurrentFile.Store((*os.File)(nil))

	// Generate a new unique name with current timestamp for the old log file
	dir := c.Directory
	archiveName := l.generateArchiveLogFileName(time.Now())
	archivePath := filepath.Join(dir, archiveName)

	// Rename current file to archive name
	currentPath := l.getStaticLogFilePath()
	if err := os.Rename(currentPath, archivePath); err != nil {
		// Critical failure: the original file is closed and couldn't be renamed
		// This is a terminal state for file logging
		l.internalLog("failed to rename log file from '%s' to '%s': %v. file logging disabled.",
			currentPath, archivePath, err)
		l.state.LoggerDisabled.Store(true)
		l.refreshLevelGate()
		return fmtErrorf("failed to rotate log file, logging is disabled: %w", err)
	}

	// Create new log file at static path
	newFile, err := l.createNewLogFile()
	if err != nil {
		l.state.LoggerDisabled.Store(true)
		l.refreshLevelGate()
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

	for _, entry := range entries {
		if !matchesLogEntry(entry, ext) {
			continue
		}
		// Count all files matching the extension, including the current one if present
		count++
	}
	return count, nil
}

// Cleanup is scoped to regular files with this extension, including files from
// earlier runs with different base names. Extensionless logs match only files
// without an extension. Each logger must own its directory exclusively.
func matchesLogEntry(entry os.DirEntry, ext string) bool {
	if !entry.Type().IsRegular() {
		return false
	}
	if ext == "" {
		return filepath.Ext(entry.Name()) == ""
	}
	return strings.HasSuffix(entry.Name(), "."+ext)
}
