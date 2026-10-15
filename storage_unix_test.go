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
