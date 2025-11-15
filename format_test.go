// FILE: lixenwraith/log/format_test.go
package log

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lixenwraith/log/sanitizer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatter tests the output of the formatter for txt, json, and raw formats
func TestFormatter(t *testing.T) {
	f := NewFormatter("txt", 1024, time.RFC3339Nano, sanitizer.PolicyRaw)
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	t.Run("txt format", func(t *testing.T) {
		data := f.Format("txt", FlagDefault, timestamp, LevelInfo, "", []any{"test message", 123})
		str := string(data)

		assert.Contains(t, str, "2024-01-01")
		assert.Contains(t, str, "INFO")
		assert.Contains(t, str, "test message")
		assert.Contains(t, str, "123")
		assert.True(t, strings.HasSuffix(str, "\n"))
	})

	f = NewFormatter("json", 1024, time.RFC3339Nano, sanitizer.PolicyRaw)
	t.Run("json format", func(t *testing.T) {
		data := f.Format("json", FlagDefault, timestamp, LevelWarn, "trace1", []any{"warning", true})

		var result map[string]any
		err := json.Unmarshal(data[:len(data)-1], &result) // Remove trailing newline
		require.NoError(t, err)

		assert.Equal(t, "WARN", result["level"])
		assert.Equal(t, "trace1", result["trace"])
		fields := result["fields"].([]any)
		assert.Equal(t, "warning", fields[0])
		assert.Equal(t, true, fields[1])
	})

	f = NewFormatter("raw", 1024, time.RFC3339Nano, sanitizer.PolicyRaw)
	t.Run("raw format", func(t *testing.T) {
		data := f.Format("raw", 0, timestamp, LevelInfo, "", []any{"raw", "data", 42})
		str := string(data)

		assert.Equal(t, "raw data 42", str)
		assert.False(t, strings.HasSuffix(str, "\n"))
	})

	t.Run("flag override raw", func(t *testing.T) {
		data := f.Format("txt", FlagRaw, timestamp, LevelInfo, "", []any{"forced", "raw"})
		str := string(data)

		assert.Equal(t, "forced raw", str)
	})

	f = NewFormatter("json", 1024, time.RFC3339Nano, sanitizer.PolicyJSON)
	t.Run("structured json", func(t *testing.T) {
		fields := map[string]any{"key1": "value1", "key2": 42}
		data := f.Format("json", FlagStructuredJSON|FlagDefault, timestamp, LevelInfo, "",
			[]any{"structured message", fields})

		var result map[string]any
		err := json.Unmarshal(data[:len(data)-1], &result)
		require.NoError(t, err)

		assert.Equal(t, "structured message", result["message"])
		assert.Equal(t, map[string]any{"key1": "value1", "key2": float64(42)}, result["fields"])
	})

	f = NewFormatter("json", 1024, time.RFC3339Nano, sanitizer.PolicyJSON)
	t.Run("special characters escaping", func(t *testing.T) {
		data := f.Format("json", FlagDefault, timestamp, LevelInfo, "",
			[]any{"test\n\r\t\"\\message"})

		str := string(data)
		assert.Contains(t, str, `test\n\r\t\"\\message`)
	})

	t.Run("error type handling", func(t *testing.T) {
		err := errors.New("test error")
		data := f.Format("txt", FlagDefault, timestamp, LevelError, "", []any{err})

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

// TestControlCharacterWrite verifies that control characters are safely handled in raw output
func TestControlCharacterWrite(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.Format = "raw"
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	// Test various control characters with expected sanitized output
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{"null bytes", "test\x00data", "test<00>data"},
		{"bell", "alert\x07message", "alert<07>message"},
		{"backspace", "back\x08space", "back<08>space"},
		{"form feed", "page\x0Cbreak", "page<0c>break"},
		{"vertical tab", "vertical\x0Btab", "vertical<0b>tab"},
		{"escape", "escape\x1B[31mcolor", "escape<1b>[31mcolor"},
		{"mixed", "\x00\x01\x02test\x1F\x7Fdata", "<00><01><02>test<1f><7f>data"},
	}

	for _, tc := range testCases {
		logger.Message(tc.input)
	}

	logger.Flush(time.Second)

	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)

	// Verify each test case produced correct sanitized output
	for _, tc := range testCases {
		assert.Contains(t, string(content), tc.expected,
			"Test case '%s' should produce hex-encoded control chars", tc.name)
	}
}

// TestRawSanitizedOutput verifies that raw output is correctly sanitized
func TestRawSanitizedOutput(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	// Use raw format instead of Write() to test sanitization
	cfg := logger.GetConfig()
	cfg.Format = "raw"
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	// 1. A string with valid multi-byte UTF-8 should be unchanged
	utf8String := "Hello │ 世界"

	// 2. A string with single-byte control chars should have them encoded
	stringWithControl := "start-\x07-end"
	expectedStringOutput := "start-<07>-end"

	// 3. A []byte with control chars should have them encoded, not stripped
	bytesWithControl := []byte("data\x00with\x08bytes")
	expectedBytesOutput := "data<00>with<08>bytes"

	// 4. A string with a multi-byte non-printable rune (U+0085, NEXT LINE)
	multiByteControl := "line1\u0085line2"
	expectedMultiByteOutput := "line1<c285>line2"

	// Log all cases
	logger.Message(utf8String, stringWithControl, bytesWithControl, multiByteControl)
	logger.Flush(time.Second)

	// Read and verify the single line of output
	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)
	logOutput := string(content)

	// The output should be one line with spaces between the sanitized parts
	expectedOutput := strings.Join([]string{
		utf8String,
		expectedStringOutput,
		expectedBytesOutput,
		expectedMultiByteOutput,
	}, " ")

	assert.Equal(t, expectedOutput, logOutput)
}