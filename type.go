// FILE: lixenwraith/log/type.go
package log

import (
	"io"
	"time"
)

// logRecord represents a single log entry
type logRecord struct {
	Flags     int64
	TimeStamp time.Time
	Level     int64
	Trace     string
	Args      []any
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

// sink is a wrapper around an io.Writer, atomic value type change workaround
type sink struct {
	w io.Writer
}