// FILE: lixenwraith/log/formatter/formatter_test.go
package formatter

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lixenwraith/log/sanitizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatter(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("fluent API", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
		f := New(s).
			Type("json").
			TimestampFormat(time.RFC3339).
			ShowLevel(true).
			ShowTimestamp(true)

		data := f.Format(0, timestamp, 0, "", []any{"test"})
		assert.Contains(t, string(data), `"level":"INFO"`)
		assert.Contains(t, string(data), `"time":"2024-01-01T12:00:00Z"`)
	})

	t.Run("txt format", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
		f := New(s).Type("txt")

		data := f.Format(FlagDefault, timestamp, 0, "", []any{"test message", 123})
		str := string(data)

		assert.Contains(t, str, "2024-01-01")
		assert.Contains(t, str, "INFO")
		assert.Contains(t, str, "test message")
		assert.Contains(t, str, "123")
		assert.True(t, strings.HasSuffix(str, "\n"))
	})

	t.Run("json format", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
		f := New(s).Type("json")

		data := f.Format(FlagDefault, timestamp, 4, "trace1", []any{"warning", true})

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
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
		f := New(s).Type("raw")

		data := f.FormatWithOptions("raw", 0, timestamp, 0, "", []any{"raw", "data", 42})
		str := string(data)

		assert.Equal(t, "raw data 42", str)
		assert.False(t, strings.HasSuffix(str, "\n"))
	})

	t.Run("flag override raw", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
		f := New(s).Type("json") // Configure as JSON

		data := f.Format(FlagRaw, timestamp, 0, "", []any{"forced", "raw"})
		str := string(data)

		assert.Equal(t, "forced raw", str)
	})

	t.Run("structured json", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyJSON)
		f := New(s).Type("json")

		fields := map[string]any{"key1": "value1", "key2": 42}
		data := f.Format(FlagStructuredJSON|FlagDefault, timestamp, 0, "",
			[]any{"structured message", fields})

		var result map[string]any
		err := json.Unmarshal(data[:len(data)-1], &result)
		require.NoError(t, err)

		assert.Equal(t, "structured message", result["message"])
		assert.Equal(t, map[string]any{"key1": "value1", "key2": float64(42)}, result["fields"])
	})

	t.Run("special characters escaping", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyJSON)
		f := New(s).Type("json")

		data := f.Format(FlagDefault, timestamp, 0, "",
			[]any{"test\n\r\t\"\\message"})

		str := string(data)
		assert.Contains(t, str, `test\n\r\t\"\\message`)
	})

	t.Run("error type handling", func(t *testing.T) {
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
		f := New(s).Type("txt")

		err := errors.New("test error")
		data := f.Format(FlagDefault, timestamp, 8, "", []any{err})

		str := string(data)
		assert.Contains(t, str, "test error")
	})
}

func TestLevelToString(t *testing.T) {
	tests := []struct {
		level    int64
		expected string
	}{
		{-4, "DEBUG"},
		{0, "INFO"},
		{4, "WARN"},
		{8, "ERROR"},
		{12, "PROC"},
		{16, "DISK"},
		{20, "SYS"},
		{999, "LEVEL(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, LevelToString(tt.level))
		})
	}
}