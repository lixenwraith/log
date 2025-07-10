// FILE: utility.go
package log

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
)

// getTrace returns a function call trace string.
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

// parseKeyValue splits a "key=value" string.
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

// validateConfigValue checks ranges and specific constraints for parsed config values.
func validateConfigValue(key string, value interface{}) error {
	keyLower := strings.ToLower(key)

	switch keyLower {
	case "name":
		if v, ok := value.(string); ok && strings.TrimSpace(v) == "" {
			return fmtErrorf("log name cannot be empty")
		}
	case "format":
		if v, ok := value.(string); ok && v != "txt" && v != "json" {
			return fmtErrorf("invalid format: '%s' (use txt or json)", v)
		}
	case "extension":
		if v, ok := value.(string); ok && strings.HasPrefix(v, ".") {
			return fmtErrorf("extension should not start with dot: %s", v)
		}
	case "buffer_size":
		if v, ok := value.(int64); ok && v <= 0 {
			return fmtErrorf("buffer_size must be positive: %d", v)
		}
	case "max_size_mb", "max_total_size_mb", "min_disk_free_mb":
		if v, ok := value.(int64); ok && v < 0 {
			return fmtErrorf("%s cannot be negative: %d", key, v)
		}
	case "flush_timer", "disk_check_interval_ms", "min_check_interval_ms", "max_check_interval_ms":
		if v, ok := value.(int64); ok && v <= 0 {
			return fmtErrorf("%s must be positive milliseconds: %d", key, v)
		}
	case "trace_depth":
		if v, ok := value.(int64); ok && (v < 0 || v > 10) {
			return fmtErrorf("trace_depth must be between 0 and 10: %d", v)
		}
	case "retention_period", "retention_check_interval":
		if v, ok := value.(float64); ok && v < 0 {
			return fmtErrorf("%s cannot be negative: %f", key, v)
		}
	}
	return nil
}

// Level converts level string to numeric constant.
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