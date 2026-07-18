package log

import (
	"strings"
	"testing"
	"time"
)

// Tests the integration between the log package and the formatter/sanitizer packages.

// TestFormatterIntegration verifies each format reaches the file writer intact.
func TestFormatterIntegration(t *testing.T) {
	tests := []struct {
		name   string
		format string
		checks []string
	}{
		{"txt", "txt", []string{`INFO "test message"`}},
		{"json", "json", []string{`"level":"INFO"`, `"fields":["test message"]`}},
		{"raw", "raw", []string{"test message"}},
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
			cfg.EnableConsole = false
			cfg.EnableFile = true
			cfg.FlushIntervalMs = 10

			mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")
			mustNoErr(t, logger.Start(), "Start")
			t.Cleanup(func() { _ = logger.Shutdown() })

			logger.Info("test message")
			mustNoErr(t, logger.Flush(time.Second), "Flush")

			mustEventually(t, time.Second, "record written", func() bool {
				return len(readLog(t, tmpDir)) > 0
			})

			content := readLog(t, tmpDir)
			for _, want := range tt.checks {
				contains(t, content, want, tt.format+" output")
			}
		})
	}
}

// TestStructuredJSONOutput verifies FlagStructuredJSON emits a message key and a
// marshaled field object rather than a positional fields array.
func TestStructuredJSONOutput(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Format = "json"
	cfg.ShowTimestamp = false
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	logger.LogStructured(LevelInfo, "structured log", map[string]any{
		"user_id": 123,
		"action":  "login",
		"success": true,
	})
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	mustEventually(t, time.Second, "record written", func() bool {
		return strings.Contains(readLog(t, tmpDir), "structured log")
	})

	content := readLog(t, tmpDir)
	contains(t, content, `"message":"structured log"`, "message key")
	// json.Marshal orders map keys lexically
	contains(t, content, `"fields":{"action":"login","success":true,"user_id":123}`, "field object")
	notContains(t, content, `"fields":[`, "structured branch must not fall through to the array form")
}

// TestControlCharacterSanitization verifies PolicyTxt hex-encodes every
// non-printable rune on the raw output path. Tab and DEL are non-printable per
// strconv.IsPrint and are encoded like any other control byte.
func TestControlCharacterSanitization(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Format = "raw"
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	cfg.Sanitization = PolicyTxt
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

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
		{"tab", "col1\tcol2", "col1<09>col2"},
		{"escape", "escape\x1B[31mcolor", "escape<1b>[31mcolor"},
		{"del", "del\x7Fmark", "del<7f>mark"},
		{"mixed", "\x00\x01\x02test\x1F\x7Fdata", "<00><01><02>test<1f><7f>data"},
		// '<' is encoded so input cannot forge a hex marker
		{"literal angle bracket", "a<00>b", "a<3c>00>b"},
		{"utf8 untouched", "Hello │ 世界", "Hello │ 世界"},
	}

	for _, tc := range testCases {
		logger.Message(tc.input)
	}
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	// Records append in submission order; the last one gates the read
	last := testCases[len(testCases)-1].expected
	mustEventually(t, time.Second, "all records written", func() bool {
		return strings.Contains(readLog(t, tmpDir), last)
	})

	content := readLog(t, tmpDir)
	for _, tc := range testCases {
		contains(t, content, tc.expected, tc.name)
	}
}

// TestRawSanitizedOutput verifies raw format emits space-joined arguments with
// no framing, and that sanitization applies per argument across string and []byte.
func TestRawSanitizedOutput(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Format = "raw"
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	cfg.Sanitization = PolicyTxt
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	const (
		utf8String       = "Hello │ 世界"
		stringWithCtl    = "start-\x07-end"
		multiByteControl = "line1\u0085line2"
	)
	bytesWithCtl := []byte("data\x00with\x08bytes")

	// U+0085 is a single non-printable rune; its two UTF-8 bytes encode as one marker
	want := strings.Join([]string{
		utf8String,
		"start-<07>-end",
		"data<00>with<08>bytes",
		"line1<c285>line2",
	}, " ")

	logger.Message(utf8String, stringWithCtl, bytesWithCtl, multiByteControl)
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	mustEventually(t, time.Second, "record written", func() bool {
		return len(readLog(t, tmpDir)) > 0
	})

	equal(t, readLog(t, tmpDir), want, "raw output must match exactly")
}

// TestPolicyRawPassthrough verifies the default policy performs no substitution.
func TestPolicyRawPassthrough(t *testing.T) {
	logger, tmpDir := newTestLogger(t)

	cfg := logger.GetConfig()
	cfg.Format = "raw"
	cfg.ShowTimestamp = false
	cfg.ShowLevel = false
	cfg.Sanitization = PolicyRaw
	mustNoErr(t, logger.ApplyConfig(cfg), "ApplyConfig")

	logger.Message("esc\x1b[31mred")
	mustNoErr(t, logger.Flush(time.Second), "Flush")

	mustEventually(t, time.Second, "record written", func() bool {
		return len(readLog(t, tmpDir)) > 0
	})

	equal(t, readLog(t, tmpDir), "esc\x1b[31mred", "PolicyRaw must not transform input")
}
