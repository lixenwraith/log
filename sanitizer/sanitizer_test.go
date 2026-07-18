package sanitizer

import (
	"strings"
	"sync"
	"testing"
)

func eq[T comparable](tb testing.TB, got, want T, ctx string) {
	tb.Helper()
	if got != want {
		tb.Errorf("%s: got %#v, want %#v", ctx, got, want)
	}
}

func TestNewSanitizer(t *testing.T) {
	// No rules configured means full passthrough
	s := New()
	in := "abc\x00xyz"
	eq(t, s.Sanitize(in), in, "default passthrough")
}

func TestSingleRule(t *testing.T) {
	tests := []struct {
		name      string
		sanitizer *Sanitizer
		in, want  string
	}{
		{"strip non-printable", New().Rule(FilterNonPrintable, TransformStrip), "a\x00b", "ab"},
		{"strip non-printable run", New().Rule(FilterNonPrintable, TransformStrip), "test\x01\x02\x03", "test"},
		{"hex encode non-printable", New().Rule(FilterNonPrintable, TransformHexEncode), "a\x00b", "a<00>b"},
		{"hex encode bell and tab", New().Rule(FilterNonPrintable, TransformHexEncode), "bell\x07tab\x09", "bell<07>tab<09>"},
		{"json escape newline", New().Rule(FilterControl, TransformJSONEscape), "line1\nline2", `line1\nline2`},
		{"json escape tab", New().Rule(FilterControl, TransformJSONEscape), "tab\there", `tab\there`},
		{"json escape nul", New().Rule(FilterControl, TransformJSONEscape), "null\x00byte", `null\u0000byte`},
		{"strip whitespace", New().Rule(FilterWhitespace, TransformStrip), "no spaces here", "nospaceshere"},
		{"strip tabs", New().Rule(FilterWhitespace, TransformStrip), "tabs\t\tgone", "tabsgone"},
		{"strip shell semicolon", New().Rule(FilterShellSpecial, TransformStrip), "cmd; echo test", "cmd echo test"},
		{"strip shell pipe", New().Rule(FilterShellSpecial, TransformStrip), "no | pipes", "no  pipes"},
		{"strip shell dollar", New().Rule(FilterShellSpecial, TransformStrip), "$var", "var"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq(t, tt.sanitizer.Sanitize(tt.in), tt.want, tt.name)
		})
	}
}

func TestRuleFunc(t *testing.T) {
	// Predicate rules take priority over filter evaluation within the same rule
	s := New().RuleFunc(func(r rune) bool { return r == 'x' }, TransformStrip)
	eq(t, s.Sanitize("axbxc"), "abc", "predicate strip")
	eq(t, s.Sanitize("clean"), "clean", "predicate miss")

	s = New().RuleFunc(func(r rune) bool { return r > 0x7f }, TransformHexEncode)
	eq(t, s.Sanitize("a√b"), "a<e2889a>b", "predicate hex encode")
}

func TestPolicy(t *testing.T) {
	tests := []struct {
		name     string
		policy   PolicyPreset
		in, want string
	}{
		{"txt control", PolicyTxt, "hello\x07world", "hello<07>world"},
		{"txt clean", PolicyTxt, "clean text", "clean text"},
		// Tab is non-printable per strconv.IsPrint and is encoded like any control byte
		{"txt tab", PolicyTxt, "col1\tcol2", "col1<09>col2"},
		{"json newline", PolicyJSON, "line1\nline2", `line1\nline2`},
		{"json tab", PolicyJSON, "\ttab", `\ttab`},
		{"shell semicolon", PolicyShell, "cmd; echo", "cmdecho"},
		{"shell whitespace", PolicyShell, "no spaces", "nospaces"},
		{"raw passthrough", PolicyRaw, "a\x00b", "a\x00b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eq(t, New().Policy(tt.policy).Sanitize(tt.in), tt.want, tt.name)
		})
	}

	t.Run("unknown policy is a no-op", func(t *testing.T) {
		s := New().Policy(PolicyPreset("bogus"))
		eq(t, s.Sanitize("a\x00b"), "a\x00b", "unknown preset")
	})
}

func TestPolicyShellExtended(t *testing.T) {
	s := New().Policy(PolicyShell)
	eq(t, s.Sanitize(`a'b"c`), "abc", "quotes")
	eq(t, s.Sanitize(`a\b`), "ab", "backslash")
	eq(t, s.Sanitize("file*?"), "file", "glob")
	eq(t, s.Sanitize("rm -rf *"), "rm-rf", "whitespace and glob")
	eq(t, s.Sanitize("a\x00\x1bb"), "ab", "control")
	eq(t, s.Sanitize("a{b}c[d]e~f!g"), "abcdefg", "braces, brackets, tilde, bang")
}

func TestRuleOrdering(t *testing.T) {
	t.Run("policy precedes later rules", func(t *testing.T) {
		// Rules append in call order and the first match wins, so a Policy
		// registered first shadows overlapping custom rules
		s := New().Policy(PolicyTxt).Rule(FilterControl, TransformStrip)
		eq(t, s.Sanitize("a\x07b\x00c"), "a<07>b<00>c", "policy wins")
	})

	t.Run("first rule wins", func(t *testing.T) {
		s := New().
			Rule(FilterControl, TransformStrip).
			Rule(FilterControl, TransformHexEncode) // unreachable
		eq(t, s.Sanitize("a\x00b"), "ab", "first rule")
	})

	t.Run("chained distinct filters", func(t *testing.T) {
		s := New().
			Rule(FilterWhitespace, TransformStrip).
			Rule(FilterShellSpecial, TransformHexEncode)
		eq(t, s.Sanitize("cmd; echo hello"), "cmd<3b>echohello", "chained")
	})

	t.Run("policy plus custom rules", func(t *testing.T) {
		s := New().
			Policy(PolicyTxt).
			Rule(FilterControl, TransformStrip).
			Rule(FilterWhitespace, TransformJSONEscape)
		// \x07 and \x7F are non-printable and match PolicyTxt first;
		// the space matches the whitespace rule but JSON-escapes to itself
		eq(t, s.Sanitize("a\x07b c\x7Fd"), "a<07>b c<7f>d", "combined")
	})
}

func TestCompositeFilter(t *testing.T) {
	s := New().Rule(FilterShellSpecial|FilterWhitespace, TransformStrip)
	eq(t, s.Sanitize("cmd; echo hello"), "cmdechohello", "composite mask")
	eq(t, s.Sanitize("no |pipes| no spaces"), "nopipesnospaces", "composite mask")
}

func TestTransformPriority(t *testing.T) {
	// applyTransform evaluates Strip first; only one transform applies per rule
	s := New().Rule(FilterControl, TransformStrip|TransformHexEncode)
	eq(t, s.Sanitize("a\x00b"), "ab", "strip precedence")
}

func TestEdgeCases(t *testing.T) {
	strip := New().Rule(FilterNonPrintable, TransformStrip)
	hex := New().Rule(FilterNonPrintable, TransformHexEncode)

	eq(t, strip.Sanitize(""), "", "empty string")
	eq(t, strip.Sanitize("\x00\x01\x02\x03"), "", "fully stripped")
	eq(t, hex.Sanitize("Hello 世界 ✓"), "Hello 世界 ✓", "printable UTF-8 passthrough")
	// U+0085 (NEL) is one non-printable rune encoded as two UTF-8 bytes
	eq(t, hex.Sanitize("line1\u0085line2"), "line1<c285>line2", "multi-byte control")
}

func TestHexMarkerEscaping(t *testing.T) {
	s := New().Policy(PolicyTxt)
	eq(t, s.Sanitize("a\x00b"), "a<00>b", "actual NUL")
	// Literal '<' is encoded so input cannot forge a marker
	eq(t, s.Sanitize("a<00>b"), "a<3c>00>b", "literal marker text")
}

func TestSanitizeCleanFastPath(t *testing.T) {
	s := New().Policy(PolicyTxt)
	in := "clean ascii text"
	eq(t, s.Sanitize(in), in, "unchanged")
	if n := testing.AllocsPerRun(100, func() { _ = s.Sanitize(in) }); n != 0 {
		t.Errorf("clean input allocated %v times, want 0", n)
	}
}

func TestAppendSanitize(t *testing.T) {
	s := New().Policy(PolicyTxt)
	buf := append([]byte(nil), "prefix:"...)
	buf = s.AppendSanitize(buf, "a\x00b")
	eq(t, string(buf), "prefix:a<00>b", "append with rules")

	// No rules configured appends verbatim
	buf = append([]byte(nil), "prefix:"...)
	buf = New().AppendSanitize(buf, "a\x00b")
	eq(t, string(buf), "prefix:a\x00b", "append passthrough")
}

func TestSanitizerConcurrent(t *testing.T) {
	s := New().Policy(PolicyTxt)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 500 {
				// Errorf is goroutine-safe; Fatal variants are not
				if got := s.Sanitize("a\x00b\x07c"); got != "a<00>b<07>c" {
					t.Errorf("concurrent Sanitize: got %q", got)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestSerializerWriteString(t *testing.T) {
	t.Run("raw applies sanitizer", func(t *testing.T) {
		se := NewSerializer("raw", New().Rule(FilterNonPrintable, TransformHexEncode))
		var buf []byte
		se.WriteString(&buf, "test\x00data")
		eq(t, string(buf), "test<00>data", "raw")
	})

	t.Run("txt quotes conditionally", func(t *testing.T) {
		se := NewSerializer("txt", New())
		var buf []byte
		se.WriteString(&buf, "hello world")
		eq(t, string(buf), `"hello world"`, "quoted")

		buf = nil
		se.WriteString(&buf, "nospace")
		eq(t, string(buf), "nospace", "unquoted")

		buf = nil
		se.WriteString(&buf, `has"quote`)
		eq(t, string(buf), `"has\"quote"`, "escaped quote")
	})

	t.Run("json escapes transport characters", func(t *testing.T) {
		se := NewSerializer("json", New())
		var buf []byte
		se.WriteString(&buf, "line1\nline2\t\"quoted\"")
		eq(t, string(buf), `"line1\nline2\t\"quoted\""`, "escapes")

		buf = nil
		se.WriteString(&buf, "null\x00byte")
		eq(t, string(buf), `"null\u0000byte"`, "control escape")

		buf = nil
		se.WriteString(&buf, "héllo 世界")
		eq(t, string(buf), `"héllo 世界"`, "UTF-8 passthrough")
	})

	t.Run("json applies sanitizer before escaping", func(t *testing.T) {
		se := NewSerializer("json", New().Policy(PolicyTxt))
		var buf []byte
		se.WriteString(&buf, "a\x00b")
		eq(t, string(buf), `"a<00>b"`, "layered")
	})
}

func TestSerializerScalars(t *testing.T) {
	san := New()

	t.Run("numbers and booleans", func(t *testing.T) {
		se := NewSerializer("json", san)
		var buf []byte
		se.WriteNumber(&buf, "42")
		se.WriteBool(&buf, true)
		se.WriteBool(&buf, false)
		eq(t, string(buf), "42truefalse", "scalars are unquoted")
	})

	t.Run("nil per format", func(t *testing.T) {
		var buf []byte
		NewSerializer("raw", san).WriteNil(&buf)
		eq(t, string(buf), "nil", "raw nil")

		buf = nil
		NewSerializer("json", san).WriteNil(&buf)
		eq(t, string(buf), "null", "json nil")

		buf = nil
		NewSerializer("txt", san).WriteNil(&buf)
		eq(t, string(buf), "null", "txt nil")
	})

	t.Run("complex values", func(t *testing.T) {
		var buf []byte
		NewSerializer("raw", san).WriteComplex(&buf, map[string]int{"a": 1})
		eq(t, string(buf), "map[a:1]", "map formatting")
	})
}

func TestNeedsQuotes(t *testing.T) {
	tests := []struct {
		format string
		in     string
		want   bool
	}{
		{"json", "anything", true},
		{"raw", "anything", false},
		{"txt", "", true},
		{"txt", "plain", false},
		{"txt", "has space", true},
		{"txt", "semi;colon", true},
		{"txt", "pipe|char", true},
		{"txt", "brace{x}", true},
		{"txt", "percent%", true},
		{"txt", "equals=", true},
		{"txt", "ctrl\x01", true},
		{"txt", "dash-underscore_", false},
	}

	for _, tt := range tests {
		se := NewSerializer(tt.format, New())
		if got := se.NeedsQuotes(tt.in); got != tt.want {
			t.Errorf("NeedsQuotes(%s, %q) = %v, want %v", tt.format, tt.in, got, tt.want)
		}
	}
}

func BenchmarkSanitizer(b *testing.B) {
	input := strings.Repeat("normal text\x00\n\t", 100)

	benchmarks := []struct {
		name      string
		sanitizer *Sanitizer
	}{
		{"Passthrough", New()},
		{"SingleRule", New().Rule(FilterNonPrintable, TransformHexEncode)},
		{"Policy", New().Policy(PolicyTxt)},
		{"Complex", New().
			Policy(PolicyTxt).
			Rule(FilterControl, TransformStrip).
			Rule(FilterWhitespace, TransformJSONEscape)},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = bm.sanitizer.Sanitize(input)
			}
		})
	}
}

func BenchmarkSanitizerClean(b *testing.B) {
	s := New().Policy(PolicyTxt)
	input := strings.Repeat("clean ascii text ", 100)

	b.ReportAllocs()
	for b.Loop() {
		_ = s.Sanitize(input)
	}
}
