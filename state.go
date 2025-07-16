// FILE: state.go
package log

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// State encapsulates the runtime state of the logger
type State struct {
	IsInitialized   atomic.Bool
	LoggerDisabled  atomic.Bool
	ShutdownCalled  atomic.Bool
	DiskFullLogged  atomic.Bool
	DiskStatusOK    atomic.Bool
	ProcessorExited atomic.Bool // Tracks if the processor goroutine is running or has exited

	flushRequestChan chan chan struct{} // Channel to request a flush
	flushMutex       sync.Mutex         // Protect concurrent Flush calls

	CurrentFile      atomic.Value  // stores *os.File
	CurrentSize      atomic.Int64  // Size of the current log file
	EarliestFileTime atomic.Value  // stores time.Time for retention
	DroppedLogs      atomic.Uint64 // Counter for logs dropped

	ActiveLogChannel atomic.Value // stores chan logRecord
	StdoutWriter     atomic.Value // stores io.Writer (os.Stdout, os.Stderr, or io.Discard)

	// Heartbeat statistics
	HeartbeatSequence  atomic.Uint64 // Counter for heartbeat sequence numbers
	LoggerStartTime    atomic.Value  // Stores time.Time for uptime calculation
	TotalLogsProcessed atomic.Uint64 // Counter for non-heartbeat logs successfully processed
	TotalRotations     atomic.Uint64 // Counter for successful log rotations
	TotalDeletions     atomic.Uint64 // Counter for successful log deletions (cleanup/retention)
}

// sink is a wrapper around an io.Writer, atomic value type change workaround
type sink struct {
	w io.Writer
}

// Shutdown gracefully closes the logger, attempting to flush pending records
// If no timeout is provided, uses a default of 2x flush interval
func (l *Logger) Shutdown(timeout ...time.Duration) error {

	if !l.state.ShutdownCalled.CompareAndSwap(false, true) {
		return nil
	}

	l.state.LoggerDisabled.Store(true)

	if !l.state.IsInitialized.Load() {
		l.state.ShutdownCalled.Store(false)
		l.state.LoggerDisabled.Store(false)
		l.state.ProcessorExited.Store(true)
		return nil
	}

	l.initMu.Lock()
	ch := l.getCurrentLogChannel()
	closedChan := make(chan logRecord)
	close(closedChan)
	l.state.ActiveLogChannel.Store(closedChan)
	if ch != closedChan {
		close(ch)
	}
	l.initMu.Unlock()

	c := l.getConfig()
	var effectiveTimeout time.Duration
	if len(timeout) > 0 {
		effectiveTimeout = timeout[0]
	} else {
		flushIntervalMs := c.FlushIntervalMs
		// Default to 2x flush interval
		effectiveTimeout = 2 * time.Duration(flushIntervalMs) * time.Millisecond
	}

	deadline := time.Now().Add(effectiveTimeout)
	pollInterval := minWaitTime // Reasonable check period
	processorCleanlyExited := false
	for time.Now().Before(deadline) {
		if l.state.ProcessorExited.Load() {
			processorCleanlyExited = true
			break
		}
		time.Sleep(pollInterval)
	}

	l.state.IsInitialized.Store(false)

	var finalErr error
	cfPtr := l.state.CurrentFile.Load()
	if cfPtr != nil {
		if currentLogFile, ok := cfPtr.(*os.File); ok && currentLogFile != nil {
			if err := currentLogFile.Sync(); err != nil {
				syncErr := fmtErrorf("failed to sync log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, syncErr)
			}
			if err := currentLogFile.Close(); err != nil {
				closeErr := fmtErrorf("failed to close log file '%s' during shutdown: %w", currentLogFile.Name(), err)
				finalErr = combineErrors(finalErr, closeErr)
			}
			l.state.CurrentFile.Store((*os.File)(nil))
		}
	}

	if !processorCleanlyExited {
		timeoutErr := fmtErrorf("logger processor did not exit within timeout (%v)", effectiveTimeout)
		finalErr = combineErrors(finalErr, timeoutErr)
	}

	return finalErr
}

// Flush explicitly triggers a sync of the current log file buffer to disk and waits for completion or timeout.
func (l *Logger) Flush(timeout time.Duration) error {
	l.state.flushMutex.Lock()
	defer l.state.flushMutex.Unlock()

	if !l.state.IsInitialized.Load() || l.state.ShutdownCalled.Load() {
		return fmtErrorf("logger not initialized or already shut down")
	}

	// Create a channel to wait for confirmation from the processor
	confirmChan := make(chan struct{})

	// Send the request with the confirmation channel
	select {
	case l.state.flushRequestChan <- confirmChan:
		// Request sent
	case <-time.After(minWaitTime): // Short timeout to prevent blocking if processor is stuck
		return fmtErrorf("failed to send flush request to processor (possible deadlock or high load)")
	}

	select {
	case <-confirmChan:
		return nil
	case <-time.After(timeout):
		return fmtErrorf("timeout waiting for flush confirmation (%v)", timeout)
	}
}