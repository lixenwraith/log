package formatter

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lixenwraith/log/sanitizer"
)

func eq[T comparable](tb testing.TB, got, want T, ctx string) {
	tb.Helper()
	if got != want {
		tb.Errorf("%s: got %#v, want %#v", ctx, got, want)
	}
}

func contains(tb testing.TB, haystack, needle, ctx string) {
	tb.Helper()
	if !strings.Contains(haystack, needle) {
		tb.Errorf("%s: %q not found in %q", ctx, needle, haystack)
	}
}

func notContains(tb testing.TB, haystack, needle, ctx string) {
	tb.Helper()
	if strings.Contains(haystack, needle) {
		tb.Errorf("%s: %q unexpectedly present in %q", ctx, needle, haystack)
	}
}

func mustNoErr(tb testing.TB, err error, ctx string) {
	tb.Helper()
	if err != nil {
		tb.Fatalf("%s: unexpected error: %v", ctx, err)
	}
}

// unmarshalRecord parses one json record, stripping the trailing newline.
func unmarshalRecord(tb testing.TB, data []byte) map[string]any {
	tb.Helper()
	var result map[string]any
	if err := json.Unmarshal(bytes.TrimSuffix(data, []byte("\n")), &result); err != nil {
		tb.Fatalf("parse record %q: %v", data, err)
	}
	return result
}

var testStamp = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

type stringerValue struct{}

func (stringerValue) String() string { return "stringer" }

func TestFormatTxt(t *testing.T) {
	f := New(sanitizer.New().Policy(sanitizer.PolicyRaw)).Type("txt")

	str := string(f.Format(FlagDefault, testStamp, 0, "", []any{"test message", 123}))
	contains(t, str, "2024-01-01", "timestamp")
	contains(t, str, "INFO", "level")
	contains(t, str, `"test message"`, "quoted argument")
	contains(t, str, "123", "numeric argument")
	if !strings.HasSuffix(str, "\n") {
		t.Error("txt records must be newline terminated")
	}
}

func TestFormatJSON(t *testing.T) {
	f := New(sanitizer.New().Policy(sanitizer.PolicyRaw)).Type("json")

	result := unmarshalRecord(t, f.Format(FlagDefault, testStamp, 4, "trace1", []any{"warning", true}))
	eq(t, result["level"], any("WARN"), "level")
	eq(t, result["trace"], any("trace1"), "trace")

	fields := result["fields"].([]any)
	eq(t, fields[0], any("warning"), "field 0")
	eq(t, fields[1], any(true), "field 1")
}

func TestFormatFluentConfiguration(t *testing.T) {
	f := New(sanitizer.New()).
		Type("json").
		TimestampFormat(time.RFC3339).
		ShowLevel(true).
		ShowTimestamp(true)

	str := string(f.Format(0, testStamp, 0, "", []any{"test"}))
	contains(t, str, `"level":"INFO"`, "configured level display")
	contains(t, str, `"time":"2024-01-01T12:00:00Z"`, "configured timestamp format")

	// An empty format string leaves the previous value in place
	f.TimestampFormat("")
	contains(t, string(f.Format(0, testStamp, 0, "", []any{"test"})),
		`"time":"2024-01-01T12:00:00Z"`, "empty format ignored")
}

func TestFormatRaw(t *testing.T) {
	f := New(sanitizer.New().Policy(sanitizer.PolicyRaw)).Type("raw")

	str := string(f.FormatWithOptions("raw", 0, testStamp, 0, "", []any{"raw", "data", 42}))
	eq(t, str, "raw data 42", "space-joined values")
	if strings.HasSuffix(str, "\n") {
		t.Error("raw records must not be newline terminated")
	}
}

func TestFlagRawBypass(t *testing.T) {
	// FlagRaw bypasses both the configured format and the sanitizer
	f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("json")

	eq(t, string(f.Format(FlagRaw, testStamp, 0, "", []any{"forced", "raw"})),
		"forced raw", "format bypass")
	eq(t, string(f.Format(FlagRaw, testStamp, 0, "", []any{"esc\x1b[31m"})),
		"esc\x1b[31m", "sanitizer bypass")
	eq(t, string(f.Format(FlagRaw, testStamp, 0, "", []any{
		[]byte("bytes"), stringerValue{}, errors.New("boom"), 7,
	})), "bytes stringer boom 7", "type handling under FlagRaw")
}

func TestStructuredJSON(t *testing.T) {
	f := New(sanitizer.New().Policy(sanitizer.PolicyJSON)).Type("json")

	fields := map[string]any{"key1": "value1", "key2": 42}
	result := unmarshalRecord(t, f.Format(FlagStructuredJSON|FlagDefault, testStamp, 0, "",
		[]any{"structured message", fields}))

	eq(t, result["message"], any("structured message"), "message key")
	want := map[string]any{"key1": "value1", "key2": float64(42)}
	if !reflect.DeepEqual(result["fields"], want) {
		t.Errorf("fields: got %#v, want %#v", result["fields"], want)
	}

	// The structured branch requires two arguments; otherwise output falls back
	// to the positional fields array
	result = unmarshalRecord(t, f.Format(FlagStructuredJSON|FlagDefault, testStamp, 0, "",
		[]any{"only a message"}))
	if _, ok := result["message"]; ok {
		t.Error("structured branch must not fire with a single argument")
	}
	if _, ok := result["fields"].([]any); !ok {
		t.Errorf("expected positional fields array, got %#v", result["fields"])
	}
}

func TestJSONEscaping(t *testing.T) {
	t.Run("transport escaping applied once under PolicyRaw", func(t *testing.T) {
		f := New(sanitizer.New().Policy(sanitizer.PolicyRaw)).Type("json")
		str := string(f.Format(FlagDefault, testStamp, 0, "", []any{"test\n\r\t\"\\message"}))
		contains(t, str, `test\n\r\t\"\\message`, "escapes")
	})

	t.Run("UTF-8 passthrough", func(t *testing.T) {
		f := New(sanitizer.New()).Type("json")
		in := "héllo 世界 ✓"
		data := f.Format(FlagDefault, testStamp, 0, "", []any{in})
		result := unmarshalRecord(t, data)
		eq(t, result["fields"].([]any)[0], any(in), "round trip")
		notContains(t, string(data), `\u00`, "no per-byte escapes of UTF-8")
	})

	t.Run("content transform precedes transport escaping", func(t *testing.T) {
		f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("json")
		result := unmarshalRecord(t, f.Format(FlagDefault, testStamp, 0, "", []any{"a\x07b"}))
		eq(t, result["fields"].([]any)[0], any("a<07>b"), "hex encoded before escaping")
	})

	t.Run("PolicyJSON double-escapes by design", func(t *testing.T) {
		// The content transform emits literal backslash sequences that the
		// transport layer then escapes again
		f := New(sanitizer.New().Policy(sanitizer.PolicyJSON)).Type("json")
		result := unmarshalRecord(t, f.Format(FlagDefault, testStamp, 0, "", []any{"a\nb"}))
		eq(t, result["fields"].([]any)[0], any(`a\nb`), "double escape")
	})
}

func TestTraceHandling(t *testing.T) {
	t.Run("txt sanitizes and unquotes", func(t *testing.T) {
		f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).
			Type("txt").ShowTimestamp(false).ShowLevel(false)
		// Control sequences in the trace must not reach a terminal verbatim
		str := string(f.Format(0, testStamp, 0, "caller\x1b[31m", []any{"msg"}))
		contains(t, str, "caller<1b>[31m", "sanitized trace")
		notContains(t, str, "\x1b", "raw escape sequence")
		if strings.HasPrefix(str, `"`) {
			t.Errorf("trace must not retain serializer quotes: %q", str)
		}
	})

	t.Run("empty trace is omitted", func(t *testing.T) {
		f := New(sanitizer.New()).Type("json")
		result := unmarshalRecord(t, f.Format(FlagDefault, testStamp, 0, "", []any{"msg"}))
		if _, ok := result["trace"]; ok {
			t.Error("empty trace must not emit a key")
		}
	})
}

func TestFlagResolution(t *testing.T) {
	f := New(sanitizer.New()).Type("txt").ShowTimestamp(true).ShowLevel(true)

	// Non-display flags alone inherit the configured defaults
	str := string(f.Format(FlagStructuredJSON, testStamp, 0, "", []any{"m"}))
	contains(t, str, "2024-01-01", "inherited timestamp")
	contains(t, str, "INFO", "inherited level")

	str = string(f.Format(FlagNoLevel, testStamp, 0, "", []any{"m"}))
	contains(t, str, "2024-01-01", "timestamp retained")
	notContains(t, str, "INFO", "level suppressed")

	str = string(f.Format(FlagNoTimestamp|FlagNoLevel, testStamp, 0, "", []any{"m"}))
	notContains(t, str, "2024-01-01", "timestamp suppressed")
	notContains(t, str, "INFO", "level suppressed")

	// FlagNo* wins over FlagShow* on conflict
	str = string(f.Format(FlagShowLevel|FlagNoLevel, testStamp, 0, "", []any{"m"}))
	notContains(t, str, "INFO", "suppression precedence")

	// Show flags override a disabled default
	off := New(sanitizer.New()).Type("txt").ShowTimestamp(false).ShowLevel(false)
	str = string(off.Format(FlagShowLevel, testStamp, 0, "", []any{"m"}))
	contains(t, str, "INFO", "explicit enable")

	// FormatWithOptions is fully explicit: unset Show bits mean off
	str = string(f.FormatWithOptions("txt", 0, testStamp, 0, "", []any{"m"}))
	notContains(t, str, "INFO", "explicit API ignores defaults")
}

func TestUnknownFormatFallback(t *testing.T) {
	f := New(sanitizer.New()).Type("txt")
	data := f.FormatWithOptions("xml", FlagShowLevel, testStamp, 8, "", []any{"boom"})
	if data == nil {
		t.Fatal("unknown format returned nil")
	}
	contains(t, string(data), "ERROR", "level")
	contains(t, string(data), "boom", "payload")

	// The configured type is normalized on the value paths as well
	unknown := New(sanitizer.New()).Type("xml")
	eq(t, string(unknown.FormatArgs("a b")), `"a b"`, "normalized to txt")
}

func TestAppendValueTypes(t *testing.T) {
	f := New(sanitizer.New()).Type("raw")

	tests := []struct {
		name string
		in   any
		want string
	}{
		{"string", "text", "text"},
		{"bytes", []byte("bytes"), "bytes"},
		{"rune", 'A', "A"},
		{"int", 42, "42"},
		{"int64", int64(64), "64"},
		{"uint", uint(7), "7"},
		{"uint64", uint64(8), "8"},
		{"float32", float32(1.5), "1.5"},
		{"float64", 2.25, "2.25"},
		{"bool", true, "true"},
		{"nil", nil, "nil"},
		{"time", testStamp, "2024-01-01T12:00:00Z"},
		{"error", errors.New("boom"), "boom"},
		{"stringer", stringerValue{}, "stringer"},
		{"complex", map[string]int{"a": 1}, "map[a:1]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq(t, string(f.FormatValue(tt.in)), tt.want, tt.name)
			eq(t, string(f.AppendValue(nil, tt.in)), tt.want, tt.name+" append")
		})
	}
}

func TestFormatArgs(t *testing.T) {
	f := New(sanitizer.New()).Type("raw")
	eq(t, string(f.FormatArgs("a", 1, true)), "a 1 true", "space joined")
	eq(t, string(f.FormatArgs()), "", "no arguments")

	// Append variants extend a caller buffer without a leading separator
	buf := append([]byte(nil), "prefix:"...)
	eq(t, string(f.AppendArgs(buf, "a", "b")), "prefix:a b", "append args")
}

func TestReturnedSliceInvalidation(t *testing.T) {
	f := New(sanitizer.New()).Type("txt").ShowTimestamp(false).ShowLevel(false)

	first := f.Format(0, testStamp, 0, "", []any{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	snapshot := string(first)
	_ = f.Format(0, testStamp, 0, "", []any{"b"})

	if snapshot == string(first) {
		t.Error("buffered Format output must be invalidated by the next buffered call")
	}
}

func TestAppendFormatStable(t *testing.T) {
	f := New(sanitizer.New()).Type("txt").ShowTimestamp(false).ShowLevel(false)

	first := f.AppendFormat(nil, 0, testStamp, 0, "", []any{"first-payload"})
	snapshot := string(first)
	_ = f.Format(0, testStamp, 0, "", []any{"interleaved-buffered-call"})
	second := f.AppendFormat(nil, 0, testStamp, 0, "", []any{"second"})

	eq(t, string(first), snapshot, "caller-owned buffer unaffected by buffered calls")
	eq(t, string(second), "second\n", "subsequent append")
}

func TestFormatterConcurrentAppend(t *testing.T) {
	f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("json")

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range 200 {
				out := f.AppendFormat(nil, FlagDefault, time.Now(), 0, "",
					[]any{"w", id, "i", j, "s", "x\x00y"})
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
		{-1, "LEVEL(-1)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			eq(t, LevelToString(tt.level), tt.expected, "LevelToString")
		})
	}
}

func BenchmarkAppendFormat(b *testing.B) {
	formats := []string{"txt", "json", "raw"}
	args := []any{"request served", "status", 200, "client_ip", "127.0.0.1"}

	for _, format := range formats {
		b.Run(format, func(b *testing.B) {
			f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type(format)
			buf := make([]byte, 0, 512)

			b.ReportAllocs()
			for b.Loop() {
				buf = f.AppendFormat(buf[:0], FlagDefault, testStamp, 0, "", args)
			}
			_ = buf
		})
	}
}
