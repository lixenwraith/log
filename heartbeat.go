// FILE: lixenwraith/log/heartbeat.go
package log

import (
	"fmt"
	"runtime"
	"time"
)

// handleHeartbeat processes a heartbeat timer tick
func (l *Logger) handleHeartbeat() {
	c := l.getConfig()
	heartbeatLevel := c.HeartbeatLevel

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
	processed := l.state.TotalLogsProcessed.Load()
	sequence := l.state.HeartbeatSequence.Add(1)

	startTimeVal := l.state.LoggerStartTime.Load()
	var uptimeHours float64 = 0
	if startTime, ok := startTimeVal.(time.Time); ok && !startTime.IsZero() {
		uptime := time.Since(startTime)
		uptimeHours = uptime.Hours()
	}

	// Get total drops (persistent through logger instance lifecycle)
	totalDropped := l.state.TotalDroppedLogs.Load()

	// Atomically get and reset interval drops
	// NOTE: If PROC heartbeat fails, interval drops are lost and total count tracks such fails
	// Design choice is not to parse the heartbeat log record and restore the count
	droppedInInterval := l.state.DroppedLogs.Swap(0)

	procArgs := []any{
		"type", "proc",
		"sequence", sequence,
		"uptime_hours", fmt.Sprintf("%.2f", uptimeHours),
		"processed_logs", processed,
		"total_dropped_logs", totalDropped,
	}

	// Add interval (since last proc heartbeat) drops if > 0
	if droppedInInterval > 0 {
		procArgs = append(procArgs, "dropped_since_last", droppedInInterval)
	}

	l.writeHeartbeatRecord(LevelProc, procArgs)
}

// logDiskHeartbeat logs disk/file statistics heartbeat
func (l *Logger) logDiskHeartbeat() {
	sequence := l.state.HeartbeatSequence.Load()
	rotations := l.state.TotalRotations.Load()
	deletions := l.state.TotalDeletions.Load()

	c := l.getConfig()
	dir := c.Directory
	ext := c.Extension
	currentSizeMB := float64(l.state.CurrentSize.Load()) / (1024 * 1024) // Current file size
	totalSizeMB := float64(-1.0)                                         // Default error value
	fileCount := -1                                                      // Default error value

	dirSize, err := l.getLogDirSize(dir, ext)
	if err == nil {
		totalSizeMB = float64(dirSize) / (1024 * 1024)
	} else {
		l.internalLog("warning - heartbeat failed to get dir size: %v\n", err)
	}

	count, err := l.getLogFileCount(dir, ext)
	if err == nil {
		fileCount = count
	} else {
		l.internalLog("warning - heartbeat failed to get file count: %v\n", err)
	}

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

	l.writeHeartbeatRecord(LevelDisk, diskArgs)
}

// logSysHeartbeat logs system/runtime statistics heartbeat
func (l *Logger) logSysHeartbeat() {
	sequence := l.state.HeartbeatSequence.Load()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	sysArgs := []any{
		"type", "sys",
		"sequence", sequence,
		"alloc_mb", fmt.Sprintf("%.2f", float64(memStats.Alloc)/(1000*1000)),
		"sys_mb", fmt.Sprintf("%.2f", float64(memStats.Sys)/(1000*1000)),
		"num_gc", memStats.NumGC,
		"num_goroutine", runtime.NumGoroutine(),
	}

	// Write the heartbeat record
	l.writeHeartbeatRecord(LevelSys, sysArgs)
}

// writeHeartbeatRecord creates and sends a heartbeat log record through the main processing channel
func (l *Logger) writeHeartbeatRecord(level int64, args []any) {
	if l.state.LoggerDisabled.Load() || l.state.ShutdownCalled.Load() {
		return
	}

	// Create heartbeat record with appropriate flags
	record := logRecord{
		Flags:     FlagDefault | FlagShowLevel,
		TimeStamp: time.Now(),
		Level:     level,
		Trace:     "",
		Args:      args,
	}

	l.sendLogRecord(record)
}