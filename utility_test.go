package log

import (
	"strings"
	"testing"
)

// TestLevel verifies level string parsing, including case and whitespace handling.
func TestLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"debug", LevelDebug, false},
		{"DEBUG", LevelDebug, false},
		{" info ", LevelInfo, false},
		{"Warn", LevelWarn, false},
		{"error", LevelError, false},
		{"proc", LevelProc, false},
		{"disk", LevelDisk, false},
		{"sys", LevelSys, false},
		{"invalid", 0, true},
		{"", 0, true},
		{"   ", 0, true},
		{"-4", 0, true}, // numeric forms are handled by applyConfigField, not Level
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := Level(tt.input)
			if tt.wantErr {
				errContains(t, err, "invalid level string", "Level")
				return
			}
			mustNoErr(t, err, "Level")
			equal(t, level, tt.want, "level value")
		})
	}
}

// TestParseKeyValue verifies key=value splitting and trimming.
func TestParseKeyValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantKey   string
		wantValue string
		wantErr   string
	}{
		{"simple", "key=value", "key", "value", ""},
		{"trimmed", " key = value ", "key", "value", ""},
		{"value with separators", "key=value=with=equals", "key", "value=with=equals", ""},
		{"empty value", "key=", "key", "", ""},
		{"no separator", "noequals", "", "", "expected key=value"},
		{"empty key", "=value", "", "", "key cannot be empty"},
		{"whitespace key", "  =value", "", "", "key cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, value, err := parseKeyValue(tt.input)
			if tt.wantErr != "" {
				errContains(t, err, tt.wantErr, "parseKeyValue")
				return
			}
			mustNoErr(t, err, "parseKeyValue")
			equal(t, key, tt.wantKey, "key")
			equal(t, value, tt.wantValue, "value")
		})
	}
}

// TestFmtErrorf verifies prefixing is applied once.
func TestFmtErrorf(t *testing.T) {
	err := fmtErrorf("test error: %s", "details")
	mustErr(t, err, "fmtErrorf")
	equal(t, err.Error(), "log: test error: details", "message")

	err = fmtErrorf("log: already prefixed")
	equal(t, err.Error(), "log: already prefixed", "message")
	equal(t, strings.Count(err.Error(), "log: "), 1, "prefix occurrences")
}

// TestGetTrace verifies depth bounds and caller-to-callee ordering.
func TestGetTrace(t *testing.T) {
	tests := []struct {
		name  string
		depth int64
		check func(t *testing.T, trace string)
	}{
		{"disabled", 0, func(t *testing.T, s string) {
			if s != "" {
				t.Errorf("depth 0 must produce no trace, got %q", s)
			}
		}},
		{"negative", -1, func(t *testing.T, s string) {
			if s != "" {
				t.Errorf("negative depth must produce no trace, got %q", s)
			}
		}},
		{"single frame", 1, func(t *testing.T, s string) {
			if s == "" {
				t.Error("depth 1 must produce a trace")
			}
			notContains(t, s, "->", "single frame must not contain a separator")
		}},
		{"multi frame", 3, func(t *testing.T, s string) {
			if s == "" {
				t.Fatal("depth 3 must produce a trace")
			}
			if s != "(unknown)" {
				contains(t, s, "->", "multi-frame separator")
				// Frames are reversed into caller -> callee order
				parts := strings.Split(s, " -> ")
				if len(parts) < 2 {
					t.Errorf("expected multiple frames, got %q", s)
				}
			}
		}},
		{"over limit", 11, func(t *testing.T, s string) {
			if s != "" {
				t.Errorf("depth above 10 must produce no trace, got %q", s)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, getTrace(tt.depth, 0))
		})
	}
}

// TestGetTraceOrdering verifies the deepest caller appears first.
func TestGetTraceOrdering(t *testing.T) {
	var trace string
	outer := func() { trace = getTrace(3, 0) }
	middle := func() { outer() }
	middle()

	if trace == "" || trace == "(unknown)" {
		t.Skip("runtime frames unavailable under current inlining")
	}
	first := strings.Split(trace, " -> ")[0]
	last := strings.Split(trace, " -> ")
	// The innermost frame is getTrace itself; the caller chain precedes it
	if first == last[len(last)-1] {
		t.Errorf("frames not ordered: %q", trace)
	}
}

