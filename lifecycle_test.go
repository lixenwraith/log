package log

import (
	"strings"
	"testing"
	"time"
)

// TestStartStopLifecycle verifies stop/restart transitions and processor liveness.
func TestStartStopLifecycle(t *testing.T) {
	logger, _ := newTestLogger(t)

	isTrue(t, logger.state.Started.Load(), "Started after setup")
	isFalse(t, logger.state.ProcessorExited.Load(), "processor must be running")

	mustNoErr(t, logger.Stop(), "Stop")
	isFalse(t, logger.state.Started.Load(), "Started after Stop")
	isTrue(t, logger.state.ProcessorExited.Load(), "Stop must join the processor")

	mustNoErr(t, logger.Start(), "restart")
	isTrue(t, logger.state.Started.Load(), "Started after restart")
	isFalse(t, logger.state.ProcessorExited.Load(), "processor must be running after restart")
}

// TestStartStopIdempotence verifies repeated Start/Stop calls are no-ops.
func TestStartStopIdempotence(t *testing.T) {
	t.Run("start already started", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		noErr(t, logger.Start(), "redundant Start")
		isTrue(t, logger.state.Started.Load(), "Started")
	})

	t.Run("stop already stopped", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		mustNoErr(t, logger.Stop(), "first Stop")
		noErr(t, logger.Stop(), "redundant Stop")
		isFalse(t, logger.state.Started.Load(), "Started")
	})
}

// TestStopReconfigureRestart verifies a format change applied while stopped
// takes effect on restart, appending to the same file.
func TestStopReconfigureRestart(t *testing.T) {
	tmpDir := t.TempDir()
	logger := NewLogger()

	cfg := DefaultConfig()
	cfg.Directory = tmpDir
	cfg.EnableConsole = false
	cfg.EnableFile = true
	cfg.Format = "txt"
	cfg.ShowTimestamp = false
	cfg.FlushIntervalMs = 10
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig txt")
	mustNoErr(t, logger.Start(), "Start")

	logger.Info("first message")
	mustNoErr(t, logger.Flush(time.Second), "Flush")
	mustNoErr(t, logger.Stop(), "Stop")

	cfg2 := logger.GetConfig()
	cfg2.Format = "json"
	mustNoErr(t, logger.ApplyConfig(cfg2), "ApplyConfig json")
	mustNoErr(t, logger.Start(), "restart")

	logger.Info("second message")
	mustNoErr(t, logger.Shutdown(time.Second), "Shutdown")

	content := readLog(t, tmpDir)
	contains(t, content, `INFO "first message"`, "record from txt configuration")
	contains(t, content, `"fields":["second message"]`, "record from json configuration")
}

// TestLoggingOnStoppedLogger verifies records submitted while stopped are discarded.
func TestLoggingOnStoppedLogger(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	logger.Info("this should be logged")
	mustNoErr(t, logger.Flush(time.Second), "Flush")
	mustNoErr(t, logger.Stop(), "Stop")

	logger.Warn("this should NOT be logged")
	mustNoErr(t, logger.Shutdown(time.Second), "Shutdown")

	content := readLog(t, tmpDir)
	contains(t, content, "this should be logged", "pre-stop record")
	notContains(t, content, "this should NOT be logged", "post-stop record")
}

// TestShutdownTerminalState verifies Shutdown is terminal and non-restartable.
func TestShutdownTerminalState(t *testing.T) {
	logger, _ := newTestLogger(t)

	isTrue(t, logger.state.IsInitialized.Load(), "IsInitialized before shutdown")
	logger.Info("pre-shutdown record")
	mustNoErr(t, logger.Shutdown(2*time.Second), "Shutdown")

	isTrue(t, logger.state.ShutdownCalled.Load(), "ShutdownCalled")
	isTrue(t, logger.state.LoggerDisabled.Load(), "LoggerDisabled")
	isFalse(t, logger.state.IsInitialized.Load(), "Shutdown must de-initialize")
	isFalse(t, logger.state.Started.Load(), "Shutdown must stop")

	// Restart is impossible without a fresh ApplyConfig
	errContains(t, logger.Start(), "logger not initialized", "Start after Shutdown")
	// Logging degrades to a silent no-op rather than panicking
	logger.Info("this will not be logged")
	errContains(t, logger.Flush(time.Second), "not initialized", "Flush after Shutdown")
}

// TestShutdownEdgeCases covers uninitialized, repeated, and timed-out shutdowns.
func TestShutdownEdgeCases(t *testing.T) {
	t.Run("before init", func(t *testing.T) {
		logger := NewLogger()
		noErr(t, logger.Shutdown(), "Shutdown on uninitialized logger")
		// State must be left reusable: ApplyConfig may still follow
		isFalse(t, logger.state.ShutdownCalled.Load(), "ShutdownCalled must be rolled back")
		isFalse(t, logger.state.LoggerDisabled.Load(), "LoggerDisabled must be rolled back")
	})

	t.Run("double shutdown", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		noErr(t, logger.Shutdown(), "first Shutdown")
		noErr(t, logger.Shutdown(), "second Shutdown")
	})

	t.Run("timeout", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		for i := range 200 {
			logger.Info("flood", i)
		}
		// Stop may time out; terminal state transitions are unconditional
		_ = logger.Shutdown(time.Millisecond)
		isTrue(t, logger.state.ShutdownCalled.Load(), "ShutdownCalled")
		isFalse(t, logger.state.IsInitialized.Load(), "IsInitialized")
	})
}

// TestFlush covers the success path and both failure modes.
func TestFlush(t *testing.T) {
	t.Run("successful", func(t *testing.T) {
		logger, tmpDir := newTestLogger(t)

		logger.Info("flush test")
		mustNoErr(t, logger.Flush(time.Second), "Flush")

		mustEventually(t, time.Second, "record written", func() bool {
			return strings.Contains(readLog(t, tmpDir), "flush test")
		})
	})

	t.Run("timeout", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		errContains(t, logger.Flush(time.Nanosecond), "timeout", "Flush")
	})

	t.Run("on stopped logger", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		mustNoErr(t, logger.Stop(), "Stop")
		errContains(t, logger.Flush(time.Second), "logger not started", "Flush")
	})

	t.Run("after shutdown", func(t *testing.T) {
		logger, _ := newTestLogger(t)
		mustNoErr(t, logger.Shutdown(), "Shutdown")
		errContains(t, logger.Flush(time.Second), "not initialized", "Flush")
	})
}

