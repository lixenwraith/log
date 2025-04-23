// --- File: processor.go ---
package log

import (
	"fmt"
	"os"
	"runtime"
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

	// Set up timers and state variables
	timers := l.setupProcessingTimers()
	defer l.closeProcessingTimers(timers)

	// Perform an initial disk check on startup
	l.performDiskCheck(true) // Force check and update status

	// Send initial heartbeats immediately instead of waiting for first tick
	heartbeatLevel, _ := l.config.Int64("log.heartbeat_level")
	if heartbeatLevel > 0 {
		if heartbeatLevel >= 1 {
			l.logProcHeartbeat()
		}
		if heartbeatLevel >= 2 {
			l.logDiskHeartbeat()
		}
		if heartbeatLevel >= 3 {
			l.logSysHeartbeat()
		}
	}

	// State variables for adaptive disk checks
	var bytesSinceLastCheck int64 = 0
	var lastCheckTime time.Time = time.Now()
	var logsSinceLastCheck int64 = 0

	// --- Main Loop ---
	for {
		select {
		case record, ok := <-ch:
			if !ok {
				// Channel closed: Perform final sync and exit
				l.performSync()
				return
			}

			// Process the received log record
			bytesWritten := l.processLogRecord(record)
			if bytesWritten > 0 {
				// Update adaptive check counters
				bytesSinceLastCheck += bytesWritten
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

		case <-timers.flushTicker.C:
			l.handleFlushTick()

		case <-timers.diskCheckTicker.C:
			// Periodic disk check
			if l.performDiskCheck(true) { // Periodic check, force cleanup if needed
				l.adjustDiskCheckInterval(timers, lastCheckTime, logsSinceLastCheck)
				// Reset counters after successful periodic check
				bytesSinceLastCheck = 0
				logsSinceLastCheck = 0
				lastCheckTime = time.Now()
			}

		case confirmChan := <-l.state.flushRequestChan:
			l.handleFlushRequest(confirmChan)

		case <-timers.retentionChan:
			l.handleRetentionCheck()

		case <-timers.heartbeatChan:
			l.handleHeartbeat()
		}
	}
}

// TimerSet holds all timers used in processLogs
type TimerSet struct {
	flushTicker     *time.Ticker
	diskCheckTicker *time.Ticker
	retentionTicker *time.Ticker
	heartbeatTicker *time.Ticker
	retentionChan   <-chan time.Time
	heartbeatChan   <-chan time.Time
}

// setupProcessingTimers creates and configures all necessary timers for the processor
func (l *Logger) setupProcessingTimers() *TimerSet {
	timers := &TimerSet{}

	// Set up flush timer
	flushInterval, _ := l.config.Int64("log.flush_interval_ms")
	if flushInterval <= 0 {
		flushInterval = 100
	}
	timers.flushTicker = time.NewTicker(time.Duration(flushInterval) * time.Millisecond)

	// Set up retention timer if enabled
	timers.retentionChan = l.setupRetentionTimer(timers)

	// Set up disk check timer
	timers.diskCheckTicker = l.setupDiskCheckTimer()

	// Set up heartbeat timer
	timers.heartbeatChan = l.setupHeartbeatTimer(timers)

	return timers
}

// closeProcessingTimers stops all active timers
func (l *Logger) closeProcessingTimers(timers *TimerSet) {
	timers.flushTicker.Stop()
	if timers.diskCheckTicker != nil {
		timers.diskCheckTicker.Stop()
	}
	if timers.retentionTicker != nil {
		timers.retentionTicker.Stop()
	}
	if timers.heartbeatTicker != nil {
		timers.heartbeatTicker.Stop()
	}
}

// setupRetentionTimer configures the retention check timer if retention is enabled
func (l *Logger) setupRetentionTimer(timers *TimerSet) <-chan time.Time {
	retentionPeriodHrs, _ := l.config.Float64("log.retention_period_hrs")
	retentionCheckMins, _ := l.config.Float64("log.retention_check_mins")
	retentionDur := time.Duration(retentionPeriodHrs * float64(time.Hour))
	retentionCheckInterval := time.Duration(retentionCheckMins * float64(time.Minute))

	if retentionDur > 0 && retentionCheckInterval > 0 {
		timers.retentionTicker = time.NewTicker(retentionCheckInterval)
		l.updateEarliestFileTime() // Initial check
		return timers.retentionTicker.C
	}
	return nil
}

// setupDiskCheckTimer configures the disk check timer
func (l *Logger) setupDiskCheckTimer() *time.Ticker {
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

	return time.NewTicker(currentDiskCheckInterval)
}

// setupHeartbeatTimer configures the heartbeat timer if heartbeats are enabled
func (l *Logger) setupHeartbeatTimer(timers *TimerSet) <-chan time.Time {
	heartbeatLevel, _ := l.config.Int64("log.heartbeat_level")
	if heartbeatLevel > 0 {
		intervalS, _ := l.config.Int64("log.heartbeat_interval_s")
		// Make sure interval is positive
		if intervalS <= 0 {
			intervalS = 60 // Default to 60 seconds
		}
		// Create a new ticker that's offset slightly to avoid skipping the first tick
		// by creating it and then waiting until exactly the next interval time
		timers.heartbeatTicker = time.NewTicker(time.Duration(intervalS) * time.Second)
		return timers.heartbeatTicker.C
	}
	return nil
}

// processLogRecord handles individual log records, returning bytes written
func (l *Logger) processLogRecord(record logRecord) int64 {
	if !l.state.DiskStatusOK.Load() {
		l.state.DroppedLogs.Add(1)
		return 0 // Skip processing if disk known to be unavailable
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
	}

	// Write to the current log file
	cfPtr := l.state.CurrentFile.Load()
	if currentLogFile, isFile := cfPtr.(*os.File); isFile && currentLogFile != nil {
		n, err := currentLogFile.Write(data)
		if err != nil {
			fmtFprintf(os.Stderr, "log: failed to write to log file: %v\n", err)
			l.state.DroppedLogs.Add(1)
			l.performDiskCheck(true) // Force check if write fails
			return 0
		} else {
			l.state.CurrentSize.Add(int64(n))
			l.state.TotalLogsProcessed.Add(1)
			return int64(n)
		}
	} else {
		l.state.DroppedLogs.Add(1) // File pointer somehow nil
		return 0
	}
}

// handleFlushTick handles the periodic flush timer tick
func (l *Logger) handleFlushTick() {
	enableSync, _ := l.config.Bool("log.enable_periodic_sync")
	if enableSync {
		l.performSync()
	}
}

// handleFlushRequest handles an explicit flush request
func (l *Logger) handleFlushRequest(confirmChan chan struct{}) {
	l.performSync()
	close(confirmChan) // Signal completion back to the Flush caller
}

// handleRetentionCheck performs file retention check and cleanup
func (l *Logger) handleRetentionCheck() {
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

// adjustDiskCheckInterval modifies the disk check interval based on logging activity
func (l *Logger) adjustDiskCheckInterval(timers *TimerSet, lastCheckTime time.Time, logsSinceLastCheck int64) {
	enableAdaptive, _ := l.config.Bool("log.enable_adaptive_interval")
	if !enableAdaptive {
		return
	}

	elapsed := time.Since(lastCheckTime)
	if elapsed < 10*time.Millisecond { // Min arbitrary reasonable value
		elapsed = 10 * time.Millisecond
	}

	logsPerSecond := float64(logsSinceLastCheck) / elapsed.Seconds()
	targetLogsPerSecond := float64(100) // Baseline

	// Get current disk check interval from config
	diskCheckIntervalMs, _ := l.config.Int64("log.disk_check_interval_ms")
	currentDiskCheckInterval := time.Duration(diskCheckIntervalMs) * time.Millisecond

	// Calculate the new interval
	var newInterval time.Duration
	if logsPerSecond < targetLogsPerSecond/2 { // Load low -> increase interval
		newInterval = time.Duration(float64(currentDiskCheckInterval) * adaptiveIntervalFactor)
	} else if logsPerSecond > targetLogsPerSecond*2 { // Load high -> decrease interval
		newInterval = time.Duration(float64(currentDiskCheckInterval) * adaptiveSpeedUpFactor)
	} else {
		// No change needed if within normal range
		return
	}

	// Clamp interval using current config
	minCheckIntervalMs, _ := l.config.Int64("log.min_check_interval_ms")
	maxCheckIntervalMs, _ := l.config.Int64("log.max_check_interval_ms")
	minCheckInterval := time.Duration(minCheckIntervalMs) * time.Millisecond
	maxCheckInterval := time.Duration(maxCheckIntervalMs) * time.Millisecond

	if newInterval < minCheckInterval {
		newInterval = minCheckInterval
	}
	if newInterval > maxCheckInterval {
		newInterval = maxCheckInterval
	}

	// Reset the ticker with the new interval
	timers.diskCheckTicker.Reset(newInterval)
}

// handleHeartbeat processes a heartbeat timer tick
func (l *Logger) handleHeartbeat() {
	heartbeatLevel, _ := l.config.Int64("log.heartbeat_level")

	// Process heartbeat based on configured level
	if heartbeatLevel >= 1 {
		l.logProcHeartbeat()
	}

	if heartbeatLevel >= 2 {
		l.logDiskHeartbeat()
	}

	if heartbeatLevel >= 3 {
		l.logSysHeartbeat()
	}
}

// logProcHeartbeat logs process/logger statistics heartbeat
func (l *Logger) logProcHeartbeat() {
	// 1. Gather process/logger stats
	processed := l.state.TotalLogsProcessed.Load()
	dropped := l.state.DroppedLogs.Load()
	sequence := l.state.HeartbeatSequence.Add(1) // Increment and get sequence number

	// Calculate uptime
	startTimeVal := l.state.LoggerStartTime.Load()
	var uptimeHours float64 = 0
	if startTime, ok := startTimeVal.(time.Time); ok && !startTime.IsZero() {
		uptime := time.Since(startTime)
		uptimeHours = uptime.Hours()
	}

	// 2. Format Args
	procArgs := []any{
		"type", "proc",
		"sequence", sequence,
		"uptime_hours", fmt.Sprintf("%.2f", uptimeHours),
		"processed_logs", processed,
		"dropped_logs", dropped,
	}

	// 3. Write the heartbeat record
	l.writeHeartbeatRecord(LevelProc, procArgs)
}

// logDiskHeartbeat logs disk/file statistics heartbeat
func (l *Logger) logDiskHeartbeat() {
	sequence := l.state.HeartbeatSequence.Load()
	rotations := l.state.TotalRotations.Load()
	deletions := l.state.TotalDeletions.Load()

	// Get file system stats
	dir, _ := l.config.String("log.directory")
	ext, _ := l.config.String("log.extension")
	currentSizeMB := float64(l.state.CurrentSize.Load()) / (1024 * 1024) // Current file size
	totalSizeMB := float64(-1.0)                                         // Default error value
	fileCount := -1                                                      // Default error value

	dirSize, err := l.getLogDirSize(dir, ext)
	if err == nil {
		totalSizeMB = float64(dirSize) / (1024 * 1024)
	} else {
		fmtFprintf(os.Stderr, "log: warning - heartbeat failed to get dir size: %v\n", err)
	}

	count, err := l.getLogFileCount(dir, ext)
	if err == nil {
		fileCount = count
	} else {
		fmtFprintf(os.Stderr, "log: warning - heartbeat failed to get file count: %v\n", err)
	}

	// Format Args
	diskArgs := []any{
		"type", "disk",
		"sequence", sequence,
		"rotated_files", rotations,
		"deleted_files", deletions,
		"total_log_size_mb", fmt.Sprintf("%.2f", totalSizeMB),
		"log_file_count", fileCount,
		"current_file_size_mb", fmt.Sprintf("%.2f", currentSizeMB),
		"disk_status_ok", l.state.DiskStatusOK.Load(),
	}

	// Add disk free space if we can get it
	freeSpace, err := l.getDiskFreeSpace(dir)
	if err == nil {
		freeSpaceMB := float64(freeSpace) / (1024 * 1024)
		diskArgs = append(diskArgs, "disk_free_mb", fmt.Sprintf("%.2f", freeSpaceMB))
	}

	// Write the heartbeat record
	l.writeHeartbeatRecord(LevelDisk, diskArgs)
}

// logSysHeartbeat logs system/runtime statistics heartbeat
func (l *Logger) logSysHeartbeat() {
	sequence := l.state.HeartbeatSequence.Load()

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Format Args
	sysArgs := []any{
		"type", "sys",
		"sequence", sequence,
		"alloc_mb", fmt.Sprintf("%.2f", float64(memStats.Alloc)/(1024*1024)),
		"sys_mb", fmt.Sprintf("%.2f", float64(memStats.Sys)/(1024*1024)),
		"num_gc", memStats.NumGC,
		"num_goroutine", runtime.NumGoroutine(),
	}

	// Write the heartbeat record
	l.writeHeartbeatRecord(LevelSys, sysArgs)
}

// writeHeartbeatRecord handles the common logic for writing a heartbeat record
func (l *Logger) writeHeartbeatRecord(level int64, args []any) {
	// Skip if logger disabled or shutting down
	if l.state.LoggerDisabled.Load() || l.state.ShutdownCalled.Load() {
		return
	}

	// Skip if disk known to be unavailable
	if !l.state.DiskStatusOK.Load() {
		return
	}

	// 1. Serialize the record
	format, _ := l.config.String("log.format")
	// Use FlagDefault | FlagShowLevel so Level appears in the output
	hbData := l.serializer.serialize(format, FlagDefault|FlagShowLevel, time.Now(), level, "", args)

	// 2. Write the record
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr == nil {
		fmtFprintf(os.Stderr, "log: error - current file handle is nil during heartbeat\n")
		return
	}

	currentLogFile, isFile := cfPtr.(*os.File)
	if !isFile || currentLogFile == nil {
		fmtFprintf(os.Stderr, "log: error - invalid file handle type during heartbeat\n")
		return
	}

	// Write with a single retry attempt
	n, err := currentLogFile.Write(hbData)
	if err != nil {
		fmtFprintf(os.Stderr, "log: failed to write heartbeat: %v\n", err)
		l.performDiskCheck(true) // Force disk check on write failure

		// One retry after disk check
		n, err = currentLogFile.Write(hbData)
		if err != nil {
			fmtFprintf(os.Stderr, "log: failed to write heartbeat on retry: %v\n", err)
		} else {
			l.state.CurrentSize.Add(int64(n))
		}
	} else {
		l.state.CurrentSize.Add(int64(n))
	}
}