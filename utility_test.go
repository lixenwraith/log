// FILE: utility_test.go
package log

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"debug", LevelDebug, false},
		{"DEBUG", LevelDebug, false},
		{" info ", LevelInfo, false},
		{"warn", LevelWarn, false},
		{"error", LevelError, false},
		{"proc", LevelProc, false},
		{"disk", LevelDisk, false},
		{"sys", LevelSys, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := Level(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, level)
			}
		})
	}
}

func TestParseKeyValue(t *testing.T) {
	tests := []struct {
		input     string
		wantKey   string
		wantValue string
		wantErr   bool
	}{
		{"key=value", "key", "value", false},
		{" key = value ", "key", "value", false},
		{"key=value=with=equals", "key", "value=with=equals", false},
		{"noequals", "", "", true},
		{"=value", "", "", true},
		{"key=", "key", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			key, value, err := parseKeyValue(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantKey, key)
				assert.Equal(t, tt.wantValue, value)
			}
		})
	}
}

func TestFmtErrorf(t *testing.T) {
	err := fmtErrorf("test error: %s", "details")
	assert.Error(t, err)
	assert.Equal(t, "log: test error: details", err.Error())

	// Already prefixed
	err = fmtErrorf("log: already prefixed")
	assert.Equal(t, "log: already prefixed", err.Error())
}

func TestGetTrace(t *testing.T) {
	// Test various depths
	tests := []struct {
		depth int64
		check func(string)
	}{
		{0, func(s string) { assert.Empty(t, s) }},
		{1, func(s string) { assert.NotEmpty(t, s) }},
		{3, func(s string) {
			assert.NotEmpty(t, s)
			assert.True(t, strings.Contains(s, "->") || s == "(unknown)")
		}},
		{11, func(s string) { assert.Empty(t, s) }}, // Over limit
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("depth_%d", tt.depth), func(t *testing.T) {
			trace := getTrace(tt.depth, 0)
			tt.check(trace)
		})
	}
}