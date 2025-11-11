// FILE: lixenwraith/log/utility.go
package log

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
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
		if !more || count >= int(depth) {
			break
		}
		funcName := filepath.Base(frame.Function)
		parts := strings.Split(funcName, ".")
		lastPart := parts[len(parts)-1]
		if strings.HasPrefix(lastPart, "func") {
			isAnonymous := true
			for _, r := range lastPart[4:] {
				if !unicode.IsDigit(r) {
					isAnonymous = false
					break
				}
			}
			if isAnonymous && len(lastPart) > 4 {
				funcName = fmt.Sprintf("(anonymous in %s)", strings.Join(parts[:len(parts)-1], "."))
			} else {
				funcName = lastPart
			}
		} else {
			funcName = lastPart
		}
		trace = append(trace, funcName)
		count++
	}
	if len(trace) == 0 {
		return "(unknown)"
	}
	// Reverse for caller -> callee order
	for i, j := 0, len(trace)-1; i < j; i, j = i+1, j-1 {
		trace[i], trace[j] = trace[j], trace[i]
	}
	return strings.Join(trace, " -> ")
}

// fmtErrorf wrapper
func fmtErrorf(format string, args ...any) error {
	if !strings.HasPrefix(format, "log: ") {
		format = "log: " + format
	}
	return fmt.Errorf(format, args...)
}

// combineErrors helper
func combineErrors(err1, err2 error) error {
	if err1 == nil {
		return err2
	}
	if err2 == nil {
		return err1
	}
	return fmt.Errorf("%v; %w", err1, err2)
}

// parseKeyValue splits a "key=value" string
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
		return 0, fmtErrorf("invalid level string: '%s' (use debug, info, warn, error, proc, disk, sys)", levelStr)
	}
}