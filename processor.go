// --- File: processor.go ---
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

const (
	// Threshold for triggering reactive disk check
	reactiveCheckThresholdBytes int64 = 10 * 1024 * 1024
	// Factors to adjust check interval
	adaptiveIntervalFactor float64 = 1.5 // Slow down factor
	adaptiveSpeedUpFactor  float64 = 0.8 // Speed up factor
)

// processLogs is the main log processing loop running in a separate goroutine
func (l *Logger) processLogs(ch <-chan logRecord) {
	l.state.ProcessorExited.Store(false)      // Mark processor as running
	defer l.state.ProcessorExited.Store(true) // Ensure flag is set on exit

	// Get configuration values for setup
	flushInterval, _ := l.config.Int64("log.flush_interval_ms")
	if flushInterval <= 0 {
		flushInterval = 100
	}
	flushTicker := time.NewTicker(time.Duration(flushInterval) * time.Millisecond)
	defer flushTicker.Stop()

	// Retention Timer
	var retentionTicker *time.Ticker
	var retentionChan <-chan time.Time = nil
	retentionPeriodHrs, _ := l.config.Float64("log.retention_period_hrs")
	retentionCheckMins, _ := l.config.Float64("log.retention_check_mins")
	retentionDur := time.Duration(retentionPeriodHrs * float64(time.Hour))
	retentionCheckInterval := time.Duration(retentionCheckMins * float64(time.Minute))

	if retentionDur > 0 && retentionCheckInterval > 0 {
		retentionTicker = time.NewTicker(retentionCheckInterval)
		defer retentionTicker.Stop()
		retentionChan = retentionTicker.C
		l.updateEarliestFileTime() // Initial check
	}

	// Disk Check Timer
	diskCheckIntervalMs, _ := l.config.Int64("log.disk_check_interval_ms")
	if diskCheckIntervalMs <= 0 {
		diskCheckIntervalMs = 5000
	}
	currentDiskCheckInterval := time.Duration(diskCheckIntervalMs) * time.Millisecond

	// Ensure initial interval respects bounds
	minCheckIntervalMs, _ := l.config.Int64("log.min_check_interval_ms")
	maxCheckIntervalMs, _ := l.config.Int64("log.max_check_interval_ms")
	minCheckInterval := time.Duration(minCheckIntervalMs) * time.Millisecond
	maxCheckInterval := time.Duration(maxCheckIntervalMs) * time.Millisecond

	if currentDiskCheckInterval < minCheckInterval {
		currentDiskCheckInterval = minCheckInterval
	}
	if currentDiskCheckInterval > maxCheckInterval {
		currentDiskCheckInterval = maxCheckInterval
	}

	diskCheckTicker := time.NewTicker(currentDiskCheckInterval)
	defer diskCheckTicker.Stop()

	// --- State Variables ---
	var bytesSinceLastCheck int64 = 0
	var lastCheckTime time.Time = time.Now()
	var logsSinceLastCheck int64 = 0

	// Perform an initial disk check on startup
	l.performDiskCheck(true) // Force check and update status

	// --- Main Loop ---
	for {
		select {
		case record, ok := <-ch:
			if !ok {
				// Channel closed: Perform final sync and exit
				l.performSync()
				return
			}

			// --- Process the received record ---
			if !l.state.DiskStatusOK.Load() {
				l.state.DroppedLogs.Add(1)
				continue // Skip processing if disk known to be unavailable
			}

			// Serialize the record
			format, _ := l.config.String("log.format")
			data := l.serializer.serialize(
				format,
				record.Flags,
				record.TimeStamp,
				record.Level,
				record.Trace,
				record.Args,
			)
			dataLen := int64(len(data))

			// Check for rotation
			currentFileSize := l.state.CurrentSize.Load()
			estimatedSize := currentFileSize + dataLen

			maxSizeMB, _ := l.config.Int64("log.max_size_mb")
			if maxSizeMB > 0 && estimatedSize > maxSizeMB*1024*1024 {
				if err := l.rotateLogFile(); err != nil {
					fmtFprintf(os.Stderr, "log: failed to rotate log file: %v\n", err)
				}
				bytesSinceLastCheck = 0 // Reset counters after rotation
				logsSinceLastCheck = 0
			}

			// Write to the current log file
			cfPtr := l.state.CurrentFile.Load()
			if currentLogFile, isFile := cfPtr.(*os.File); isFile && currentLogFile != nil {
				n, err := currentLogFile.Write(data)
				if err != nil {
					fmtFprintf(os.Stderr, "log: failed to write to log file: %v\n", err)
					l.state.DroppedLogs.Add(1)
					l.performDiskCheck(true) // Force check if write fails
				} else {
					l.state.CurrentSize.Add(int64(n))
					bytesSinceLastCheck += int64(n)
					logsSinceLastCheck++

					// Reactive Check Trigger
					if bytesSinceLastCheck > reactiveCheckThresholdBytes {
						if l.performDiskCheck(false) { // Check without forcing cleanup yet
							bytesSinceLastCheck = 0 // Reset if check OK
							logsSinceLastCheck = 0
							lastCheckTime = time.Now()
						}
					}
				}
			} else {
				l.state.DroppedLogs.Add(1) // File pointer somehow nil
			}

		case <-flushTicker.C:
			enableSync, _ := l.config.Bool("log.enable_periodic_sync")
			if enableSync {
				l.performSync()
			}

		case <-diskCheckTicker.C:
			// Periodic disk check
			if l.performDiskCheck(true) { // Periodic check, force cleanup if needed
				enableAdaptive, _ := l.config.Bool("log.enable_adaptive_interval")
				if enableAdaptive {
					elapsed := time.Since(lastCheckTime)
					if elapsed < 10*time.Millisecond {
						elapsed = 10 * time.Millisecond
					}

					logsPerSecond := float64(logsSinceLastCheck) / elapsed.Seconds()
					targetLogsPerSecond := float64(100) // Baseline

					if logsPerSecond < targetLogsPerSecond/2 { // Load low -> increase interval
						currentDiskCheckInterval = time.Duration(float64(currentDiskCheckInterval) * adaptiveIntervalFactor)
					} else if logsPerSecond > targetLogsPerSecond*2 { // Load high -> decrease interval
						currentDiskCheckInterval = time.Duration(float64(currentDiskCheckInterval) * adaptiveSpeedUpFactor)
					}

					// Clamp interval using current config
					minCheckIntervalMs, _ := l.config.Int64("log.min_check_interval_ms")
					maxCheckIntervalMs, _ := l.config.Int64("log.max_check_interval_ms")
					minCheckInterval := time.Duration(minCheckIntervalMs) * time.Millisecond
					maxCheckInterval := time.Duration(maxCheckIntervalMs) * time.Millisecond

					if currentDiskCheckInterval < minCheckInterval {
						currentDiskCheckInterval = minCheckInterval
					}
					if currentDiskCheckInterval > maxCheckInterval {
						currentDiskCheckInterval = maxCheckInterval
					}

					diskCheckTicker.Reset(currentDiskCheckInterval)
				}
				// Reset counters after successful periodic check
				bytesSinceLastCheck = 0
				logsSinceLastCheck = 0
				lastCheckTime = time.Now()
			}

		case confirmChan := <-l.state.flushRequestChan:
			l.performSync()
			close(confirmChan) // Signal completion back to the Flush caller

		case <-retentionChan:
			// Check file retention
			retentionPeriodHrs, _ := l.config.Float64("log.retention_period_hrs")
			retentionDur := time.Duration(retentionPeriodHrs * float64(time.Hour))

			if retentionDur > 0 {
				etPtr := l.state.EarliestFileTime.Load()
				if earliest, ok := etPtr.(time.Time); ok && !earliest.IsZero() {
					if time.Since(earliest) > retentionDur {
						if err := l.cleanExpiredLogs(earliest); err == nil {
							l.updateEarliestFileTime()
						} else {
							fmtFprintf(os.Stderr, "log: failed to clean expired logs: %v\n", err)
						}
					}
				} else if !ok || earliest.IsZero() {
					l.updateEarliestFileTime()
				}
			}
		}
	}
}

// performSync syncs the current log file
func (l *Logger) performSync() {
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

	// Retry logic for potential collisions (rare)
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
	l.state.CurrentSize.Store(0) // Reset size for the new file

	if oldFilePtr != nil {
		if oldFile, ok := oldFilePtr.(*os.File); ok && oldFile != nil {
			if err := oldFile.Close(); err != nil {
				fmtFprintf(os.Stderr, "log: failed to close old log file '%s': %v\n", oldFile.Name(), err)
				// Continue with new file anyway
			}
		}
	}

	l.updateEarliestFileTime() // Update earliest time after rotation
	return nil
}