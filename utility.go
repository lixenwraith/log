package log

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode"

	"github.com/lixenwraith/log/formatter"
)

// getTrace returns a function call trace string
func getTrace(depth int64, skip int) string {
	if depth <= 0 || depth > 10 {
		return ""
	}
	pc := make([]uintptr, int(depth)+skip)
	n := runtime.Callers(skip+1, pc) // +1 because Callers includes its own frame
	if n == 0 {
		return "(unknown)"
	}
	frames := runtime.CallersFrames(pc[:n])
	var trace []string
	count := 0
	for {
		frame, more := frames.Next()
		if count >= int(depth) {
			break
		}
		funcName := filepath.Base(frame.Function)
		prefix, lastPart, found := strings.CutLast(funcName, ".")
		if !found {
			lastPart = funcName
		}
		if strings.HasPrefix(lastPart, "func") {
			isAnonymous := true
			for _, r := range lastPart[4:] {
				if !unicode.IsDigit(r) {
					isAnonymous = false
					break
				}
			}
			if isAnonymous && len(lastPart) > 4 {
				funcName = fmt.Sprintf("(anonymous in %s)", prefix)
			} else {
				funcName = lastPart
			}
		} else {
			funcName = lastPart
		}
		trace = append(trace, funcName)
		count++
		if !more {
			break
		}
	}
	if len(trace) == 0 {
		return "(unknown)"
	}
	// Reverse for caller -> callee order
	slices.Reverse(trace)
	return strings.Join(trace, " -> ")
}

// fmtErrorf wraps fmt.Errorf with a "log: " prefix
func fmtErrorf(format string, args ...any) error {
	if !strings.HasPrefix(format, "log: ") {
		format = "log: " + format
	}
	return fmt.Errorf(format, args...)
}

// parseKeyValue splits a "key=value" string into its components
func parseKeyValue(arg string) (string, string, error) {
	parts := strings.SplitN(strings.TrimSpace(arg), "=", 2)
	if len(parts) != 2 {
		return "", "", fmtErrorf("invalid format in override string '%s', expected key=value", arg)
	}
	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])
	if key == "" {
		return "", "", fmtErrorf("key cannot be empty in override string '%s'", arg)
	}
	return key, value, nil
}

// Level converts level string to numeric constant
func Level(levelStr string) (int64, error) {
	switch strings.ToLower(strings.TrimSpace(levelStr)) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return LevelDebug, nil
	case "info":
		return LevelInfo, nil
	case "warn":
		return LevelWarn, nil
	case "error":
		return LevelError, nil
	case "proc":
		return LevelProc, nil
	case "disk":
		return LevelDisk, nil
	case "sys":
		return LevelSys, nil
	default:
		return 0, fmtErrorf("invalid level string: '%s' (use trace, debug, info, warn, error, proc, disk, sys)", levelStr)
	}
}

// LevelToString renders a level constant as its display name
func LevelToString(level int64) string { return formatter.LevelToString(level) }
