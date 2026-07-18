package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Assertion helpers replacing testify.
// must* variants abort via Fatal and are restricted to the test goroutine.
// Non-fatal variants are safe to call from spawned goroutines.

func equal[T comparable](tb testing.TB, got, want T, ctx string) bool {
	tb.Helper()
	if got != want {
		tb.Errorf("%s: got %#v, want %#v", ctx, got, want)
		return false
	}
	return true
}

func mustEqual[T comparable](tb testing.TB, got, want T, ctx string) {
	tb.Helper()
	if got != want {
		tb.Fatalf("%s: got %#v, want %#v", ctx, got, want)
	}
}

func isTrue(tb testing.TB, cond bool, ctx string) bool {
	tb.Helper()
	if !cond {
		tb.Errorf("%s: expected true", ctx)
		return false
	}
	return true
}

func isFalse(tb testing.TB, cond bool, ctx string) bool {
	tb.Helper()
	if cond {
		tb.Errorf("%s: expected false", ctx)
		return false
	}
	return true
}

func noErr(tb testing.TB, err error, ctx string) {
	tb.Helper()
	if err != nil {
		tb.Errorf("%s: unexpected error: %v", ctx, err)
	}
}

func mustNoErr(tb testing.TB, err error, ctx string) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("%s: unexpected error: %v", ctx, err)
	}
}

// errContains requires a non-nil error whose message contains sub.
func errContains(tb testing.TB, err error, sub, ctx string) {
	tb.Helper()
	switch {
	case err == nil:
		tb.Errorf("%s: expected error containing %q, got nil", ctx, sub)
	case !strings.Contains(err.Error(), sub):
		tb.Errorf("%s: error %q does not contain %q", ctx, err, sub)
	}
}

func mustErr(tb testing.TB, err error, ctx string) {
	tb.Helper()
	if err == nil {
		tb.Fatalf("%s: expected error, got nil", ctx)
	}
}

func contains(tb testing.TB, haystack, needle, ctx string) {
	tb.Helper()
	if !strings.Contains(haystack, needle) {
		tb.Errorf("%s: %q not found in:\n%s", ctx, needle, haystack)
	}
}

func notContains(tb testing.TB, haystack, needle, ctx string) {
	tb.Helper()
	if strings.Contains(haystack, needle) {
		tb.Errorf("%s: %q unexpectedly present in:\n%s", ctx, needle, haystack)
	}
}

// mustEventually polls cond until true or timeout. Replaces sleep-and-check loops
// against the asynchronous processor.
func mustEventually(tb testing.TB, timeout time.Duration, ctx string, cond func() bool) {
	tb.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("%s: condition not met within %v", ctx, timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newTestLogger returns a started file-backed logger in a per-test temp directory.
// Shutdown is registered as cleanup, ordered before temp dir removal.
func newTestLogger(tb testing.TB) (*Logger, string) {
	tb.Helper()
	dir := tb.TempDir()

	logger := NewLogger()
	cfg := DefaultConfig()
	cfg.EnableConsole = false
	cfg.EnableFile = true
	cfg.Directory = dir
	cfg.BufferSize = 1000
	cfg.FlushIntervalMs = 10

	mustNoErr(tb, logger.ApplyConfig(cfg), "ApplyConfig")
	mustNoErr(tb, logger.Start(), "Start")
	tb.Cleanup(func() { _ = logger.Shutdown() })

	return logger, dir
}

// readLog returns the contents of the active log file.
func readLog(tb testing.TB, dir string) string {
	tb.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "log.log"))
	if err != nil {
		tb.Fatalf("read log file: %v", err)
	}
	return string(data)
}

// readAllLogs concatenates every *.log file in dir. Required wherever rotation
// may split output across files. Directory order, not chronological; use only
// for substring assertions.
func readAllLogs(tb testing.TB, dir string) string {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatalf("read dir %s: %v", dir, err)
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			tb.Fatalf("read %s: %v", e.Name(), err)
		}
		sb.Write(data)
	}
	return sb.String()
}

// countLogFiles returns the number of *.log entries in dir.
func countLogFiles(tb testing.TB, dir string) int {
	tb.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		tb.Fatalf("read dir %s: %v", dir, err)
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".log") {
			n++
		}
	}
	return n
}
