package formatter

import (
	"bytes"
	"encoding/json"
	"math"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/lixenwraith/log/sanitizer"
)

func FuzzFormatJSON(f *testing.F) {
	for _, s := range []string{"", "safe", "\xff\n\"\\", "\u0085\x1b[0m", "世界"} {
		f.Add(s, s, s, uint64(0), uint8(0))
	}
	f.Fuzz(func(t *testing.T, value, key, layout string, bits uint64, mode uint8) {
		policies := []sanitizer.PolicyPreset{sanitizer.PolicyRaw, sanitizer.PolicyTxt, sanitizer.PolicyJSON, sanitizer.PolicyShell}
		fm := New(sanitizer.New().Policy(policies[int(mode)%4])).Type("json").TimestampFormat(layout).ContextKeys(key)
		flags := FlagDefault
		args := []any{value, []byte(value), math.Float64frombits(bits), uint32(bits)}
		switch mode % 3 {
		case 1:
			flags |= FlagKV
			args = []any{key, value, "number", math.Float64frombits(bits)}
		case 2:
			flags |= FlagStructuredJSON
			args = []any{value, map[string]any{key: []any{value, math.Float64frombits(bits)}}}
		}
		prefix := []byte("prefix:")
		out := fm.AppendFormatCtx(bytes.Clone(prefix), Context{Tag: value}, flags, testStamp, int64(bits), value, args)
		if !bytes.HasPrefix(out, prefix) {
			t.Fatal("prefix changed")
		}
		data := out[len(prefix):]
		if !utf8.Valid(data) || !json.Valid(data) || bytes.Count(data, []byte{'\n'}) != 1 || data[len(data)-1] != '\n' {
			t.Fatalf("invalid JSON record %q", data)
		}
		if second := fm.AppendFormatCtx(nil, Context{Tag: value}, flags, testStamp, int64(bits), value, args); !bytes.Equal(data, second) {
			t.Fatal("non-deterministic formatter")
		}
	})
}

func FuzzFormatTextPolicy(f *testing.F) {
	for _, s := range []string{"", "\n\r\x1b[31m", "\xff<00>\u0085", "a=b \"c\""} {
		f.Add(s, s, s)
	}
	f.Fuzz(func(t *testing.T, value, key, layout string) {
		fm := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("txt").TimestampFormat(layout).ContextKeys(key, key)
		out := fm.AppendFormatCtx(nil, Context{Tag: value}, FlagDefault|FlagKV, testStamp, 0, value, []any{key, value})
		if !bytes.HasSuffix(out, []byte{'\n'}) {
			t.Fatal("missing terminator")
		}
		for _, r := range string(out[:len(out)-1]) {
			if unicode.IsControl(r) || !unicode.IsPrint(r) {
				t.Fatalf("unsafe rune %U in %q", r, out)
			}
		}
	})
}

type panickingStringer struct{}

func (panickingStringer) String() string { panic("broken Stringer") }

func TestFormatHardening(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		cycle := map[string]any{}
		cycle["self"] = cycle
		for _, v := range []any{math.NaN(), make(chan int), cycle, json.RawMessage(`invalid`)} {
			f := New().Type("json")
			record := unmarshalRecord(t, f.Format(FlagStructuredJSON, testStamp, 0, "", []any{"msg", map[string]any{"bad": v}}))
			if _, ok := record["fields"].(map[string]any)["_marshal_error"].(string); !ok {
				t.Fatalf("missing marshal error: %v", record)
			}
		}
	})
	t.Run("nested sanitization", func(t *testing.T) {
		f := New(sanitizer.New().Policy(sanitizer.PolicyTxt)).Type("json")
		record := unmarshalRecord(t, f.Format(FlagStructuredJSON, testStamp, 0, "", []any{"msg", map[string]any{"k\n": []any{map[string]any{"v": "\x1b[31m"}, 42, true}}}))
		nested := record["fields"].(map[string]any)["k<0a>"].([]any)
		eq(t, nested[0].(map[string]any)["v"], any("<1b>[31m"), "nested content policy")
		eq(t, nested[1], any(float64(42)), "number preserved")
	})
	t.Run("explicit suppression wins", func(t *testing.T) {
		f := New().Type("json")
		record := unmarshalRecord(t, f.FormatWithOptions("json", FlagDefault|FlagNoLevel|FlagNoTimestamp, testStamp, 0, "", nil))
		eq(t, len(record), 0, "suppressed metadata")
	})
	t.Run("bad stringer is printable", func(t *testing.T) {
		for _, flags := range []int64{0, FlagRaw} {
			out := New().Format(flags, testStamp, 0, "", []any{panickingStringer{}})
			if !strings.Contains(string(out), "PANIC") {
				t.Fatalf("missing fmt panic marker: %q", out)
			}
		}
	})
}

func TestCyclicComplexValues(t *testing.T) {
	cycle := map[string]any{}
	cycle["self"] = cycle
	slice := make([]any, 1)
	slice[0] = slice
	for _, value := range []any{cycle, slice} {
		for _, format := range []string{"txt", "json", "raw"} {
			for _, flags := range []int64{0, FlagRaw} {
				output := New().Type(format).Format(flags, testStamp, 0, "", []any{value})
				if !strings.Contains(string(output), "cyclic or oversized value") {
					t.Fatalf("%s: missing cycle marker: %q", format, output)
				}
			}
		}
	}
}
