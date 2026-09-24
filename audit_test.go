package log

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigOwnershipAndRollback(t *testing.T) {
	l, dir := newTestLogger(t)
	cfg := l.GetConfig()
	cfg.Format = "json"
	mustNoErr(t, l.ApplyConfig(cfg), "apply snapshot")
	cfg.Format = "raw"
	equal(t, l.GetConfig().Format, "json", "caller config is not retained")
	old := *l.GetConfig()
	blocker := filepath.Join(dir, "blocker")
	mustNoErr(t, os.WriteFile(blocker, nil, 0600), "create blocker")
	mustNoErr(t, os.Mkdir(filepath.Join(dir, "directory"), 0700), "create directory blocker")
	for _, path := range []string{filepath.Join(blocker, "child"), dir} {
		bad := l.GetConfig()
		bad.Directory = path
		bad.Name = "directory"
		bad.Extension = ""
		bad.Format = "txt"
		bad.Level = LevelError
		mustErr(t, l.ApplyConfig(bad), "failed reconfiguration")
		equal(t, *l.GetConfig(), old, "configuration rolled back")
		isTrue(t, l.Enabled(LevelInfo), "failed config did not close emit gate")
	}
	l.Info("survives")
	mustNoErr(t, l.Flush(time.Second), "flush old configuration")
	contains(t, readLog(t, dir), `"fields":["survives"]`, "formatter rolled back")
}

func TestConfigBounds(t *testing.T) {
	for name, mutate := range map[string]func(*Config){
		"name traversal":      func(c *Config) { c.Name = "../outside" },
		"name dot":            func(c *Config) { c.Name = ".." },
		"extension traversal": func(c *Config) { c.Extension = "../log" },
		"NUL":                 func(c *Config) { c.Name = "bad\x00name" },
		"buffer":              func(c *Config) { c.BufferSize = math.MaxInt64 },
		"size":                func(c *Config) { c.MaxSizeKB = math.MaxInt64 },
		"flush":               func(c *Config) { c.FlushIntervalMs = math.MaxInt64 },
		"heartbeat":           func(c *Config) { c.HeartbeatIntervalS = math.MaxInt64 },
		"retention nan":       func(c *Config) { c.RetentionPeriodHrs = math.NaN() },
		"retention infinity":  func(c *Config) { c.RetentionCheckMins = math.Inf(1) },
		"retention overflow":  func(c *Config) { c.RetentionPeriodHrs = 1e30 },
		"retention underflow": func(c *Config) { c.RetentionCheckMins = 1e-30 },
	} {
		t.Run(name, func(t *testing.T) { c := DefaultConfig(); mutate(c); mustErr(t, c.Validate(), "validate") })
	}
	var nilConfig *Config
	mustErr(t, nilConfig.Validate(), "nil config")
	for _, b := range []*Builder{NewBuilder().MaxSizeMB(math.MaxInt64), NewBuilder().MaxTotalSizeMB(math.MaxInt64), NewBuilder().MinDiskFreeMB(math.MaxInt64)} {
		_, err := b.Build()
		mustErr(t, err, "MB overflow")
	}
	l := NewLogger()
	isFalse(t, l.Enabled(math.MaxInt64), "closed level gate")
}

func TestConcurrentOverridesMerge(t *testing.T) {
	l, _ := newTestLogger(t)
	overrides := []string{"trace_depth=2", "level=debug", "format=json", "show_level=false", "show_timestamp=false", "sanitization=txt"}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for _, override := range overrides {
		wg.Go(func() { <-start; noErr(t, l.ApplyConfigString(override), override) })
	}
	close(start)
	wg.Wait()
	c := l.GetConfig()
	equal(t, c.TraceDepth, int64(2), "depth")
	equal(t, c.Level, LevelDebug, "level")
	equal(t, c.Format, "json", "format")
	isFalse(t, c.ShowLevel, "show level")
	isFalse(t, c.ShowTimestamp, "show timestamp")
	equal(t, c.Sanitization, PolicyTxt, "sanitization")
}

func TestStartDoesNotReplaceProcessor(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	mustNoErr(t, l.ApplyConfig(c), "apply")
	var launches atomic.Int64
	l.SetSpawn(func(fn func()) { launches.Add(1); go fn() })
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() { noErr(t, l.Start(), "concurrent start") })
	}
	wg.Wait()
	equal(t, launches.Load(), int64(1), "only one processor")
	mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	mustNoErr(t, l.ApplyConfig(c), "reset after shutdown")
	mustNoErr(t, l.Start(), "start reset logger")
	isTrue(t, l.Enabled(LevelInfo), "reset reopens gate")
	mustNoErr(t, l.Shutdown(time.Second), "shutdown reset logger")
}

type blockedWriter struct {
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(p), nil
}

func TestTimedOutShutdownKeepsProcessorResources(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.Directory = t.TempDir()
	c.EnableFile = true
	c.EnableConsole = true
	mustNoErr(t, l.ApplyConfig(c), "apply")
	w := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	l.state.StdoutWriter.Store(&sink{w: w})
	mustNoErr(t, l.Start(), "start")
	l.Info("accepted before timeout")
	select {
	case <-w.entered:
	case <-time.After(time.Second):
		t.Fatal("processor never reached writer")
	}
	// Always release the writer even when an assertion fails.
	defer func() { close(w.release); noErr(t, l.Shutdown(time.Second), "retry shutdown") }()
	errContains(t, l.Shutdown(time.Millisecond), "timeout", "shutdown timeout")
	f := l.state.CurrentFile.Load().(*os.File)
	_, err := f.Stat()
	mustNoErr(t, err, "file still owned by draining processor")
	mustErr(t, l.Start(), "restart blocked")
	mustErr(t, l.ApplyConfig(c), "resource replacement blocked")
}

func TestAsyncArgumentSnapshot(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.Format = "json"
	mustNoErr(t, l.ApplyConfig(c), "apply")
	var run func()
	l.SetSpawn(func(fn func()) { run = fn })
	mustNoErr(t, l.Start(), "start deferred processor")
	args := []any{"original"}
	l.Info(args...)
	args[0] = "mutated"
	fields := map[string]any{"value": "original"}
	l.LogStructured(LevelInfo, "message", fields)
	fields["value"] = "mutated"
	go run()
	mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	data := readLog(t, c.Directory)
	notContains(t, data, "mutated", "snapshotted arguments and map")
	equal(t, strings.Count(data, "original"), 2, "original values")
}

func TestFlushReportsSyncError(t *testing.T) {
	l, _ := newTestLogger(t)
	mustNoErr(t, l.Flush(time.Second), "initial flush")
	file := l.state.CurrentFile.Load().(*os.File)
	mustNoErr(t, file.Close(), "inject closed file")
	err := l.Flush(time.Second)
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("expected sync error, got %v", err)
	}
}

func TestConsoleOnlyRetentionIsInert(t *testing.T) {
	c := DefaultConfig()
	c.EnableConsole = false
	c.Directory = t.TempDir()
	c.RetentionPeriodHrs = 1
	name := filepath.Join(c.Directory, "old.log")
	mustNoErr(t, os.WriteFile(name, []byte("keep"), 0600), "write")
	old := time.Now().Add(-2 * time.Hour)
	mustNoErr(t, os.Chtimes(name, old, old), "set age")
	l := NewLogger()
	mustNoErr(t, l.ApplyConfig(c), "apply")
	l.state.EarliestFileTime.Store(old)
	l.handleRetentionCheck()
	mustNoErr(t, l.cleanExpiredLogs(old), "explicit cleanup")
	data, err := os.ReadFile(name)
	mustNoErr(t, err, "file survived")
	equal(t, string(data), "keep", "contents")
	timers := l.setupProcessingTimers()
	defer l.stopProcessingTimers(timers)
	if timers.retentionChan != nil {
		t.Fatal("console-only retention timer enabled")
	}
}

func TestArchiveCollisionBeyondOneThousand(t *testing.T) {
	l, dir := newTestLogger(t)
	stamp := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	base := "log_" + stamp.Format("060102_150405")
	mustNoErr(t, os.Mkdir(filepath.Join(dir, base+".log"), 0700), "directory collision")
	for i := 1; i <= 1000; i++ {
		mustNoErr(t, os.WriteFile(filepath.Join(dir, base+"_"+itoa(i)+".log"), nil, 0600), "archive")
	}
	equal(t, l.generateArchiveLogFileName(stamp), base+"_1001.log", "no capped collision reuse")
}

func TestExtensionlessAccountingAndCleanup(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	c.Extension = ""
	mustNoErr(t, l.ApplyConfig(c), "apply")
	defer l.Shutdown()
	for _, name := range []string{"old", "other.txt"} {
		mustNoErr(t, os.WriteFile(filepath.Join(c.Directory, name), []byte("123"), 0600), "write")
	}
	size, err := l.getLogDirSize(c.Directory, "")
	mustNoErr(t, err, "size")
	equal(t, size, int64(3), "extensionless size")
	count, err := l.getLogFileCount(c.Directory, "")
	mustNoErr(t, err, "count")
	equal(t, count, 2, "extensionless count")
	mustNoErr(t, l.cleanOldLogs(0), "zero demand does not remove archives")
	mustNoErr(t, l.cleanOldLogs(3), "cleanup")
	_, err = os.Stat(filepath.Join(c.Directory, "other.txt"))
	mustNoErr(t, err, "other extension preserved")
}

func TestTextSanitizesMetadata(t *testing.T) {
	var out bytes.Buffer
	l := NewLogger()
	c := DefaultConfig()
	c.Format = "txt"
	c.Sanitization = PolicyTxt
	c.TimestampFormat = "bad\n\x1b"
	c.EnableConsole = true
	mustNoErr(t, l.ApplyConfig(c), "apply")
	l.SetContextKeys("key\n\x1b")
	l.state.StdoutWriter.Store(&sink{w: &out})
	mustNoErr(t, l.Start(), "start")
	l.LogContext(Context{Tag: "value"}, FlagKV, LevelInfo, 0, "key\n\x1b", "value")
	mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	equal(t, bytes.Count(out.Bytes(), []byte{'\n'}), 1, "one record line")
	notContains(t, out.String(), "\x1b", "no terminal escape")
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestConsoleWriteFailureAccounting(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	mustNoErr(t, l.ApplyConfig(c), "apply")
	l.state.StdoutWriter.Store(&sink{w: shortWriter{}})
	mustNoErr(t, l.Start(), "start")
	l.Info("short write")
	mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	equal(t, l.state.TotalDroppedLogs.Load(), uint64(1), "console drop")
	equal(t, l.state.DroppedLogs.Load(), uint64(1), "interval drop")
	equal(t, l.state.TotalLogsProcessed.Load(), uint64(0), "not processed successfully")
}

func TestDrainFlushIsBounded(t *testing.T) {
	// Refill after every write, reproducing a queue which never becomes empty.
	// A snapshot drain must still complete after the original pending count.
	l := NewLogger()
	c := DefaultConfig()
	c.Format = "raw"
	mustNoErr(t, l.ApplyConfig(c), "apply")
	ch := make(chan logRecord, 4)
	for range cap(ch) {
		ch <- logRecord{Args: []any{"x"}}
	}
	l.state.StdoutWriter.Store(&sink{w: writerFunc(func(p []byte) (int, error) { ch <- logRecord{Args: []any{"later"}}; return len(p), nil })})
	done := make(chan struct{})
	go func() { l.drain(ch); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("drain starved on refilled queue")
	}
	equal(t, l.state.TotalLogsProcessed.Load(), uint64(4), "bounded work")
	equal(t, len(ch), 4, "later records left for next iteration")
}

type writerFunc func([]byte) (int, error)

func (fn writerFunc) Write(p []byte) (int, error) { return fn(p) }

func TestReconfigureDrainsOldFileAndFormat(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.Directory = t.TempDir()
	c.EnableFile = true
	c.EnableConsole = true
	c.Format = "raw"
	c.MaxSizeKB = 1
	c.FlushIntervalMs = 1000
	mustNoErr(t, l.ApplyConfig(c), "apply")
	w := &blockedWriter{entered: make(chan struct{}), release: make(chan struct{})}
	l.state.StdoutWriter.Store(&sink{w: w})
	var diagnostics []string
	l.SetErrorHandler(func(msg string) { diagnostics = append(diagnostics, msg) })
	mustNoErr(t, l.Start(), "start")
	payload := strings.Repeat("x", 1500)
	l.Info(payload)
	<-w.entered
	l.Info("old-format-tail")
	updated := l.GetConfig()
	updated.EnableFile = false
	updated.EnableConsole = false
	updated.Format = "json"
	done := make(chan error, 1)
	go func() { done <- l.ApplyConfig(updated) }()
	mustEventually(t, time.Second, "stop began", func() bool { return !l.state.Started.Load() })
	close(w.release)
	mustNoErr(t, <-done, "reconfigure")
	mustNoErr(t, l.Shutdown(time.Second), "shutdown")
	data := readAllLogs(t, c.Directory)
	contains(t, data, payload, "original payload")
	contains(t, data, "old-format-tail", "queued record used old format")
	equal(t, len(data), len(payload)+len("old-format-tail"), "exact drained bytes")
	equal(t, len(diagnostics), 0, "closed current handle after draining rotation")
}
