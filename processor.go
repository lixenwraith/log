package log

import (
	"io"
	"os"
	"time"

	"github.com/lixenwraith/log/formatter"
)

// processLogs is the main log processing loop running in a separate goroutine.
// Exits on stop, draining buffered records first. No panic recovery: a fault
// here is fatal by design and is surfaced by the host's spawner.
func (l *Logger) processLogs(ch <-chan logRecord, stop <-chan struct{}, done chan<- struct{}, flush <-chan flushRequest) {
	defer func() {
		l.state.ProcessorExited.Store(true)
		close(done)
	}()

	// Set up timers and state variables
	timers := l.setupProcessingTimers()
	defer l.stopProcessingTimers(timers)

	c := l.getConfig()

	// Perform an initial disk check on startup (skip if file output is disabled)
	if c.EnableFile {
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
		case <-stop:
			l.drain(ch)
			l.processorErr = l.performSync()
			return

		case record := <-ch:
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

		case request := <-flush:
			// Barrier: drain queued records before sync
			l.handleFlushRequest(ch, request)

		case <-timers.retentionChan:
			l.handleRetentionCheck()

		case <-timers.heartbeatChan:
			l.handleHeartbeat()
		}
	}
}

// drain processes every buffered record without blocking
func (l *Logger) drain(ch <-chan logRecord) {
	// A snapshot bounds the flush work even while producers keep the queue full.
	// Stop has already joined producers, so this also drains its entire queue.
	for range len(ch) {
		l.processLogRecord(<-ch)
	}
}

func (l *Logger) handleFlushRequest(ch <-chan logRecord, request flushRequest) {
	l.drain(ch)
	request.done <- l.performSync()
}

// processLogRecord handles individual log records and returns bytes written
func (l *Logger) processLogRecord(record logRecord) int64 {
	c := l.getConfig()
	enableFile := c.EnableFile
	if enableFile && !l.state.DiskStatusOK.Load() {
		// Simple increment of both counters
		l.handleFailedSend()
		return 0
	}

	// Atomically load formatter instance
	formatterPtr := l.formatter.Load()
	if formatterPtr == nil {
		// Defensive: Should never happen after initialization
		return 0
	}
	f := formatterPtr.(*formatter.Formatter)

	// Format the log entry using atomically-loaded formatter
	formattedData := f.FormatCtx(
		record.Ctx,
		record.Flags,
		record.TimeStamp,
		record.Level,
		record.Trace,
		record.Args,
	)
	formattedDataLen := int64(len(formattedData))

	var consoleErr error
	if c.EnableConsole {
		if target, _ := l.state.StdoutWriter.Load().(*sink); target != nil {
			writer := target.w
			if c.ConsoleTarget == "split" && record.Level >= LevelWarn {
				writer = os.Stderr
			}
			n, err := writer.Write(formattedData)
			if err == nil && n != len(formattedData) {
				err = io.ErrShortWrite
			}
			consoleErr = err
			if err != nil {
				l.internalLog("failed to write to console: %v\n", err)
			}
		}
	}

	// Skip file operations if file output is disabled
	if !enableFile {
		if consoleErr != nil {
			l.handleFailedSend()
			return 0
		}
		if !record.heartbeat {
			l.state.TotalLogsProcessed.Add(1)
		}
		return formattedDataLen // Return data length for adaptive interval calculations
	}

	// File rotation check
	currentFileSize := l.state.CurrentSize.Load()
	estimatedSize := currentFileSize + formattedDataLen

	maxSizeKB := c.MaxSizeKB
	if maxSizeKB > 0 && estimatedSize > maxSizeKB*sizeMultiplier {
		if err := l.rotateLogFile(); err != nil {
			l.internalLog("failed to rotate log file: %v\n", err)
			// Account for the dropped log that triggered the failed rotation
			l.handleFailedSend()
			return 0
		}
	}

	// Write to file
	cfPtr := l.state.CurrentFile.Load()
	if currentLogFile, isFile := cfPtr.(*os.File); isFile && currentLogFile != nil {
		n, err := currentLogFile.Write(formattedData)
		if err != nil {
			l.internalLog("failed to write to log file: %v\n", err)
			l.handleFailedSend()
			l.state.CurrentSize.Add(int64(n))
			l.performDiskCheck(true)
			return 0
		} else {
			l.state.CurrentSize.Add(int64(n))
			if consoleErr != nil {
				l.handleFailedSend()
			} else if !record.heartbeat {
				l.state.TotalLogsProcessed.Add(1)
			}
			return int64(n)
		}
	} else {
		l.handleFailedSend()
		return 0
	}
}

// handleFlushTick handles the periodic flush timer tick
func (l *Logger) handleFlushTick() {
	c := l.getConfig()
	enableSync := c.EnablePeriodicSync
	if enableSync {
		if err := l.performSync(); err != nil {
			l.internalLog("failed to sync log file: %v\n", err)
		}
	}
}

// handleRetentionCheck performs file retention check and cleanup
func (l *Logger) handleRetentionCheck() {
	c := l.getConfig()
	retentionPeriodHrs := c.RetentionPeriodHrs
	retentionDur := time.Duration(retentionPeriodHrs * float64(time.Hour))

	if c.EnableFile && retentionDur > 0 {
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

	elapsed := max(time.Since(lastCheckTime), minWaitTime) // Min arbitrary reasonable value

	logsPerSecond := float64(logsSinceLastCheck) / elapsed.Seconds()
	targetLogsPerSecond := float64(100) // Baseline

	currentDiskCheckInterval := timers.diskInterval
	if currentDiskCheckInterval == 0 {
		currentDiskCheckInterval = time.Duration(c.DiskCheckIntervalMs) * time.Millisecond
	}

	// Calculate the new interval
	var newInterval time.Duration
	if logsPerSecond < targetLogsPerSecond/2 { // Load low -> increase interval
		scaled := float64(currentDiskCheckInterval) * adaptiveIntervalFactor
		if scaled >= float64(time.Duration(c.MaxCheckIntervalMs)*time.Millisecond) {
			newInterval = time.Duration(c.MaxCheckIntervalMs) * time.Millisecond
		} else {
			newInterval = time.Duration(scaled)
		}
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

	timers.diskInterval = newInterval
	timers.diskCheckTicker.Reset(newInterval)
}
