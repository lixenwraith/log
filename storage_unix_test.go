//go:build linux || freebsd || darwin

package log

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A symlink or FIFO planted at the log path is neither followed nor allowed to
// block the processor; a FIFO that opens is still not a regular file. The
// refusal is reported once.
func TestReopenRefusesNonRegularPath(t *testing.T) {
	plant := map[string]func(t *testing.T, path, target string){
		"symlink": func(t *testing.T, path, target string) {
			mustNoErr(t, os.WriteFile(target, nil, 0644), "target")
			mustNoErr(t, os.Symlink(target, path), "symlink")
		},
		"fifo": func(t *testing.T, path, _ string) {
			mustNoErr(t, syscall.Mkfifo(path, 0644), "mkfifo")
		},
		"fifo with a reader": func(t *testing.T, path, _ string) {
			mustNoErr(t, syscall.Mkfifo(path, 0644), "mkfifo")
			r, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			mustNoErr(t, err, "reader")
			t.Cleanup(func() { _ = r.Close() })
		},
	}
	for name, fn := range plant {
		t.Run(name, func(t *testing.T) {
			l, path, diags := followLogger(t, 0)
			logFlush(t, l, "before")
			target := filepath.Join(t.TempDir(), "target")
			mustNoErr(t, os.Remove(path), "rm")
			fn(t, path, target)
			time.Sleep(fileCheckInterval + 5*time.Millisecond)
			logFlush(t, l, "after") // a blocked processor fails the Flush
			if data, err := os.ReadFile(target); err == nil {
				equal(t, len(data), 0, "bytes written through the symlink")
			}
			equal(t, l.state.TotalReopens.Load(), uint64(0), "reopens")
			if d := diags(); len(d) != 1 || !strings.Contains(d[0], "not reopened") {
				t.Errorf("diagnostics %q", d)
			}
		})
	}
}

// The flush tick syncs only a file written since its last successful sync, so
// an idle logger makes no fsync calls. A pipe stands in for the file: it
// refuses fsync, so each attempt is reported.
func TestFlushTickSyncsOnlyAfterAWrite(t *testing.T) {
	l := NewLogger()
	c := DefaultConfig()
	c.EnableConsole = false
	c.EnableFile = true
	c.Directory = t.TempDir()
	mustNoErr(t, l.ApplyConfig(c), "apply") // not started: the test is the processor
	defer l.Shutdown()
	r, w, err := os.Pipe()
	mustNoErr(t, err, "pipe")
	defer r.Close()
	defer w.Close()
	attempts := 0
	l.SetErrorHandler(func(msg string) {
		if strings.Contains(msg, "failed to sync") {
			attempts++
		}
	})
	file := l.state.CurrentFile.Swap(w)
	defer l.state.CurrentFile.Store(file)

	l.handleFlushTick()
	equal(t, attempts, 0, "idle tick")
	stamp := time.Now()
	l.watch.lastCheck = stamp // a due path check would reopen the log path
	l.processLogRecord(logRecord{Flags: l.getFlags(), TimeStamp: stamp, Level: LevelInfo, Args: []any{"written"}})
	l.handleFlushTick()
	l.handleFlushTick()
	equal(t, attempts, 2, "a failed sync is tried again")
	l.state.CurrentFile.Store(file)
	l.handleFlushTick()
	l.state.CurrentFile.Store(w)
	l.handleFlushTick()
	equal(t, attempts, 2, "a successful sync ends the attempts")
}
