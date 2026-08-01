package log

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// // procRecords parses PROC heartbeat records out of json-formatted content.
// // Heartbeat arguments are emitted as a flat key/value array.
// func procRecords(tb testing.TB, content string) []map[string]any {
// 	tb.Helper()
// 	var out []map[string]any
// 	for _, line := range strings.Split(content, "\n") {
// 		if !strings.Contains(line, `"level":"PROC"`) {
// 			continue
// 		}
// 		var entry map[string]any
// 		if json.Unmarshal([]byte(line), &entry) != nil {
// 			continue
// 		}
// 		fields, ok := entry["fields"].([]any)
// 		if !ok {
// 			continue
// 		}
// 		rec := make(map[string]any, len(fields)/2)
// 		for i := 0; i+1 < len(fields); i += 2 {
// 			if key, ok := fields[i].(string); ok {
// 				rec[key] = fields[i+1]
// 			}
// 		}
// 		out = append(out, rec)
// 	}
// 	return out
// }

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
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Format = "json"
	cfg.HeartbeatLevel = 0
	cfg.HeartbeatIntervalS = 1
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	logger.Info("marker")
	mustNoErr(t, logger.Flush(time.Second), "Flush")
	time.Sleep(1200 * time.Millisecond) // span at least one interval

	content := readLog(t, tmpDir)
	contains(t, content, "marker", "regular record")
	notContains(t, content, `"level":"PROC"`, "proc heartbeat")
	equal(t, logger.state.HeartbeatSequence.Load(), uint64(0), "HeartbeatSequence")
}

// TestDroppedLogs verifies buffer overflow is counted and reported by the heartbeat.
func TestDroppedLogs(t *testing.T) {
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = t.TempDir()
	cfg.EnableConsole = false
	cfg.EnableFile = true
	cfg.Format = "json"
	cfg.BufferSize = 1 // guarantees drops under flood
	cfg.FlushIntervalMs = 10
	cfg.HeartbeatLevel = 1
	cfg.HeartbeatIntervalS = 1

	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")
	mustNoErr(t, logger.Start(), "Start")
	t.Cleanup(func() { _ = logger.Shutdown() })

	for i := range 100 {
		logger.Info("flood", i)
	}

	dropped := logger.state.TotalDroppedLogs.Load()
	if dropped == 0 {
		t.Fatal("flood produced no drops")
	}

	// The interval counter is reported only when non-zero, so wait for the
	// tick-driven heartbeat that follows the flood
	mustEventually(t, 5*time.Second, "heartbeat reporting interval drops", func() bool {
		for _, rec := range procRecords(t, readLog(t, cfg.Directory)) {
			if _, ok := rec["dropped_since_last"]; ok {
				return true
			}
		}
		return false
	})

	records := procRecords(t, readLog(t, cfg.Directory))
	last := records[len(records)-1]
	if got := numField(last, "total_dropped_logs"); got < float64(dropped) {
		t.Errorf("total_dropped_logs %v below observed drops %d", got, dropped)
	}
}

// TestDroppedHeartbeatAccounting verifies a heartbeat discarded by the processor
// during a disk failure is still reflected in the total drop count reported by
// the next successful heartbeat.
func TestDroppedHeartbeatAccounting(t *testing.T) {
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = t.TempDir()
	cfg.EnableConsole = false
	cfg.EnableFile = true
	cfg.Format = "json"
	cfg.BufferSize = 10
	cfg.HeartbeatLevel = 1
	cfg.HeartbeatIntervalS = 1
	cfg.InternalErrorsToStderr = false // internal logs would add drops

	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")
	mustNoErr(t, logger.Start(), "Start")
	t.Cleanup(func() { _ = logger.Shutdown() })

	// Drops during the flood are nondeterministic; capture the actual count
	for i := range int(cfg.BufferSize) + 50 {
		logger.Info("flood", i)
	}
	floodDrops := logger.state.TotalDroppedLogs.Load()
	if floodDrops == 0 {
		t.Fatal("flood produced no drops")
	}

	// Let the first tick-driven heartbeat consume the interval counter
	mustEventually(t, 3*time.Second, "first tick heartbeat", func() bool {
		return logger.state.HeartbeatSequence.Load() >= 2
	})

	// Force the disk-unavailable state; the processor discards every record
	diskFull := logger.GetConfig()
	diskFull.MinDiskFreeKB = 1 << 40
	mustNoErr(t, logger.ApplyConfig(diskFull), "ApplyConfig disk full")
	isFalse(t, logger.performDiskCheck(true), "performDiskCheck under disk full")
	isFalse(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK")

	// Hold the failure until a heartbeat has been produced and discarded
	seq := logger.state.HeartbeatSequence.Load()
	mustEventually(t, 3*time.Second, "heartbeat produced while disk full", func() bool {
		return logger.state.HeartbeatSequence.Load() > seq
	})
	droppedWithDiskFull := logger.state.TotalDroppedLogs.Load()
	if droppedWithDiskFull <= floodDrops {
		t.Fatalf("processor did not drop during disk failure: %d", droppedWithDiskFull)
	}

	// Restore and wait for a heartbeat that reaches the file
	diskOK := logger.GetConfig()
	diskOK.MinDiskFreeKB = 0
	mustNoErr(t, logger.ApplyConfig(diskOK), "ApplyConfig disk ok")
	isTrue(t, logger.performDiskCheck(true), "performDiskCheck after recovery")
	isTrue(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK after recovery")

	seq = logger.state.HeartbeatSequence.Load()
	mustEventually(t, 4*time.Second, "heartbeat written after recovery", func() bool {
		if logger.state.HeartbeatSequence.Load() <= seq {
			return false
		}
		records := procRecords(t, readLog(t, cfg.Directory))
		if len(records) == 0 {
			return false
		}
		return numField(records[len(records)-1], "sequence") > float64(seq)
	})

	records := procRecords(t, readLog(t, cfg.Directory))
	last := records[len(records)-1]

	// The dropped heartbeat is unrecoverable in the interval counter but must
	// remain visible in the monotonic total
	if got := numField(last, "total_dropped_logs"); got < float64(droppedWithDiskFull) {
		t.Errorf("total_dropped_logs %v does not cover drops observed during failure %d",
			got, droppedWithDiskFull)
	}
	if got := numField(last, "processed_logs"); got == 0 {
		t.Error("processed_logs must be non-zero after recovery")
	}
}

// TestAdaptiveDiskCheck exercises interval adjustment under varying log rates.
func TestAdaptiveDiskCheck(t *testing.T) {
	logger, _ := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.EnableAdaptiveInterval = true
	cfg.DiskCheckIntervalMs = 100
	cfg.MinCheckIntervalMs = 50
	cfg.MaxCheckIntervalMs = 500
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	// Low rate, then burst: both adjustment branches
	for i := range 10 {
		logger.Info("adaptive test", i)
		time.Sleep(10 * time.Millisecond)
	}
	for i := range 100 {
		logger.Info("burst", i)
	}
	mustNoErr(t, logger.Flush(2*time.Second), "Flush")

	isTrue(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK")
	if logger.state.TotalLogsProcessed.Load() == 0 {
		t.Error("no records processed")
	}
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
