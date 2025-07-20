// FILE: lixenwraith/log/processor.go
package log

import (
	"os"
	"time"
)

// processLogs is the main log processing loop running in a separate goroutine
func (l *Logger) processLogs(ch <-chan logRecord) {
	l.state.ProcessorExited.Store(false)
	defer l.state.ProcessorExited.Store(true)

	// Set up timers and state variables
	timers := l.setupProcessingTimers()
	defer l.closeProcessingTimers(timers)

	c := l.getConfig()

	// Perform an initial disk check on startup (skip if file output is disabled)
	if !c.DisableFile {
		l.performDiskCheck(true)
	}

	// Send initial heartbeats immediately instead of waiting for first tick
	heartbeatLevel := c.HeartbeatLevel
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
	var lastCheckTime = time.Now()
	var logsSinceLastCheck int64 = 0

	// --- Main Loop ---
	for {
		select {
		case record, ok := <-ch:
			if !ok {
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
					if l.performDiskCheck(false) {
						bytesSinceLastCheck = 0
						logsSinceLastCheck = 0
						lastCheckTime = time.Now()
					}
				}
			}

		case <-timers.flushTicker.C:
			l.handleFlushTick()

		case <-timers.diskCheckTicker.C:
			// Periodic disk check
			if l.performDiskCheck(true) {
				l.adjustDiskCheckInterval(timers, lastCheckTime, logsSinceLastCheck)
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

// processLogRecord handles individual log records, returning bytes written
func (l *Logger) processLogRecord(record logRecord) int64 {
	c := l.getConfig()
	// Check if the record should process this record
	disableFile := c.DisableFile
	if !disableFile && !l.state.DiskStatusOK.Load() {
		l.state.DroppedLogs.Add(1)
		return 0
	}

	// Serialize the log entry once
	format := c.Format
	data := l.serializer.serialize(
		format,
		record.Flags,
		record.TimeStamp,
		record.Level,
		record.Trace,
		record.Args,
	)
	dataLen := int64(len(data))

	// Mirror to stdout if enabled
	enableStdout := c.EnableStdout
	if enableStdout {
		if s := l.state.StdoutWriter.Load(); s != nil {
			if sinkWrapper, ok := s.(*sink); ok && sinkWrapper != nil {
				// Handle split mode
				if c.StdoutTarget == "split" {
					if record.Level >= LevelWarn {
						// Write WARN and ERROR to stderr
						_, _ = os.Stderr.Write(data)
					} else {
						// Write INFO and DEBUG to stdout
						_, _ = sinkWrapper.w.Write(data)
					}
				} else {
					// Write to the configured target (stdout or stderr)
					_, _ = sinkWrapper.w.Write(data)
				}
			}
		}
	}

	// Skip file operations if file output is disabled
	if disableFile {
		l.state.TotalLogsProcessed.Add(1)
		return dataLen // Return data length for adaptive interval calculations
	}

	// File rotation check
	currentFileSize := l.state.CurrentSize.Load()
	estimatedSize := currentFileSize + dataLen

	maxSizeKB := c.MaxSizeKB
	if maxSizeKB > 0 && estimatedSize > maxSizeKB*sizeMultiplier {
		if err := l.rotateLogFile(); err != nil {
			l.internalLog("failed to rotate log file: %v\n", err)
			// Account for the dropped log that triggered the failed rotation
			l.state.DroppedLogs.Add(1)
			return 0
		}
	}

	// Write to file
	cfPtr := l.state.CurrentFile.Load()
	if currentLogFile, isFile := cfPtr.(*os.File); isFile && currentLogFile != nil {
		n, err := currentLogFile.Write(data)
		if err != nil {
			l.internalLog("failed to write to log file: %v\n", err)
			l.state.DroppedLogs.Add(1)
			l.performDiskCheck(true)
			return 0
		} else {
			l.state.CurrentSize.Add(int64(n))
			l.state.TotalLogsProcessed.Add(1)
			return int64(n)
		}
	} else {
		l.state.DroppedLogs.Add(1)
		return 0
	}
}

// handleFlushTick handles the periodic flush timer tick
func (l *Logger) handleFlushTick() {
	c := l.getConfig()
	enableSync := c.EnablePeriodicSync
	if enableSync {
		l.performSync()
	}
}

// handleFlushRequest handles an explicit flush request
func (l *Logger) handleFlushRequest(confirmChan chan struct{}) {
	l.performSync()
	close(confirmChan)
}

// handleRetentionCheck performs file retention check and cleanup
func (l *Logger) handleRetentionCheck() {
	c := l.getConfig()
	retentionPeriodHrs := c.RetentionPeriodHrs
	retentionDur := time.Duration(retentionPeriodHrs * float64(time.Hour))

	if retentionDur > 0 {
		etPtr := l.state.EarliestFileTime.Load()
		if earliest, ok := etPtr.(time.Time); ok && !earliest.IsZero() {
			if time.Since(earliest) > retentionDur {
				if err := l.cleanExpiredLogs(earliest); err == nil {
					l.updateEarliestFileTime()
				} else {
					l.internalLog("failed to clean expired logs: %v\n", err)
				}
			}
		} else if !ok || earliest.IsZero() {
			l.updateEarliestFileTime()
		}
	}
}

// adjustDiskCheckInterval modifies the disk check interval based on logging activity
func (l *Logger) adjustDiskCheckInterval(timers *TimerSet, lastCheckTime time.Time, logsSinceLastCheck int64) {
	c := l.getConfig()
	enableAdaptive := c.EnableAdaptiveInterval
	if !enableAdaptive {
		return
	}

	elapsed := time.Since(lastCheckTime)
	if elapsed < minWaitTime { // Min arbitrary reasonable value
		elapsed = minWaitTime
	}

	logsPerSecond := float64(logsSinceLastCheck) / elapsed.Seconds()
	targetLogsPerSecond := float64(100) // Baseline

	diskCheckIntervalMs := c.DiskCheckIntervalMs
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
	minCheckIntervalMs := c.MinCheckIntervalMs
	maxCheckIntervalMs := c.MaxCheckIntervalMs
	minCheckInterval := time.Duration(minCheckIntervalMs) * time.Millisecond
	maxCheckInterval := time.Duration(maxCheckIntervalMs) * time.Millisecond

	if newInterval < minCheckInterval {
		newInterval = minCheckInterval
	}
	if newInterval > maxCheckInterval {
		newInterval = maxCheckInterval
	}

	timers.diskCheckTicker.Reset(newInterval)
}