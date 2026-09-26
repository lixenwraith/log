package log

import (
	"io"
	"time"

	"github.com/lixenwraith/log/formatter"
)

// Context carries caller-stamped values emitted with a record
type Context = formatter.Context

// ContextSlots is the number of correlation values a Context carries
const ContextSlots = formatter.ContextSlots

// logRecord represents a single log entry
type logRecord struct {
	heartbeat bool
	TimeStamp time.Time
	Trace     string
	Args      []any
	Ctx       Context
	Flags     int64
	Level     int64
}

// TimerSet holds all timers used in processLogs
type TimerSet struct {
	flushTicker     *time.Ticker
	diskCheckTicker *time.Ticker
	retentionTicker *time.Ticker
	heartbeatTicker *time.Ticker
	retentionChan   <-chan time.Time
	heartbeatChan   <-chan time.Time
	diskInterval    time.Duration
}

// sink is a wrapper around an io.Writer, atomic value type change workaround
type sink struct {
	w io.Writer
}

// flushRequest belongs to one processor generation; the buffered reply permits
// completion even when the caller has already timed out.
type flushRequest struct {
	done chan error
}
