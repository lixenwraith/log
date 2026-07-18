package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestFullLifecycle exercises builder construction, every log entry point,
// runtime reconfiguration, and heartbeat emission end to end.
func TestFullLifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	logger, err := NewBuilder().
		Directory(tmpDir).
		LevelString("debug").
		Format("json").
		MaxSizeKB(1).
		BufferSize(1000).
		EnableConsole(false).
		EnableFile(true).
		HeartbeatLevel(3).
		HeartbeatIntervalS(1).
		Build()

	mustNoErr(t, err, "Build")
	if logger == nil {
		t.Fatal("Build returned a nil logger without an error")
	}
	mustNoErr(t, logger.Start(), "Start")
	t.Cleanup(func() { noErr(t, logger.Shutdown(2*time.Second), "Shutdown") })

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warning message")
	logger.Error("error message")

	logger.LogStructured(LevelInfo, "structured log", map[string]any{
		"user_id": 123,
		"action":  "login",
		"success": true,
	})

	logger.Write("raw data write")
	logger.InfoTrace(2, "trace info")

	mustNoErr(t, logger.ApplyConfigString("console_target=stderr", "trace_depth=1"), "ApplyConfigString")
	logger.Info("after reconfiguration")

	// MaxSizeKB=1 forces rotation, so assertions span every file in the directory
	mustEventually(t, 3*time.Second, "proc heartbeat emitted", func() bool {
		return strings.Contains(readAllLogs(t, tmpDir), `"type","proc"`)
	})
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	content := readAllLogs(t, tmpDir)
	contains(t, content, `"level":"DEBUG"`, "debug level record")
	contains(t, content, `"message":"structured log"`, "structured message key")
	contains(t, content, `"user_id":123`, "structured field")
	contains(t, content, "raw data write", "raw write")
	contains(t, content, "after reconfiguration", "post-reconfiguration record")
	contains(t, content, `"type","disk"`, "disk heartbeat")
	contains(t, content, `"type","sys"`, "sys heartbeat")

	files, err := os.ReadDir(tmpDir)
	mustNoErr(t, err, "ReadDir")
	if len(files) < 1 {
		t.Error("no log files created")
	}
}

// TestConcurrentOperations verifies stability under simultaneous logging,
// reconfiguration, and flushing.
func TestConcurrentOperations(t *testing.T) {
	logger, _ := newTestLogger(t)

	var wg sync.WaitGroup

	for i := range 5 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 20 {
				logger.Info("worker", id, "log", j)
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 3 {
			// Non-fatal only: Fatal outside the test goroutine is undefined behavior
			noErr(t, logger.ApplyConfigString(fmt.Sprintf("trace_depth=%d", i)), "ApplyConfigString")
			time.Sleep(50 * time.Millisecond)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 5 {
			// Timeout must exceed worst-case contention on flushMutex under load
			noErr(t, logger.Flush(2*time.Second), "concurrent Flush")
			time.Sleep(30 * time.Millisecond)
		}
	}()

	wg.Wait()
	noErr(t, logger.Flush(2*time.Second), "final Flush")
}

// TestErrorRecovery covers construction and runtime failure paths.
func TestErrorRecovery(t *testing.T) {
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
			t.Error("Build must return a nil logger on failure")
		}
	})

	t.Run("disk full", func(t *testing.T) {
		logger, _ := newTestLogger(t)

		cfg := logger.GetConfig()
		cfg.MinDiskFreeKB = 1 << 40 // unsatisfiable free-space requirement
		mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

		isFalse(t, logger.performDiskCheck(true), "performDiskCheck under simulated disk full")
		isFalse(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK")

		preDropped := logger.state.DroppedLogs.Load()
		logger.Info("this log entry should be dropped")

		// The processor drops asynchronously after dequeuing
		mustEventually(t, time.Second, "drop counter incremented", func() bool {
			return logger.state.DroppedLogs.Load() > preDropped
		})

		// Recovery: restoring the threshold must clear the failure state
		cfg = logger.GetConfig()
		cfg.MinDiskFreeKB = 0
		mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig recovery")
		isTrue(t, logger.performDiskCheck(true), "performDiskCheck after recovery")
		isTrue(t, logger.state.DiskStatusOK.Load(), "DiskStatusOK after recovery")
	})
}

