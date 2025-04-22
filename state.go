package log

import (
	"sync/atomic"
)

// State encapsulates the runtime state of the logger
type State struct {
	IsInitialized   atomic.Bool
	LoggerDisabled  atomic.Bool
	ShutdownCalled  atomic.Bool
	DiskFullLogged  atomic.Bool
	DiskStatusOK    atomic.Bool
	ProcessorExited atomic.Bool // Tracks if the processor goroutine is running or has exited

	CurrentFile      atomic.Value  // stores *os.File
	CurrentSize      atomic.Int64  // Size of the current log file
	EarliestFileTime atomic.Value  // stores time.Time for retention
	DroppedLogs      atomic.Uint64 // Counter for logs dropped
	LoggedDrops      atomic.Uint64 // Counter for dropped logs message already logged

	ActiveLogChannel atomic.Value // stores chan logRecord
}