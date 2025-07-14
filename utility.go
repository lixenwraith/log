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

// validateConfigValue validates a single configuration field
func validateConfigValue(key string, value any) error {
	keyLower := strings.ToLower(key)
	switch keyLower {
	case "name":
		v, ok := value.(string)
		if !ok {
			return fmtErrorf("name must be string, got %T", value)
		}
		if strings.TrimSpace(v) == "" {
			return fmtErrorf("log name cannot be empty")
		}

	case "format":
		v, ok := value.(string)
		if !ok {
			return fmtErrorf("format must be string, got %T", value)
		}
		if v != "txt" && v != "json" && v != "raw" {
			return fmtErrorf("invalid format: '%s' (use txt, json, or raw)", v)
		}

	case "extension":
		v, ok := value.(string)
		if !ok {
			return fmtErrorf("extension must be string, got %T", value)
		}
		if strings.HasPrefix(v, ".") {
			return fmtErrorf("extension should not start with dot: %s", v)
		}

	case "timestamp_format":
		v, ok := value.(string)
		if !ok {
			return fmtErrorf("timestamp_format must be string, got %T", value)
		}
		if strings.TrimSpace(v) == "" {
			return fmtErrorf("timestamp_format cannot be empty")
		}

	case "buffer_size":
		v, ok := value.(int64)
		if !ok {
			return fmtErrorf("buffer_size must be int64, got %T", value)
		}
		if v <= 0 {
			return fmtErrorf("buffer_size must be positive: %d", v)
		}

	case "max_size_mb", "max_total_size_mb", "min_disk_free_mb":
		v, ok := value.(int64)
		if !ok {
			return fmtErrorf("%s must be int64, got %T", key, value)
		}
		if v < 0 {
			return fmtErrorf("%s cannot be negative: %d", key, v)
		}

	case "flush_interval_ms", "disk_check_interval_ms", "min_check_interval_ms", "max_check_interval_ms":
		v, ok := value.(int64)
		if !ok {
			return fmtErrorf("%s must be int64, got %T", key, value)
		}
		if v <= 0 {
			return fmtErrorf("%s must be positive milliseconds: %d", key, v)
		}

	case "trace_depth":
		v, ok := value.(int64)
		if !ok {
			return fmtErrorf("trace_depth must be int64, got %T", value)
		}
		if v < 0 || v > 10 {
			return fmtErrorf("trace_depth must be between 0 and 10: %d", v)
		}

	case "retention_period_hrs", "retention_check_mins":
		v, ok := value.(float64)
		if !ok {
			return fmtErrorf("%s must be float64, got %T", key, value)
		}
		if v < 0 {
			return fmtErrorf("%s cannot be negative: %f", key, v)
		}

	case "heartbeat_level":
		v, ok := value.(int64)
		if !ok {
			return fmtErrorf("heartbeat_level must be int64, got %T", value)
		}
		if v < 0 || v > 3 {
			return fmtErrorf("heartbeat_level must be between 0 and 3: %d", v)
		}

	case "heartbeat_interval_s":
		_, ok := value.(int64)
		if !ok {
			return fmtErrorf("heartbeat_interval_s must be int64, got %T", value)
		}
		// Note: only validate positive if heartbeat is enabled (cross-field validation)

	case "stdout_target":
		v, ok := value.(string)
		if !ok {
			return fmtErrorf("stdout_target must be string, got %T", value)
		}
		if v != "stdout" && v != "stderr" {
			return fmtErrorf("invalid stdout_target: '%s' (use stdout or stderr)", v)
		}

	case "level":
		// Level validation if needed
		_, ok := value.(int64)
		if !ok {
			return fmtErrorf("level must be int64, got %T", value)
		}

	// Fields that don't need validation beyond type
	case "directory", "show_timestamp", "show_level", "enable_adaptive_interval",
		"enable_periodic_sync", "enable_stdout", "disable_file", "internal_errors_to_stderr":
		// Type checking handled by config system
		return nil

	default:
		// Unknown field - let config system handle it
		return nil
	}

	return nil
}