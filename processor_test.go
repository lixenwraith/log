package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// procRecords parses PROC heartbeat records out of json-formatted content.
// Heartbeat arguments are emitted as a keyed object (FlagKV); the flat
// key/value array is still accepted for records written without the flag.
func procRecords(tb testing.TB, content string) []map[string]any {
	tb.Helper()
	var out []map[string]any
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, `"level":"PROC"`) {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		switch fields := entry["fields"].(type) {
		case map[string]any:
			out = append(out, fields)
		case []any:
			rec := make(map[string]any, len(fields)/2)
			for i := 0; i+1 < len(fields); i += 2 {
				if key, ok := fields[i].(string); ok {
					rec[key] = fields[i+1]
				}
			}
			out = append(out, rec)
		}
	}
	return out
}

// numField extracts a numeric heartbeat field; absent fields yield 0.
func numField(rec map[string]any, key string) float64 {
	v, _ := rec[key].(float64)
	return v
}

// TestLoggerHeartbeat verifies each heartbeat level emits its record type.
func TestLoggerHeartbeat(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Format = "json"
	cfg.HeartbeatLevel = 3
	cfg.HeartbeatIntervalS = 1
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	// The processor emits an initial set on start, ahead of the first tick
	mustEventually(t, 3*time.Second, "heartbeats written", func() bool {
		c := readLog(t, tmpDir)
		return strings.Contains(c, `"level":"PROC"`) &&
			strings.Contains(c, `"level":"DISK"`) &&
			strings.Contains(c, `"level":"SYS"`)
	})

	content := readLog(t, tmpDir)
	contains(t, content, "uptime_hours", "proc payload")
	contains(t, content, "processed_logs", "proc payload")
	contains(t, content, "disk_status_ok", "disk payload")
	contains(t, content, "log_file_count", "disk payload")
	contains(t, content, "num_goroutine", "sys payload")
	contains(t, content, "alloc_mb", "sys payload")
}

// TestHeartbeatDisabled verifies level 0 emits nothing.
func TestHeartbeatDisabled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, out := memoryHeartbeatLogger(t, 0)
		mustNoErr(t, l.Start(), "start")
		synctest.Sleep(2 * time.Second)
		l.Info("marker")
		mustNoErr(t, l.Flush(time.Second), "flush")
		contains(t, out.String(), "marker", "ordinary record")
		notContains(t, out.String(), `"level":"PROC"`, "disabled heartbeat")
		equal(t, l.state.HeartbeatSequence.Load(), uint64(0), "no heartbeat sequence")
		mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	})
}

func memoryHeartbeatLogger(t *testing.T, level int64) (*Logger, *bytes.Buffer) {
	t.Helper()
	l := NewLogger()
	c := DefaultConfig()
	c.Format = "json"
	c.HeartbeatLevel = level
	c.HeartbeatIntervalS = 1
	c.BufferSize = 1
	mustNoErr(t, l.ApplyConfig(c), "apply")
	out := new(bytes.Buffer)
	l.state.StdoutWriter.Store(&sink{w: out})
	return l, out
}

// The processor cannot start until the flood is complete, making overflow exact.
func TestDroppedLogs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l, out := memoryHeartbeatLogger(t, 1)
		release := make(chan struct{})
		l.SetSpawn(func(fn func()) { go func() { <-release; fn() }() })
		mustNoErr(t, l.Start(), "start")
		for i := range 100 {
			l.Info("flood", i)
		}
		equal(t, l.state.TotalDroppedLogs.Load(), uint64(99), "exact overflow")
		close(release)
		synctest.Wait()
		// The initial heartbeat encounters the full queue and adds one more drop.
		synctest.Sleep(time.Second)
		records := procRecords(t, out.String())
		mustEqual(t, len(records), 1, "tick heartbeat")
		equal(t, numField(records[0], "total_dropped_logs"), float64(100), "includes dropped initial heartbeat")
		equal(t, numField(records[0], "processed_logs"), float64(1), "excludes heartbeats")
		mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	})
}

func TestDroppedHeartbeatAccounting(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.Format = "json"
	mustNoErr(t, l.ApplyConfig(c), "apply")
	defer l.Shutdown()
	ch := make(chan logRecord, 1)
	l.state.ActiveLogChannel.Store(ch)
	l.processLogRecord(logRecord{Args: []any{"application record"}})
	l.state.DiskStatusOK.Store(false)
	l.logProcHeartbeat()
	l.processLogRecord(<-ch)
	equal(t, l.state.TotalDroppedLogs.Load(), uint64(1), "dropped heartbeat")
	l.state.DiskStatusOK.Store(true)
	l.logProcHeartbeat()
	l.processLogRecord(<-ch)
	records := procRecords(t, readLog(t, c.Directory))
	mustEqual(t, len(records), 1, "recovery heartbeat")
	equal(t, numField(records[0], "total_dropped_logs"), float64(1), "persistent drops")
	equal(t, numField(records[0], "dropped_since_last"), float64(1), "interval drops")
	equal(t, l.state.TotalLogsProcessed.Load(), uint64(1), "heartbeat excluded from processed count")
}

func TestAdaptiveDiskCheck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := NewLogger()
		c := DefaultConfig()
		c.DiskCheckIntervalMs = 100
		c.MinCheckIntervalMs = 50
		c.MaxCheckIntervalMs = 500
		mustNoErr(t, l.ApplyConfig(c), "apply")
		timers := l.setupProcessingTimers()
		defer l.stopProcessingTimers(timers)
		for range 8 {
			l.adjustDiskCheckInterval(timers, time.Now().Add(-time.Second), 0)
		}
		equal(t, timers.diskInterval, 500*time.Millisecond, "repeated low load reaches maximum")
		for range 16 {
			l.adjustDiskCheckInterval(timers, time.Now().Add(-time.Second), 1000)
		}
		equal(t, timers.diskInterval, 50*time.Millisecond, "repeated high load reaches minimum")
	})
}

// TestFlushBarrier verifies records enqueued before Flush are written before it returns.
func TestFlushBarrier(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	const records = 50
	for i := range records {
		logger.Info("barrier", i)
	}
	mustNoErr(t, logger.Flush(2*time.Second), "Flush")

	// No polling: the barrier must hold on the first read
	content := readLog(t, tmpDir)
	for i := range records {
		contains(t, content, "barrier "+itoa(i), "record enqueued before Flush")
	}
}

// itoa avoids a strconv import for small non-negative values.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
