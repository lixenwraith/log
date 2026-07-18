package log

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// TestDefaultConfig verifies default values and copy independence.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	equal(t, cfg.Level, LevelInfo, "Level")
	equal(t, cfg.Name, "log", "Name")
	equal(t, cfg.Extension, "log", "Extension")
	equal(t, cfg.Directory, "./log", "Directory")
	equal(t, cfg.Format, "raw", "Format")
	equal(t, cfg.Sanitization, PolicyRaw, "Sanitization")
	equal(t, cfg.ConsoleTarget, "stderr", "ConsoleTarget")
	equal(t, cfg.TimestampFormat, time.RFC3339Nano, "TimestampFormat")
	equal(t, cfg.BufferSize, int64(1024), "BufferSize")
	isTrue(t, cfg.ShowTimestamp, "ShowTimestamp")
	isTrue(t, cfg.ShowLevel, "ShowLevel")
	isTrue(t, cfg.EnableConsole, "EnableConsole")
	isFalse(t, cfg.EnableFile, "EnableFile")

	noErr(t, cfg.Validate(), "default config must validate")

	// Each call must yield an independent copy of the package-level default
	other := DefaultConfig()
	if cfg == other {
		t.Error("DefaultConfig returned a shared pointer")
	}
	cfg.Level = LevelError
	equal(t, other.Level, LevelInfo, "second copy must be unaffected")
}

// TestConfigClone verifies full-value copy and bidirectional independence.
func TestConfigClone(t *testing.T) {
	src := DefaultConfig()
	src.Level = LevelDebug
	src.Directory = "/custom/path"
	src.RetentionPeriodHrs = 12.5

	dst := src.Clone()
	mustEqual(t, *dst, *src, "clone must equal source")

	src.Level = LevelError
	equal(t, dst.Level, LevelDebug, "clone unaffected by source mutation")

	dst.Name = "renamed"
	equal(t, src.Name, "log", "source unaffected by clone mutation")
}

// TestConfigValidate covers each validation branch.
func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*Config)
		wantError string
	}{
		{"valid config", func(c *Config) {}, ""},
		{"empty name", func(c *Config) { c.Name = "" }, "log name cannot be empty"},
		{"whitespace name", func(c *Config) { c.Name = "   " }, "log name cannot be empty"},
		{"invalid format", func(c *Config) { c.Format = "invalid" }, "invalid format"},
		{"invalid sanitization", func(c *Config) { c.Sanitization = "bogus" }, "invalid sanitization policy"},
		{"extension with dot", func(c *Config) { c.Extension = ".log" }, "extension should not start with dot"},
		{"empty timestamp format", func(c *Config) { c.TimestampFormat = " " }, "timestamp_format cannot be empty"},
		{"invalid console target", func(c *Config) { c.ConsoleTarget = "invalid" }, "invalid console_target"},
		{"zero buffer size", func(c *Config) { c.BufferSize = 0 }, "buffer_size must be positive"},
		{"negative buffer size", func(c *Config) { c.BufferSize = -1 }, "buffer_size must be positive"},
		{"negative max size", func(c *Config) { c.MaxSizeKB = -1 }, "size limits cannot be negative"},
		{"negative total size", func(c *Config) { c.MaxTotalSizeKB = -1 }, "size limits cannot be negative"},
		{"negative min disk free", func(c *Config) { c.MinDiskFreeKB = -1 }, "size limits cannot be negative"},
		{"zero flush interval", func(c *Config) { c.FlushIntervalMs = 0 }, "interval settings must be positive"},
		{"zero disk check interval", func(c *Config) { c.DiskCheckIntervalMs = 0 }, "interval settings must be positive"},
		{"negative trace depth", func(c *Config) { c.TraceDepth = -1 }, "trace_depth must be between 0 and 10"},
		{"excessive trace depth", func(c *Config) { c.TraceDepth = 11 }, "trace_depth must be between 0 and 10"},
		{"boundary trace depth", func(c *Config) { c.TraceDepth = 10 }, ""},
		{"negative retention", func(c *Config) { c.RetentionPeriodHrs = -1 }, "retention settings cannot be negative"},
		{"invalid heartbeat level", func(c *Config) { c.HeartbeatLevel = 4 }, "heartbeat_level must be between 0 and 3"},
		{
			name:      "heartbeat enabled without interval",
			modify:    func(c *Config) { c.HeartbeatLevel = 1; c.HeartbeatIntervalS = 0 },
			wantError: "heartbeat_interval_s must be positive",
		},
		{
			name:      "min greater than max check interval",
			modify:    func(c *Config) { c.MinCheckIntervalMs = 1000; c.MaxCheckIntervalMs = 500 },
			wantError: "min_check_interval_ms",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()

			if tt.wantError == "" {
				noErr(t, err, "Validate")
				return
			}
			errContains(t, err, tt.wantError, "Validate")
		})
	}
}

// TestConfigRequiresRestart verifies which field changes force a processor restart.
func TestConfigRequiresRestart(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*Config)
		want   bool
	}{
		{"no change", func(c *Config) {}, false},
		{"level", func(c *Config) { c.Level = LevelError }, false},
		{"format", func(c *Config) { c.Format = "json" }, false},
		{"sanitization", func(c *Config) { c.Sanitization = PolicyTxt }, false},
		{"trace depth", func(c *Config) { c.TraceDepth = 3 }, false},
		{"console target", func(c *Config) { c.ConsoleTarget = "stdout" }, false},
		{"buffer size", func(c *Config) { c.BufferSize = 2048 }, true},
		{"enable file", func(c *Config) { c.EnableFile = !c.EnableFile }, true},
		{"directory", func(c *Config) { c.Directory = "/other" }, true},
		{"name", func(c *Config) { c.Name = "other" }, true},
		{"extension", func(c *Config) { c.Extension = "txt" }, true},
		{"flush interval", func(c *Config) { c.FlushIntervalMs = 500 }, true},
		{"heartbeat level", func(c *Config) { c.HeartbeatLevel = 2 }, true},
		{"retention period", func(c *Config) { c.RetentionPeriodHrs = 4 }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldCfg := DefaultConfig()
			newCfg := oldCfg.Clone()
			tt.modify(newCfg)
			equal(t, configRequiresRestart(oldCfg, newCfg), tt.want, "configRequiresRestart")
		})
	}
}

// TestCombineConfigErrors verifies aggregation and prefix deduplication.
func TestCombineConfigErrors(t *testing.T) {
	if err := combineConfigErrors(nil); err != nil {
		t.Errorf("empty slice: got %v, want nil", err)
	}

	single := fmtErrorf("only one")
	mustEqual(t, combineConfigErrors([]error{single}), single, "single error passthrough")

	err := combineConfigErrors([]error{fmtErrorf("first"), fmtErrorf("second")})
	mustErr(t, err, "combineConfigErrors")
	msg := err.Error()
	contains(t, msg, "multiple configuration errors", "header")
	contains(t, msg, "1. first", "first entry")
	contains(t, msg, "2. second", "second entry")
	// Per-error "log: " prefixes must be stripped, leaving only the header prefix
	equal(t, strings.Count(msg, "log: "), 1, "prefix occurrences")
}

// TestConcurrentApplyConfig verifies reconfiguration under concurrent load.
func TestConcurrentApplyConfig(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := logger.GetConfig()
			if id%2 == 0 {
				cfg.Level, cfg.Format = LevelDebug, "json"
			} else {
				cfg.Level, cfg.Format = LevelInfo, "txt"
			}
			cfg.TraceDepth = int64(id % 5)

			// Non-fatal only: Fatal from a non-test goroutine is undefined behavior
			noErr(t, logger.ApplyConfig(cfg), "concurrent ApplyConfig")
			logger.Info("config test", id)
		}(i)
	}
	wg.Wait()

	logger.Info("after concurrent config")
	noErr(t, logger.Flush(time.Second), "Flush")

	mustEventually(t, time.Second, "post-reconfiguration record written", func() bool {
		return strings.Contains(readLog(t, tmpDir), "after concurrent config")
	})
}

