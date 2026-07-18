package log

import (
	"os"
	"path/filepath"
	"testing"
)

// TestBuilderBuild verifies configuration flows from the fluent API into the logger.
func TestBuilderBuild(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewBuilder().
		Directory(tmpDir).
		LevelString("debug").
		Format("json").
		BufferSize(2048).
		EnableConsole(false).
		EnableFile(true).
		MaxSizeMB(10).
		HeartbeatLevel(2).
		Build()

	mustNoErr(t, err, "Build")
	if logger == nil {
		t.Fatal("Build returned a nil logger without an error")
	}
	t.Cleanup(func() { _ = logger.Shutdown() })

	cfg := logger.GetConfig()
	equal(t, cfg.Directory, tmpDir, "Directory")
	equal(t, cfg.Level, LevelDebug, "Level")
	equal(t, cfg.Format, "json", "Format")
	equal(t, cfg.BufferSize, int64(2048), "BufferSize")
	isFalse(t, cfg.EnableConsole, "EnableConsole")
	isTrue(t, cfg.EnableFile, "EnableFile")
	equal(t, cfg.MaxSizeKB, int64(10*sizeMultiplier), "MaxSizeKB")
	equal(t, cfg.HeartbeatLevel, int64(2), "HeartbeatLevel")

	// Build applies but does not start the processor
	isTrue(t, logger.state.IsInitialized.Load(), "IsInitialized")
	isFalse(t, logger.state.Started.Load(), "Started")
}

// TestBuilderUnitConversion verifies KB/MB setter pairs share one field.
func TestBuilderUnitConversion(t *testing.T) {
	tests := []struct {
		name string
		set  func(*Builder) *Builder
		get  func(*Config) int64
		want int64
	}{
		{"MaxSizeKB", func(b *Builder) *Builder { return b.MaxSizeKB(512) }, func(c *Config) int64 { return c.MaxSizeKB }, 512},
		{"MaxSizeMB", func(b *Builder) *Builder { return b.MaxSizeMB(2) }, func(c *Config) int64 { return c.MaxSizeKB }, 2 * sizeMultiplier},
		{"MaxTotalSizeKB", func(b *Builder) *Builder { return b.MaxTotalSizeKB(512) }, func(c *Config) int64 { return c.MaxTotalSizeKB }, 512},
		{"MaxTotalSizeMB", func(b *Builder) *Builder { return b.MaxTotalSizeMB(3) }, func(c *Config) int64 { return c.MaxTotalSizeKB }, 3 * sizeMultiplier},
		{"MinDiskFreeKB", func(b *Builder) *Builder { return b.MinDiskFreeKB(64) }, func(c *Config) int64 { return c.MinDiskFreeKB }, 64},
		{"MinDiskFreeMB", func(b *Builder) *Builder { return b.MinDiskFreeMB(4) }, func(c *Config) int64 { return c.MinDiskFreeKB }, 4 * sizeMultiplier},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBuilder().Directory(t.TempDir()).EnableConsole(false)
			logger, err := tt.set(b).Build()
			mustNoErr(t, err, "Build")
			t.Cleanup(func() { _ = logger.Shutdown() })
			equal(t, tt.get(logger.GetConfig()), tt.want, tt.name)
		})
	}
}

// TestBuilderErrorAccumulation verifies a deferred error aborts Build.
func TestBuilderErrorAccumulation(t *testing.T) {
	logger, err := NewBuilder().
		LevelString("invalid-level-string").
		Directory("/some/dir"). // must never be applied
		Build()

	errContains(t, err, "invalid level string", "Build")
	if logger != nil {
		t.Error("Build must return a nil logger on error")
	}

	// A subsequent setter must not clear the accumulated error
	logger, err = NewBuilder().
		LevelString("nonsense").
		LevelString("info").
		Build()
	mustErr(t, err, "Build after error recovery attempt")
	if logger != nil {
		t.Error("Build must return a nil logger on error")
	}
}

// TestBuilderValidationFailure verifies validation errors surface from ApplyConfig.
func TestBuilderValidationFailure(t *testing.T) {
	t.Run("invalid format", func(t *testing.T) {
		logger, err := NewBuilder().Format("yaml").Build()
		errContains(t, err, "invalid format", "Build")
		if logger != nil {
			t.Error("Build must return a nil logger on error")
		}
	})

	t.Run("unwritable directory", func(t *testing.T) {
		// Directory mode is not enforced against uid 0
		if os.Geteuid() == 0 {
			t.Skip("running as root; directory permissions are not enforced")
		}
		parent := t.TempDir()
		mustNoErr(t, os.Chmod(parent, 0o500), "chmod parent")
		t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })

		logger, err := NewBuilder().
			Directory(filepath.Join(parent, "nested")).
			EnableFile(true).
			Build()

		errContains(t, err, "failed to create log directory", "Build")
		if logger != nil {
			t.Error("Build must return a nil logger on error")
		}
	})
}

// TestBuilderDefaults verifies an unconfigured builder yields the package defaults.
func TestBuilderDefaults(t *testing.T) {
	logger, err := NewBuilder().EnableConsole(false).Build()
	mustNoErr(t, err, "Build")
	t.Cleanup(func() { _ = logger.Shutdown() })

	cfg := logger.GetConfig()
	def := DefaultConfig()
	equal(t, cfg.Level, def.Level, "Level")
	equal(t, cfg.Format, def.Format, "Format")
	equal(t, cfg.Name, def.Name, "Name")
	equal(t, cfg.BufferSize, def.BufferSize, "BufferSize")
	equal(t, cfg.Sanitization, def.Sanitization, "Sanitization")
}

