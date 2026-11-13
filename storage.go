package log

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// performSync syncs the current log file; a success clears the unsynced mark
func (l *Logger) performSync() error {
	if !l.getConfig().EnableFile {
		return nil
	}
	if file, _ := l.state.CurrentFile.Load().(*os.File); file != nil {
		if err := file.Sync(); err != nil {
			return fmtErrorf("failed to sync log file '%s': %w", file.Name(), err)
		}
		l.watch.unsynced = false
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
		dirSize, err := l.getLogDirSize(c)
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

// getLogDirSize calculates the total size of the files matchesLogEntry selects
func (l *Logger) getLogDirSize(c *Config) (int64, error) {
	dir := c.Directory
	var size int64
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmtErrorf("failed to read log directory '%s': %w", dir, err)
	}

	for _, entry := range entries {
		if !matchesLogEntry(entry, c) {
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
	active := l.openFileInfo()
	var logs []logFileMeta
	for _, entry := range entries {
		if !matchesLogEntry(entry, c) || entry.Name() == staticLogName {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil || os.SameFile(info, active) {
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

	for _, entry := range entries {
		if !matchesLogEntry(entry, c) {
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

	active := l.openFileInfo()
	for _, entry := range entries {
		if !matchesLogEntry(entry, c) || entry.Name() == staticLogName {
			continue
		}
		info, errInfo := entry.Info()
		if errInfo != nil || os.SameFile(info, active) {
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

// openFileInfo identifies the file being written; cleanup spares it under any
// name, as it may have been renamed since the last path check
func (l *Logger) openFileInfo() os.FileInfo {
	if file, _ := l.state.CurrentFile.Load().(*os.File); file != nil {
		if info, err := file.Stat(); err == nil {
			return info
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

// openLogPath opens the log path again after a rotation or an external change.
// Unlike the configured first open it never follows a symlink, never blocks on
// a FIFO, and keeps only a regular file: the path may have been replaced.
func openLogPath(path string) (*os.File, os.FileInfo, error) {
	file, err := os.OpenFile(path, reopenFlags, 0644)
	if err != nil {
		return nil, nil, fmtErrorf("failed to open/create log file '%s': %w", path, err)
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = errors.New("not a regular file")
	}
	if err != nil {
		_ = file.Close()
		return nil, nil, fmtErrorf("refused log file '%s': %w", path, err)
	}
	return file, info, nil
}

// checkLogPath keeps the processor on its log path. At most once per
// fileCheckInterval of record time, or when forced, it compares the path with
// the open file: a path that is gone or names another file (rename-on-save
// editors, sed -i, rm, mv) is opened afresh. CurrentSize is resynced from the
// file either way. Reports whether the check ran. Processor goroutine only.
func (l *Logger) checkLogPath(now time.Time, force bool) bool {
	w := &l.watch
	if !force && now.Sub(w.lastCheck) < fileCheckInterval {
		return false
	}
	w.lastCheck = now

	file, _ := l.state.CurrentFile.Load().(*os.File)
	if file == nil {
		return true
	}
	if w.file != file {
		info, err := file.Stat()
		if err != nil {
			w.file = nil
			return true // unusable handle: the write reports it
		}
		w.file, w.info, w.path = file, info, l.getStaticLogFilePath()
	}

	pathInfo, err := os.Stat(w.path)
	if err == nil && os.SameFile(pathInfo, w.info) {
		l.state.CurrentSize.Store(pathInfo.Size())
		w.warned = false
		return true
	}
	if errors.Is(err, fs.ErrPermission) {
		return true // cannot tell: keep the open file
	}
	newFile, info, err := openLogPath(w.path)
	if err != nil {
		if !w.warned {
			w.warned = true
			l.internalLog("log path no longer names the open file, not reopened: %v\n", err)
		}
		return true
	}
	l.state.CurrentFile.Store(newFile)
	if err := file.Close(); err != nil {
		l.internalLog("failed to close replaced log file: %v\n", err)
	}
	w.file, w.info, w.warned = newFile, info, false
	l.state.CurrentSize.Store(info.Size())
	l.state.TotalReopens.Add(1)
	return true
}

// rotateLogFile implements the rename-on-rotate strategy
// Closes current file, renames it with timestamp, creates new static file
func (l *Logger) rotateLogFile() error {
	c := l.getConfig()
	currentPath := l.getStaticLogFilePath()

	currentFile, _ := l.state.CurrentFile.Load().(*os.File)
	if currentFile == nil {
		// This can happen if file logging was disabled and re-enabled
		// No current file, just create a new one
		newFile, info, err := openLogPath(currentPath)
		if err != nil {
			return fmtErrorf("failed to create log file during rotation: %w", err)
		}
		l.state.CurrentFile.Store(newFile)
		l.state.CurrentSize.Store(info.Size())
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
	archivePath := filepath.Join(c.Directory, l.generateArchiveLogFileName(time.Now()))

	// Rename current file to archive name
	if err := os.Rename(currentPath, archivePath); errors.Is(err, fs.ErrNotExist) {
		// Removed since the last path check: nothing to archive
		l.internalLog("log file '%s' was removed before rotation, nothing to archive\n", currentPath)
	} else if err != nil {
		// Critical failure: the original file is closed and couldn't be renamed
		// This is a terminal state for file logging
		l.internalLog("failed to rename log file from '%s' to '%s': %v. file logging disabled.",
			currentPath, archivePath, err)
		l.state.LoggerDisabled.Store(true)
		l.refreshLevelGate()
		return fmtErrorf("failed to rotate log file, logging is disabled: %w", err)
	}

	// Create new log file at static path
	newFile, info, err := openLogPath(currentPath)
	if err != nil {
		l.state.LoggerDisabled.Store(true)
		l.refreshLevelGate()
		return fmtErrorf("failed to create new log file after rotation: %w", err)
	}

	// Update state
	l.state.CurrentFile.Store(newFile)
	l.state.CurrentSize.Store(info.Size())
	l.state.TotalRotations.Add(1)

	// Update earliest file time after successful rotation
	l.updateEarliestFileTime()

	return nil
}

// getLogFileCount counts the files matchesLogEntry selects
func (l *Logger) getLogFileCount(c *Config) (int, error) {
	dir := c.Directory
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return -1, fmtErrorf("failed to read log directory '%s': %w", dir, err)
	}

	for _, entry := range entries {
		if !matchesLogEntry(entry, c) {
			continue
		}
		count++ // the active file included
	}
	return count, nil
}

// matchesLogEntry selects the regular files that cleanup, retention and size
// limits manage. By extension by default, so files of earlier runs under
// per-run names are still cleaned, and the logger must own its directory; with
// CleanupOwnFiles only {name}.{ext} and its archives, so others can share it.
func matchesLogEntry(entry os.DirEntry, c *Config) bool {
	if !entry.Type().IsRegular() {
		return false
	}
	name := entry.Name()
	if c.CleanupOwnFiles {
		return isOwnLogName(name, c.Name, c.Extension)
	}
	if c.Extension == "" {
		return filepath.Ext(name) == ""
	}
	return strings.HasSuffix(name, "."+c.Extension)
}

// isOwnLogName reports whether file is base's active file or an archive that
// generateArchiveLogFileName makes: base_YYMMDD_HHMMSS[_N], then the extension
func isOwnLogName(file, base, ext string) bool {
	if ext != "" {
		var ok bool
		if file, ok = strings.CutSuffix(file, "."+ext); !ok {
			return false
		}
	}
	rest, ok := strings.CutPrefix(file, base)
	if !ok || rest == "" {
		return ok
	}
	if len(rest) < 14 || rest[0] != '_' || rest[7] != '_' || !isDigits(rest[1:7]) || !isDigits(rest[8:14]) {
		return false
	}
	rest = rest[14:]
	return rest == "" || len(rest) > 1 && rest[0] == '_' && isDigits(rest[1:])
}

func isDigits(s string) bool {
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
