package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lixenwraith/log"
)

func eq[T comparable](tb testing.TB, got, want T, ctx string) {
	tb.Helper()
	if got != want {
		tb.Errorf("%s: got %#v, want %#v", ctx, got, want)
	}
}

func mustNoErr(tb testing.TB, err error, ctx string) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("%s: unexpected error: %v", ctx, err)
	}
}

func errContains(tb testing.TB, err error, sub, ctx string) {
	tb.Helper()
	switch {
	case err == nil:
		tb.Errorf("%s: expected error containing %q, got nil", ctx, sub)
	case !strings.Contains(err.Error(), sub):
		tb.Errorf("%s: error %q does not contain %q", ctx, err, sub)
	}
}

// newTestBuilder returns a builder bound to a started json-format logger.
func newTestBuilder(tb testing.TB) (*Builder, *log.Logger, string) {
	tb.Helper()
	tmpDir := tb.TempDir()

	appLogger, err := log.NewBuilder().
		Directory(tmpDir).
		Format("json").
		LevelString("debug").
		EnableConsole(false).
		EnableFile(true).
		Build()
	mustNoErr(tb, err, "Build")
	mustNoErr(tb, appLogger.Start(), "Start")
	tb.Cleanup(func() { _ = appLogger.Shutdown() })

	return NewBuilder().WithLogger(appLogger), appLogger, tmpDir
}

// readLogLines polls the active log file until it holds at least want records.
func readLogLines(tb testing.TB, dir string, want int) []string {
	tb.Helper()
	path := filepath.Join(dir, "log.log")
	deadline := time.Now().Add(2 * time.Second)

	for {
		if data, err := os.ReadFile(path); err == nil {
			trimmed := strings.TrimRight(string(data), "\n")
			if trimmed != "" {
				lines := strings.Split(trimmed, "\n")
				if len(lines) >= want {
					return lines
				}
			}
		}
		if time.Now().After(deadline) {
			tb.Fatalf("did not read %d log lines from %s", want, dir)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// recordOf parses one json record into its level and flat fields array.
func recordOf(tb testing.TB, line string) (string, []any) {
	tb.Helper()
	var entry map[string]any
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		tb.Fatalf("parse log line %q: %v", line, err)
	}
	level, _ := entry["level"].(string)
	fields, ok := entry["fields"].([]any)
	if !ok {
		tb.Fatalf("record has no fields array: %s", line)
	}
	return level, fields
}

// checkFields compares the leading elements of a fields array.
func checkFields(tb testing.TB, fields []any, want []any, ctx string) {
	tb.Helper()
	if len(fields) < len(want) {
		tb.Fatalf("%s: got %d fields, want at least %d: %v", ctx, len(fields), len(want), fields)
	}
	for i, w := range want {
		if fields[i] != w {
			tb.Errorf("%s: field %d = %#v, want %#v", ctx, i, fields[i], w)
		}
	}
}

// TestBuilderSources verifies logger resolution from an instance, a config, or defaults.
func TestBuilderSources(t *testing.T) {
	t.Run("existing logger", func(t *testing.T) {
		builder, logger, _ := newTestBuilder(t)

		adapter, err := builder.BuildGnet()
		mustNoErr(t, err, "BuildGnet")
		if adapter == nil {
			t.Fatal("BuildGnet returned nil")
		}
		if adapter.logger != logger {
			t.Error("adapter must reuse the provided logger")
		}
	})

	t.Run("config creates and caches a logger", func(t *testing.T) {
		cfg := log.DefaultConfig()
		cfg.Directory = t.TempDir()
		cfg.EnableConsole = false

		builder := NewBuilder().WithConfig(cfg)
		adapter, err := builder.BuildFastHTTP()
		mustNoErr(t, err, "BuildFastHTTP")
		if adapter == nil {
			t.Fatal("BuildFastHTTP returned nil")
		}

		logger, err := builder.GetLogger()
		mustNoErr(t, err, "GetLogger")
		t.Cleanup(func() { _ = logger.Shutdown() })

		// Subsequent builds reuse the cached instance
		second, err := builder.GetLogger()
		mustNoErr(t, err, "GetLogger second call")
		if second != logger {
			t.Error("builder must cache the created logger")
		}
		eq(t, logger.GetConfig().Directory, cfg.Directory, "applied directory")
	})

	t.Run("nil config falls back to defaults", func(t *testing.T) {
		logger, err := NewBuilder().WithConfig(nil).GetLogger()
		mustNoErr(t, err, "GetLogger")
		t.Cleanup(func() { _ = logger.Shutdown() })
		eq(t, logger.GetConfig().Format, log.DefaultConfig().Format, "default format")
	})

	t.Run("nil logger is rejected", func(t *testing.T) {
		builder := NewBuilder().WithLogger(nil)
		_, err := builder.BuildGnet()
		errContains(t, err, "provided logger cannot be nil", "BuildGnet")

		// The deferred error persists across build calls
		_, err = builder.BuildFiber()
		errContains(t, err, "provided logger cannot be nil", "BuildFiber")
	})

	t.Run("invalid config propagates", func(t *testing.T) {
		cfg := log.DefaultConfig()
		cfg.Directory = t.TempDir()
		cfg.Format = "yaml"

		_, err := NewBuilder().WithConfig(cfg).BuildGnet()
		errContains(t, err, "invalid format", "BuildGnet")
	})

	t.Run("all adapters build from one logger", func(t *testing.T) {
		builder, logger, _ := newTestBuilder(t)

		gnetAdapter, err := builder.BuildGnet()
		mustNoErr(t, err, "BuildGnet")
		structuredAdapter, err := builder.BuildStructuredGnet()
		mustNoErr(t, err, "BuildStructuredGnet")
		fasthttpAdapter, err := builder.BuildFastHTTP()
		mustNoErr(t, err, "BuildFastHTTP")
		fiberAdapter, err := builder.BuildFiber()
		mustNoErr(t, err, "BuildFiber")

		if gnetAdapter.logger != logger || structuredAdapter.logger != logger ||
			fasthttpAdapter.logger != logger || fiberAdapter.logger != logger {
			t.Error("every adapter must share the provided logger")
		}
	})
}

// TestGnetAdapter verifies level mapping and the fatal handler override.
func TestGnetAdapter(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	var fatalCalled bool
	adapter, err := builder.BuildGnet(WithFatalHandler(func(msg string) {
		fatalCalled = true
	}))
	mustNoErr(t, err, "BuildGnet")

	adapter.Debugf("gnet debug id=%d", 1)
	adapter.Infof("gnet info id=%d", 2)
	adapter.Warnf("gnet warn id=%d", 3)
	adapter.Errorf("gnet error id=%d", 4)
	adapter.Fatalf("gnet fatal id=%d", 5)

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 5)
	eq(t, len(lines), 5, "record count")

	expected := []struct{ level, msg string }{
		{"DEBUG", "gnet debug id=1"},
		{"INFO", "gnet info id=2"},
		{"WARN", "gnet warn id=3"},
		{"ERROR", "gnet error id=4"},
		{"ERROR", "gnet fatal id=5"},
	}

	for i, line := range lines {
		level, fields := recordOf(t, line)
		eq(t, level, expected[i].level, "level")
		checkFields(t, fields, []any{"msg", expected[i].msg, "source", "gnet"}, expected[i].msg)
	}

	// The fatal record carries a marker beyond the common prefix
	_, fatalFields := recordOf(t, lines[4])
	checkFields(t, fatalFields, []any{"msg", "gnet fatal id=5", "source", "gnet", "fatal", true}, "fatal marker")
	if !fatalCalled {
		t.Error("custom fatal handler was not invoked")
	}
}

// TestStructuredGnetAdapter verifies key/value extraction from printf formats.
func TestStructuredGnetAdapter(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	adapter, err := builder.BuildStructuredGnet()
	mustNoErr(t, err, "BuildStructuredGnet")

	adapter.Infof("request served status=%d client_ip=%s", 200, "127.0.0.1")
	// No key=verb pattern: the whole message collapses into a msg field
	adapter.Warnf("plain message %d", 42)

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 2)
	eq(t, len(lines), 2, "record count")

	level, fields := recordOf(t, lines[0])
	eq(t, level, "INFO", "level")
	// JSON numbers decode as float64
	checkFields(t, fields, []any{
		"msg", "request served",
		"status", 200.0,
		"client_ip", "127.0.0.1",
		"source", "gnet",
	}, "extracted fields")

	level, fields = recordOf(t, lines[1])
	eq(t, level, "WARN", "level")
	checkFields(t, fields, []any{"msg", "plain message 42", "source", "gnet"}, "fallback")
}

// TestFastHTTPAdapter verifies content-based level detection.
func TestFastHTTPAdapter(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	adapter, err := builder.BuildFastHTTP()
	mustNoErr(t, err, "BuildFastHTTP")

	messages := []string{
		"this is some informational message",
		"a debug message for the developers",
		"warning: something might be wrong",
		"an error occurred while processing",
	}
	for _, msg := range messages {
		adapter.Printf("%s", msg)
	}

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, len(messages))
	eq(t, len(lines), len(messages), "record count")

	levels := []string{"INFO", "DEBUG", "WARN", "ERROR"}
	for i, line := range lines {
		level, fields := recordOf(t, line)
		eq(t, level, levels[i], "detected level")
		checkFields(t, fields, []any{"msg", messages[i], "source", "fasthttp"}, messages[i])
	}
}

// TestDetectLogLevel covers the keyword table directly.
func TestDetectLogLevel(t *testing.T) {
	tests := []struct {
		msg  string
		want int64
	}{
		{"connection failed", log.LevelError},
		{"FATAL condition", log.LevelError},
		{"panic recovered", log.LevelError},
		{"Error: bad input", log.LevelError},
		{"deprecated call site", log.LevelWarn},
		{"WARNING: retrying", log.LevelWarn},
		{"trace enabled", log.LevelDebug},
		{"debug output", log.LevelDebug},
		{"server started", log.LevelInfo},
		{"", log.LevelInfo},
		// Error keywords are matched before warning keywords
		{"warning: request failed", log.LevelError},
	}

	for _, tt := range tests {
		if got := DetectLogLevel(tt.msg); got != tt.want {
			t.Errorf("DetectLogLevel(%q) = %d, want %d", tt.msg, got, tt.want)
		}
	}
}

// TestFastHTTPOptions verifies the default level and detector overrides.
// Note: LevelInfo is zero, which the adapter treats as "not detected", so a
// detector cannot force Info over a non-Info default.
func TestFastHTTPDefaultLevel(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	adapter, err := builder.BuildFastHTTP(
		WithDefaultLevel(log.LevelWarn),
		WithLevelDetector(func(msg string) int64 {
			if strings.Contains(msg, "boom") {
				return log.LevelError
			}
			return log.LevelInfo // indistinguishable from "no detection"
		}),
	)
	mustNoErr(t, err, "BuildFastHTTP")

	adapter.Printf("undetected message")
	adapter.Printf("boom happened")

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 2)

	level, _ := recordOf(t, lines[0])
	eq(t, level, "WARN", "default level applies when detection yields Info")
	level, _ = recordOf(t, lines[1])
	eq(t, level, "ERROR", "detector overrides the default")
}

// TestFiberAdapter verifies the FormatLogger surface and both handler overrides.
func TestFiberAdapter(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	var fatalCalled, panicCalled bool
	adapter, err := builder.BuildFiber(
		WithFiberFatalHandler(func(msg string) { fatalCalled = true }),
		WithFiberPanicHandler(func(msg string) { panicCalled = true }),
	)
	mustNoErr(t, err, "BuildFiber")

	adapter.Tracef("fiber trace id=%d", 1)
	adapter.Debugf("fiber debug id=%d", 2)
	adapter.Infof("fiber info id=%d", 3)
	adapter.Warnf("fiber warn id=%d", 4)
	adapter.Errorf("fiber error id=%d", 5)
	adapter.Fatalf("fiber fatal id=%d", 6)
	adapter.Panicf("fiber panic id=%d", 7)

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 7)
	eq(t, len(lines), 7, "record count")

	expected := []struct{ level, msg string }{
		{"DEBUG", "fiber trace id=1"},
		{"DEBUG", "fiber debug id=2"},
		{"INFO", "fiber info id=3"},
		{"WARN", "fiber warn id=4"},
		{"ERROR", "fiber error id=5"},
		{"ERROR", "fiber fatal id=6"},
		{"ERROR", "fiber panic id=7"},
	}

	for i, line := range lines {
		level, fields := recordOf(t, line)
		eq(t, level, expected[i].level, "level")
		checkFields(t, fields, []any{"msg", expected[i].msg, "source", "fiber"}, expected[i].msg)
	}

	// Trace maps onto debug and is distinguished by an extra field
	_, traceFields := recordOf(t, lines[0])
	checkFields(t, traceFields, []any{"msg", "fiber trace id=1", "source", "fiber", "level", "trace"}, "trace marker")

	if !fatalCalled {
		t.Error("custom fatal handler was not invoked")
	}
	if !panicCalled {
		t.Error("custom panic handler was not invoked")
	}
}

// TestFiberAdapterPlain verifies the Logger surface built from fmt.Sprint.
func TestFiberAdapterPlain(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	adapter, err := builder.BuildFiber()
	mustNoErr(t, err, "BuildFiber")

	adapter.Info("plain ", "info")
	adapter.Error("plain ", "error")

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 2)

	level, fields := recordOf(t, lines[0])
	eq(t, level, "INFO", "level")
	checkFields(t, fields, []any{"msg", "plain info", "source", "fiber"}, "Info")

	level, fields = recordOf(t, lines[1])
	eq(t, level, "ERROR", "level")
	checkFields(t, fields, []any{"msg", "plain error", "source", "fiber"}, "Error")
}

// TestFiberAdapterStructured verifies the WithLogger surface.
func TestFiberAdapterStructured(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	adapter, err := builder.BuildFiber()
	mustNoErr(t, err, "BuildFiber")

	adapter.Infow("request served", "status", 200, "client_ip", "127.0.0.1", "method", "GET")
	adapter.Debugw("query executed", "duration_ms", 42, "query", "SELECT")
	adapter.Warnw("slow response", "duration_ms", 900)

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 3)
	eq(t, len(lines), 3, "record count")

	// Adapter-owned fields precede caller-supplied pairs
	level, fields := recordOf(t, lines[0])
	eq(t, level, "INFO", "level")
	checkFields(t, fields, []any{
		"msg", "request served", "source", "fiber",
		"status", 200.0, "client_ip", "127.0.0.1", "method", "GET",
	}, "Infow")

	level, fields = recordOf(t, lines[1])
	eq(t, level, "DEBUG", "level")
	checkFields(t, fields, []any{
		"msg", "query executed", "source", "fiber",
		"duration_ms", 42.0, "query", "SELECT",
	}, "Debugw")

	level, fields = recordOf(t, lines[2])
	eq(t, level, "WARN", "level")
	checkFields(t, fields, []any{
		"msg", "slow response", "source", "fiber", "duration_ms", 900.0,
	}, "Warnw")
}

// TestFiberAdapterStructuredFatal verifies Fatalw ordering and handler dispatch.
func TestFiberAdapterStructuredFatal(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	var fatalCalled bool
	adapter, err := builder.BuildFiber(
		WithFiberFatalHandler(func(msg string) { fatalCalled = true }),
	)
	mustNoErr(t, err, "BuildFiber")

	adapter.Fatalw("shutting down", "code", 3)

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 1)

	level, fields := recordOf(t, lines[0])
	eq(t, level, "ERROR", "level")
	checkFields(t, fields, []any{
		"msg", "shutting down", "source", "fiber", "fatal", true, "code", 3.0,
	}, "Fatalw")
	if !fatalCalled {
		t.Error("custom fatal handler was not invoked")
	}
}

// TestFiberAdapterWriter verifies the io.Writer implementation.
func TestFiberAdapterWriter(t *testing.T) {
	builder, logger, tmpDir := newTestBuilder(t)

	adapter, err := builder.BuildFiber()
	mustNoErr(t, err, "BuildFiber")

	payload := []byte("writer output\n")
	n, err := adapter.Write(payload)
	mustNoErr(t, err, "Write")
	eq(t, n, len(payload), "byte count includes the trimmed newline")

	mustNoErr(t, logger.Flush(time.Second), "Flush")
	lines := readLogLines(t, tmpDir, 1)

	level, fields := recordOf(t, lines[0])
	eq(t, level, "INFO", "level")
	checkFields(t, fields, []any{"msg", "writer output", "source", "fiber"}, "Write")
}

