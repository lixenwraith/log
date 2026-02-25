package log

import (
	"sync"
	"sync/atomic"
)

// State encapsulates the runtime state of the logger
type State struct {
	// General state
	IsInitialized   atomic.Bool // Tracks successful initialization, not start of log processor
	LoggerDisabled  atomic.Bool // Tracks logger stop due to issues (e.g. disk full)
	ShutdownCalled  atomic.Bool // Tracks if Shutdown() has been called, a terminal state
	DiskFullLogged  atomic.Bool // Tracks if a disk full error has been logged to prevent log spam
	DiskStatusOK    atomic.Bool // Tracks if disk space and size limits are currently met
	Started         atomic.Bool // Tracks calls to Start() and Stop()
	ProcessorExited atomic.Bool // Tracks if the processor goroutine is running or has exited

	// Flushing state
	flushRequestChan chan chan struct{} // Channel to request a flush
	flushMutex       sync.Mutex         // Protect concurrent Flush calls

	// Outputs
	CurrentFile  atomic.Value // stores *os.File
	StdoutWriter atomic.Value // stores io.Writer (os.Stdout, os.Stderr, or io.Discard)

	// File State
	CurrentSize      atomic.Int64 // Size of the current log file
	EarliestFileTime atomic.Value // stores time.Time for retention

	// Log state
	ActiveLogChannel atomic.Value  // stores chan logRecord
	DroppedLogs      atomic.Uint64 // Counter for logs dropped since last heartbeat
	TotalDroppedLogs atomic.Uint64 // Counter for total logs dropped since logger start

	// Heartbeat statistics
	HeartbeatSequence  atomic.Uint64 // Counter for heartbeat sequence numbers
	LoggerStartTime    atomic.Value  // Stores time.Time for uptime calculation
	TotalLogsProcessed atomic.Uint64 // Counter for non-heartbeat logs successfully processed
	TotalRotations     atomic.Uint64 // Counter for successful log rotations
	TotalDeletions     atomic.Uint64 // Counter for successful log deletions (cleanup/retention)
}