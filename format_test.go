// FILE: lixenwraith/log/format_test.go
package log

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSerializer tests the output of the serializer for txt, json, and raw formats
func TestSerializer(t *testing.T) {
	s := newSerializer()
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("txt format", func(t *testing.T) {
		data := s.serialize("txt", FlagDefault, timestamp, LevelInfo, "", []any{"test message", 123})
		str := string(data)

		assert.Contains(t, str, "2024-01-01")
		assert.Contains(t, str, "INFO")
		assert.Contains(t, str, "test message")
		assert.Contains(t, str, "123")
		assert.True(t, strings.HasSuffix(str, "\n"))
	})

	t.Run("json format", func(t *testing.T) {
		data := s.serialize("json", FlagDefault, timestamp, LevelWarn, "trace1", []any{"warning", true})

		var result map[string]any
		err := json.Unmarshal(data[:len(data)-1], &result) // Remove trailing newline
		require.NoError(t, err)

		assert.Equal(t, "WARN", result["level"])
		assert.Equal(t, "trace1", result["trace"])
		fields := result["fields"].([]any)
		assert.Equal(t, "warning", fields[0])
		assert.Equal(t, true, fields[1])
	})

	t.Run("raw format", func(t *testing.T) {
		data := s.serialize("raw", 0, timestamp, LevelInfo, "", []any{"raw", "data", 42})
		str := string(data)

		assert.Equal(t, "raw data 42", str)
		assert.False(t, strings.HasSuffix(str, "\n"))
	})

	t.Run("flag override raw", func(t *testing.T) {
		data := s.serialize("txt", FlagRaw, timestamp, LevelInfo, "", []any{"forced", "raw"})
		str := string(data)

		assert.Equal(t, "forced raw", str)
	})

	t.Run("structured json", func(t *testing.T) {
		fields := map[string]any{"key1": "value1", "key2": 42}
		data := s.serialize("json", FlagStructuredJSON|FlagDefault, timestamp, LevelInfo, "",
			[]any{"structured message", fields})

		var result map[string]any
		err := json.Unmarshal(data[:len(data)-1], &result)
		require.NoError(t, err)

		assert.Equal(t, "structured message", result["message"])
		assert.Equal(t, map[string]any{"key1": "value1", "key2": float64(42)}, result["fields"])
	})

	t.Run("special characters escaping", func(t *testing.T) {
		data := s.serialize("json", FlagDefault, timestamp, LevelInfo, "",
			[]any{"test\n\r\t\"\\message"})

		str := string(data)
		assert.Contains(t, str, `test\n\r\t\"\\message`)
	})

	t.Run("error type handling", func(t *testing.T) {
		err := errors.New("test error")
		data := s.serialize("txt", FlagDefault, timestamp, LevelError, "", []any{err})

		str := string(data)
		assert.Contains(t, str, "test error")
	})
}

// TestLevelToString verifies the conversion of log level constants to strings
func TestLevelToString(t *testing.T) {
	tests := []struct {
		level    int64
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelProc, "PROC"},
		{LevelDisk, "DISK"},
		{LevelSys, "SYS"},
		{999, "LEVEL(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, levelToString(tt.level))
		})
	}
}