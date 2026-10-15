//go:build !unix

package log

import (
	"math"
	"os"
)

// reopenFlags open the log path again; see openLogPath
const reopenFlags = os.O_APPEND | os.O_CREATE | os.O_WRONLY

// getDiskFreeSpace reports unlimited space where statfs is unavailable.
// Total-size limits still apply; only the free-space check is inert.
func (l *Logger) getDiskFreeSpace(path string) (int64, error) {
	return math.MaxInt64, nil
}
