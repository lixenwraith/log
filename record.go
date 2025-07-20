// FILE: lixenwraith/log/record.go
package log

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// getCurrentLogChannel safely retrieves the current log channel
func (l *Logger) getCurrentLogChannel() chan logRecord {
	chVal := l.state.ActiveLogChannel.Load()
	return chVal.(chan logRecord)
}

// getFlags from config
func (l *Logger) getFlags() int64 {
	var flags int64 = 0
	cfg := l.getConfig()

	if cfg.ShowLevel {
		flags |= FlagShowLevel
	}
	if cfg.ShowTimestamp {
		flags |= FlagShowTimestamp
	}
	return flags
}

// sendLogRecord handles safe sending to the active channel
func (l *Logger) sendLogRecord(record logRecord) {
	defer func() {
		if r := recover(); r != nil { // Catch panic on send to closed channel
			l.handleFailedSend(record)
		}
	}()

	if l.state.ShutdownCalled.Load() || l.state.LoggerDisabled.Load() {
		// Process drops even if logger is disabled or shutting down
		l.handleFailedSend(record)
		return
	}

	ch := l.getCurrentLogChannel()

	// Non-blocking send
	select {
	case ch <- record:
		// Success: record sent, channel was not full, check if log drops need to be reported
		if record.unreportedDrops == 0 {
			// Get number of dropped logs and reset the counter to zero
			droppedCount := l.state.DroppedLogs.Swap(0)

			if droppedCount > 0 {
				// Dropped logs report
				dropRecord := logRecord{
					Flags:           FlagDefault,
					TimeStamp:       time.Now(),
					Level:           LevelError,
					Args:            []any{"Logs were dropped", "dropped_count", droppedCount},
					unreportedDrops: droppedCount, // Carry the count for recovery
				}
				// No success check is required, count is restored if it fails
				l.sendLogRecord(dropRecord)
			}
		}
	default:
		l.handleFailedSend(record)
	}
}

// handleFailedSend restores or increments drop counter
func (l *Logger) handleFailedSend(record logRecord) {
	// For regular record, add 1 to dropped log count
	// For drop report, restore the count
	amountToAdd := uint64(1)
	if record.unreportedDrops > 0 {
		amountToAdd = record.unreportedDrops
	}
	l.state.DroppedLogs.Add(amountToAdd)
}

// log handles the core logging logic
func (l *Logger) log(flags int64, level int64, depth int64, args ...any) {
	if !l.state.IsInitialized.Load() {
		return
	}

	cfg := l.getConfig()
	if level < cfg.Level {
		return
	}

	var trace string
	if depth > 0 {
		const skipTrace = 3 // log.Info -> log -> getTrace (Adjust if call stack changes)
		trace = getTrace(depth, skipTrace)
	}

	record := logRecord{
		Flags:           flags,
		TimeStamp:       time.Now(),
		Level:           level,
		Trace:           trace,
		Args:            args,
		unreportedDrops: 0, // 0 for regular logs
	}
	l.sendLogRecord(record)
}

// internalLog handles writing internal logger diagnostics to stderr, if enabled.
func (l *Logger) internalLog(format string, args ...any) {
	// Check if internal error reporting is enabled
	cfg := l.getConfig()
	if !cfg.InternalErrorsToStderr {
		return
	}

	// Ensure consistent "log: " prefix
	if !strings.HasPrefix(format, "log: ") {
		format = "log: " + format
	}

	// Write to stderr
	fmt.Fprintf(os.Stderr, format, args...)
}