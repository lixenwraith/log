// This file tests the integration between log package and formatter package
package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoggerFormatterIntegration verifies logger correctly uses the new formatter package
func TestLoggerFormatterIntegration(t *testing.T) {
	tests := []struct {
		name   string
		format string
		check  func(t *testing.T, content string)
	}{
		{
			name:   "txt format",
			format: "txt",
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, `INFO "test message"`)
			},
		},
		{
			name:   "json format",
			format: "json",
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, `"level":"INFO"`)
				assert.Contains(t, content, `"fields":["test message"]`)
			},
		},
		{
			name:   "raw format",
			format: "raw",
			check: func(t *testing.T, content string) {
				assert.Contains(t, content, "test message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			logger := NewLogger()

			cfg := DefaultConfig()
			cfg.Directory = tmpDir
			cfg.Format = tt.format
			cfg.ShowTimestamp = false
			cfg.ShowLevel = true
			cfg.EnableFile = true
			cfg.FlushIntervalMs = 10

			err := logger.ApplyConfig(cfg)
			require.NoError(t, err)

			err = logger.Start()
			require.NoError(t, err)
			defer logger.Shutdown()

			logger.Info("test message")

			err = logger.Flush(time.Second)
			require.NoError(t, err)

			time.Sleep(50 * time.Millisecond)

			content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
			require.NoError(t, err)

			tt.check(t, string(content))
		})
	}
}

// TestControlCharacterWriteWithFormatter verifies control character handling through formatter
func TestControlCharacterWriteWithFormatter(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.Format = "raw"
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	cfg.Sanitization = PolicyTxt
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

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

	time.Sleep(50 * time.Millisecond) // Small delay for file write

	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)

	for _, tc := range testCases {
		assert.Contains(t, string(content), tc.expected,
			"Test case '%s' should produce hex-encoded control chars", tc.name)
	}
}

// TestRawSanitizedOutputWithFormatter verifies raw output sanitization through formatter
func TestRawSanitizedOutputWithFormatter(t *testing.T) {
	logger, tmpDir := createTestLogger(t)
	defer logger.Shutdown()

	cfg := logger.GetConfig()
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	cfg.Format = "raw"
	cfg.Sanitization = PolicyTxt
	err := logger.ApplyConfig(cfg)
	require.NoError(t, err)

	utf8String := "Hello │ 世界"
	stringWithControl := "start-\x07-end"
	expectedStringOutput := "start-<07>-end"
	bytesWithControl := []byte("data\x00with\x08bytes")
	expectedBytesOutput := "data<00>with<08>bytes"
	multiByteControl := "line1\u0085line2"
	expectedMultiByteOutput := "line1<c285>line2"

	logger.Message(utf8String, stringWithControl, bytesWithControl, multiByteControl)
	logger.Flush(time.Second)

	content, err := os.ReadFile(filepath.Join(tmpDir, "log.log"))
	require.NoError(t, err)
	logOutput := string(content)

	expectedOutput := strings.Join([]string{
		utf8String,
		expectedStringOutput,
		expectedBytesOutput,
		expectedMultiByteOutput,
	}, " ")

	assert.Equal(t, expectedOutput, logOutput)
}