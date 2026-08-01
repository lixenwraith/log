//go:build !unix

package log

import "math"

// getDiskFreeSpace reports unlimited space where statfs is unavailable.
// Total-size limits still apply; only the free-space check is inert.
func (l *Logger) getDiskFreeSpace(path string) (int64, error) {
	return math.MaxInt64, nil
}
