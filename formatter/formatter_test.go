package formatter

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
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
		// PolicyRaw — transport escaping applies exactly once.
		// PolicyJSON + json format double-escapes (see TestJSONSanitizerLayering).
		s := sanitizer.New().Policy(sanitizer.PolicyRaw)
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

func TestJSONUTF8Passthrough(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	f := New(sanitizer.New()).Type("json")
	in := "héllo 世界 ✓"
	data := f.Format(FlagDefault, timestamp, 0, "", []any{in})

	var result map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSuffix(data, []byte("\n")), &result))
	assert.Equal(t, in, result["fields"].([]any)[0])
	assert.NotContains(t, string(data), `\u00`, "no per-byte escapes of UTF-8")
}

func TestJSONSanitizerLayering(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Content transform (PolicyTxt) applied before transport escaping
	f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("json")
	data := f.Format(FlagDefault, timestamp, 0, "", []any{"a\x07b"})
	var result map[string]any
	require.NoError(t, json.Unmarshal(bytes.TrimSuffix(data, []byte("\n")), &result))
	assert.Equal(t, "a<07>b", result["fields"].([]any)[0])

	// PolicyJSON + json format: content transform emits literal backslash
	// sequences; transport escaping preserves them (double-escape by design)
	f2 := New(sanitizer.New().Policy(sanitizer.PolicyJSON)).Type("json")
	data2 := f2.Format(FlagDefault, timestamp, 0, "", []any{"a\nb"})
	require.NoError(t, json.Unmarshal(bytes.TrimSuffix(data2, []byte("\n")), &result))
	assert.Equal(t, `a\nb`, result["fields"].([]any)[0])
}

func TestFlagResolution(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	f := New(sanitizer.New()).Type("txt").ShowTimestamp(true).ShowLevel(true)

	// Non-display flags alone inherit configured defaults
	str := string(f.Format(FlagStructuredJSON, timestamp, 0, "", []any{"m"}))
	assert.Contains(t, str, "2024-01-01")
	assert.Contains(t, str, "INFO")

	// Explicit suppression
	str = string(f.Format(FlagNoLevel, timestamp, 0, "", []any{"m"}))
	assert.Contains(t, str, "2024-01-01")
	assert.NotContains(t, str, "INFO")

	str = string(f.Format(FlagNoTimestamp|FlagNoLevel, timestamp, 0, "", []any{"m"}))
	assert.NotContains(t, str, "2024-01-01")
	assert.NotContains(t, str, "INFO")

	// FormatWithOptions is fully explicit: unset Show bits mean off
	str = string(f.FormatWithOptions("txt", 0, timestamp, 0, "", []any{"m"}))
	assert.NotContains(t, str, "INFO")
}

func TestUnknownFormatFallback(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	f := New(sanitizer.New()).Type("txt")
	data := f.FormatWithOptions("xml", FlagShowLevel, timestamp, 8, "", []any{"boom"})
	require.NotNil(t, data)
	assert.Contains(t, string(data), "ERROR")
	assert.Contains(t, string(data), "boom")
}

func TestReturnedSliceInvalidation(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	f := New(sanitizer.New()).Type("txt").ShowTimestamp(false).ShowLevel(false)

	first := f.Format(0, timestamp, 0, "", []any{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	snapshot := string(first)
	_ = f.Format(0, timestamp, 0, "", []any{"b"})
	assert.NotEqual(t, snapshot, string(first),
		"buffered Format output is invalidated by the next buffered call")
}

func TestAppendFormatStable(t *testing.T) {
	timestamp := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	f := New(sanitizer.New()).Type("txt").ShowTimestamp(false).ShowLevel(false)

	first := f.AppendFormat(nil, 0, timestamp, 0, "", []any{"first-payload"})
	snapshot := string(first)
	_ = f.Format(0, timestamp, 0, "", []any{"interleaved-buffered-call"})
	second := f.AppendFormat(nil, 0, timestamp, 0, "", []any{"second"})

	assert.Equal(t, snapshot, string(first), "caller-owned buffer unaffected by buffered calls")
	assert.Equal(t, "second\n", string(second))
}

func TestFormatterConcurrentAppend(t *testing.T) {
	f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("json")
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				out := f.AppendFormat(nil, FlagDefault, time.Now(), 0, "", []any{"w", id, "i", j, "s", "x\x00y"})
				if !json.Valid(bytes.TrimSuffix(out, []byte("\n"))) {
					t.Errorf("invalid JSON: %s", out)
					return
				}
			}
		}(i)
	}
	wg.Wait()
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
