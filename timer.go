// FILE: lixenwraith/log/processor.go
package log

import "time"

// setupProcessingTimers creates and configures all necessary timers for the processor
func (l *Logger) setupProcessingTimers() *TimerSet {
	timers := &TimerSet{}

	c := l.getConfig()

	// Set up flush timer
	flushInterval := c.FlushIntervalMs
	if flushInterval <= 0 {
		flushInterval = DefaultConfig().FlushIntervalMs
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
	c := l.getConfig()
	retentionPeriodHrs := c.RetentionPeriodHrs
	retentionCheckMins := c.RetentionCheckMins
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
	c := l.getConfig()
	diskCheckIntervalMs := c.DiskCheckIntervalMs
	if diskCheckIntervalMs <= 0 {
		diskCheckIntervalMs = 5000
	}
	currentDiskCheckInterval := time.Duration(diskCheckIntervalMs) * time.Millisecond

	// Ensure initial interval respects bounds
	minCheckIntervalMs := c.MinCheckIntervalMs
	maxCheckIntervalMs := c.MaxCheckIntervalMs
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

// setupHeartbeatTimer configures the heartbeat timer if enabled
func (l *Logger) setupHeartbeatTimer(timers *TimerSet) <-chan time.Time {
	c := l.getConfig()
	heartbeatLevel := c.HeartbeatLevel
	if heartbeatLevel > 0 {
		intervalS := c.HeartbeatIntervalS
		// Make sure interval is positive
		if intervalS <= 0 {
			intervalS = DefaultConfig().HeartbeatIntervalS
		}
		timers.heartbeatTicker = time.NewTicker(time.Duration(intervalS) * time.Second)
		return timers.heartbeatTicker.C
	}
	return nil
}