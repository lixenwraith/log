package sanitizer

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func FuzzSanitize(f *testing.F) {
	for _, s := range []string{"", "plain", "\xff\xfe\n\x80", "<00>\x00", "\u0085\u009b[31m", "世界\u202e", "a'$(id)\\*"} {
		f.Add(s, uint8(0), uint64(FilterControl), uint64(TransformHexEncode))
	}
	f.Fuzz(func(t *testing.T, input string, policy uint8, filter, transform uint64) {
		policies := []PolicyPreset{PolicyRaw, PolicyTxt, PolicyJSON, PolicyShell}
		s := New().Policy(policies[int(policy)%len(policies)]).Rule(filter, transform)
		want := s.Sanitize(input)
		got := s.AppendSanitize([]byte("prefix:"), input)
		if string(got) != "prefix:"+want {
			t.Fatalf("append/string disagreement: %q != %q", got, want)
		}
		if want != s.Sanitize(input) {
			t.Fatal("non-deterministic sanitizer")
		}
		if policy%4 == 3 {
			for _, r := range New().Policy(PolicyShell).Sanitize(input) {
				if unicode.IsControl(r) || unicode.IsSpace(r) || matchesFilter(r, FilterShellSpecial) {
					t.Fatalf("shell filter leaked %U", r)
				}
			}
		}
	})
}

func FuzzSerializerJSON(f *testing.F) {
	for _, s := range []string{"", "\xff\xfe\x00", "a\"\\\nb", "\u0085", "世界"} {
		f.Add(s, uint8(0))
	}
	f.Fuzz(func(t *testing.T, input string, policy uint8) {
		policies := []PolicyPreset{PolicyRaw, PolicyTxt, PolicyJSON, PolicyShell}
		s := New().Policy(policies[int(policy)%len(policies)])
		out := []byte("prefix:")
		NewSerializer("json", s).WriteString(&out, input)
		if !strings.HasPrefix(string(out), "prefix:") {
			t.Fatal("prefix changed")
		}
		data := out[len("prefix:"):]
		if !utf8.Valid(data) || !json.Valid(data) {
			t.Fatalf("invalid JSON string %q", data)
		}
		var got, want string
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		// encoding/json is an independent oracle for malformed UTF-8 replacement.
		oracle, err := json.Marshal(s.Sanitize(input))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(oracle, &want); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("round trip: %q != %q", got, want)
		}
	})
}

func TestSanitizerMalformedUTF8AndTransforms(t *testing.T) {
	for _, input := range []string{"\xff", "\xff\n\xfe", "\n\xff"} {
		s := New().Policy(PolicyJSON)
		eq(t, string(s.AppendSanitize(nil, input)), s.Sanitize(input), "malformed UTF-8 consistency")
	}
	eq(t, New().Rule(FilterControl, 0).Sanitize("a\x00b"), "a\x00b", "zero transform preserves")
	eq(t, New().Policy(PolicyJSON).Sanitize("\u0085\u009b"), `\u0085\u009b`, "C1 controls escaped")
	var out []byte
	NewSerializer("unknown", nil).WriteString(&out, "a b")
	eq(t, string(out), `"a b"`, "nil sanitizer and unknown format")
}

func TestSerializerInvalidNumbers(t *testing.T) {
	for _, input := range []string{"", "NaN", "+Inf", "-Inf", "01", "1,2", "null", "1\n", "1 "} {
		var out []byte
		NewSerializer("json", nil).WriteNumber(&out, input)
		var got string
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatalf("%q must be represented as string: %q (%v)", input, out, err)
		}
		eq(t, got, input, "invalid number round trip")
	}
}
