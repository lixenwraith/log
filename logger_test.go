package log

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestNewLogger verifies initial state of an unconfigured logger.
func TestNewLogger(t *testing.T) {
	logger := NewLogger()

	isFalse(t, logger.state.IsInitialized.Load(), "IsInitialized")
	isFalse(t, logger.state.LoggerDisabled.Load(), "LoggerDisabled")
	isFalse(t, logger.state.Started.Load(), "Started")
	isTrue(t, logger.state.ProcessorExited.Load(), "ProcessorExited")

	// A default formatter must exist to avoid nil dereference before ApplyConfig
	if logger.formatter.Load() == nil {
		t.Error("formatter not pre-initialized")
	}
	// Start before ApplyConfig must fail
	errContains(t, logger.Start(), "logger not initialized", "Start")
}

// TestApplyConfig verifies initialization and log file creation.
func TestApplyConfig(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	isTrue(t, logger.state.IsInitialized.Load(), "IsInitialized")
	if _, err := os.Stat(filepath.Join(tmpDir, "log.log")); err != nil {
		t.Errorf("active log file missing: %v", err)
	}
}

// TestApplyConfigRejection verifies invalid configs are rejected without mutating state.
func TestApplyConfigRejection(t *testing.T) {
	logger, _ := newTestLogger(t)
	before := *logger.GetConfig()

	errContains(t, logger.ApplyConfig(nil), "cannot be nil", "nil config")

	bad := logger.GetConfig()
	bad.Format = "yaml"
	errContains(t, logger.ApplyConfig(bad), "invalid format", "invalid format")

	mustEqual(t, *logger.GetConfig(), before, "config after rejected applies")
}

// TestApplyConfigString covers key-value overrides, error paths, and rollback.
func TestApplyConfigString(t *testing.T) {
	logger, _ := newTestLogger(t)
	// Dedicated directory target; never point a file-enabled logger at a shared path
	movedDir := filepath.Join(t.TempDir(), "moved")

	tests := []struct {
		name      string
		overrides []string
		wantErr   string
		verify    func(t *testing.T, cfg *Config)
	}{
		{
			name:      "numeric level and directory",
			overrides: []string{"level=-4", "directory=" + movedDir, "format=json"},
			verify: func(t *testing.T, cfg *Config) {
				equal(t, cfg.Level, LevelDebug, "Level")
				equal(t, cfg.Directory, movedDir, "Directory")
				equal(t, cfg.Format, "json", "Format")
			},
		},
		{
			name:      "named level",
			overrides: []string{"level=warn"},
			verify:    func(t *testing.T, cfg *Config) { equal(t, cfg.Level, LevelWarn, "Level") },
		},
		{
			name:      "boolean values",
			overrides: []string{"enable_console=true", "enable_file=true", "show_timestamp=false"},
			verify: func(t *testing.T, cfg *Config) {
				isTrue(t, cfg.EnableConsole, "EnableConsole")
				isTrue(t, cfg.EnableFile, "EnableFile")
				isFalse(t, cfg.ShowTimestamp, "ShowTimestamp")
			},
		},
		{
			name:      "float and policy values",
			overrides: []string{"retention_period_hrs=1.5", "sanitization=txt"},
			verify: func(t *testing.T, cfg *Config) {
				equal(t, cfg.RetentionPeriodHrs, 1.5, "RetentionPeriodHrs")
				equal(t, cfg.Sanitization, PolicyTxt, "Sanitization")
			},
		},
		{name: "missing separator", overrides: []string{"invalid"}, wantErr: "expected key=value"},
		{name: "empty key", overrides: []string{"=value"}, wantErr: "key cannot be empty"},
		{name: "unknown key", overrides: []string{"unknown_key=value"}, wantErr: "unknown configuration key"},
		{name: "bad integer", overrides: []string{"buffer_size=not_a_number"}, wantErr: "invalid integer value"},
		{name: "bad boolean", overrides: []string{"enable_file=yes-please"}, wantErr: "invalid boolean value"},
		{name: "bad level name", overrides: []string{"level=verbose"}, wantErr: "invalid level value"},
		// Field parse succeeds; rejection happens in Validate
		{name: "unvalidated policy", overrides: []string{"sanitization=bogus"}, wantErr: "invalid sanitization policy"},
		{
			name:      "multiple errors combined",
			overrides: []string{"unknown_key=1", "buffer_size=x"},
			wantErr:   "multiple configuration errors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := *logger.GetConfig()
			err := logger.ApplyConfigString(tt.overrides...)

			if tt.wantErr != "" {
				errContains(t, err, tt.wantErr, "ApplyConfigString")
				equal(t, *logger.GetConfig(), before, "config must be unchanged on error")
				return
			}
			mustNoErr(t, err, "ApplyConfigString")
			tt.verify(t, logger.GetConfig())
		})
	}
}

// TestLoggerLoggingLevels checks level-based filtering of emitted records.
func TestLoggerLoggingLevels(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	// Writes are asynchronous; poll until all expected records land
	mustEventually(t, time.Second, "log records written", func() bool {
		c := readLog(t, tmpDir)
		return strings.Contains(c, "info message") &&
			strings.Contains(c, "warn message") &&
			strings.Contains(c, "error message")
	})

	content := readLog(t, tmpDir)
	notContains(t, content, "debug message", "debug below configured level")
}

// TestLoggerTraceDepth verifies trace emission is gated by depth without panicking.
func TestLoggerTraceDepth(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Level = LevelDebug
	cfg.Format = "txt"
	cfg.ShowTimestamp = false
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	logger.Info("no trace here")   // TraceDepth 0 -> no trace field
	logger.DebugTrace(2, "traced") // explicit depth -> trace present
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	mustEventually(t, time.Second, "traced record written", func() bool {
		return strings.Contains(readLog(t, tmpDir), "traced")
	})

	for _, line := range strings.Split(readLog(t, tmpDir), "\n") {
		if strings.Contains(line, "no trace here") && strings.Contains(line, "->") {
			t.Errorf("unexpected trace on zero-depth record: %s", line)
		}
	}
}

// TestLoggerConcurrency exercises concurrent producers against a single processor.
func TestLoggerConcurrency(t *testing.T) {
	logger, _ := newTestLogger(t)

	const goroutines, perGoroutine = 10, 100
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range perGoroutine {
				logger.Info("goroutine", i, "log", j)
			}
		}(i)
	}
	wg.Wait()
	noErr(t, logger.Flush(time.Second), "Flush")

	// Upper bound only: processor-side write/rotation failures increment
	// DroppedLogs without TotalDroppedLogs, so an exact identity is unsafe
	processed := logger.state.TotalLogsProcessed.Load()
	dropped := logger.state.TotalDroppedLogs.Load()
	if total := uint64(goroutines * perGoroutine); processed+dropped > total {
		t.Errorf("counters exceed submitted records: processed=%d dropped=%d total=%d",
			processed, dropped, total)
	}
	if processed == 0 {
		t.Error("no records processed")
	}
}

// TestLoggerConsoleTargets verifies console-only operation for each target.
func TestLoggerConsoleTargets(t *testing.T) {
	for _, target := range []string{"stdout", "stderr", "split"} {
		t.Run(target, func(t *testing.T) {
			logger := NewLogger()
			cfg := DefaultConfig()
			cfg.Directory = t.TempDir()
			cfg.EnableConsole = true
			cfg.EnableFile = false
			cfg.ConsoleTarget = target

			mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")
			mustNoErr(t, logger.Start(), "Start")
			t.Cleanup(func() { _ = logger.Shutdown() })

			// split routes >=WARN to stderr; exercise both branches
			logger.Info("console info")
			logger.Error("console error")
			noErr(t, logger.Flush(time.Second), "Flush")
		})
	}
}

// TestLoggerWrite verifies Write emits raw bytes with no formatting, framing, or sanitization.
func TestLoggerWrite(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	// PolicyTxt would hex-encode control bytes; FlagRaw must bypass it
	cfg := logger.GetConfig()
	cfg.Sanitization = PolicyTxt
	cfg.Format = "txt"
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	logger.Write("raw", "output", 123)
	logger.Write("\x1b[31m")
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	mustEventually(t, time.Second, "raw record written", func() bool {
		return strings.Contains(readLog(t, tmpDir), "\x1b[31m")
	})

	content := readLog(t, tmpDir)
	contains(t, content, "raw output 123", "space-joined raw args")
	notContains(t, content, "<1b>", "sanitizer must be bypassed under FlagRaw")
	if strings.HasSuffix(content, "\n") {
		t.Error("Write must not append a trailing newline")
	}
}
