//go:build unix

package log

import (
	"os"
	"path/filepath"
	"syscall"
)

// getDiskFreeSpace retrieves available disk space for the given path
func (l *Logger) getDiskFreeSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmtErrorf("log directory '%s' does not exist for disk check: %w", path, err)
		}
		return 0, fmtErrorf("failed to stat log directory '%s': %w", path, err)
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
	}

	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmtErrorf("failed to get disk stats for '%s': %w", path, err)
	}
	// Explicit cast to int64 to satisfy both Linux and FreeBSD
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
