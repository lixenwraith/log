// FILE: lixenwraith/log/state.go
package log

import (
	"sync"
	"sync/atomic"
)

// State encapsulates the runtime state of the logger
type State struct {
	// General state
	IsInitialized   atomic.Bool
	LoggerDisabled  atomic.Bool
	ShutdownCalled  atomic.Bool
	DiskFullLogged  atomic.Bool
	DiskStatusOK    atomic.Bool
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
	DroppedLogs      atomic.Uint64 // Counter for logs dropped

	// Heartbeat statistics
	HeartbeatSequence  atomic.Uint64 // Counter for heartbeat sequence numbers
	LoggerStartTime    atomic.Value  // Stores time.Time for uptime calculation
	TotalLogsProcessed atomic.Uint64 // Counter for non-heartbeat logs successfully processed
	TotalRotations     atomic.Uint64 // Counter for successful log rotations
	TotalDeletions     atomic.Uint64 // Counter for successful log deletions (cleanup/retention)
}