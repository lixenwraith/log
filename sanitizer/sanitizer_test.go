package sanitizer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewSanitizer(t *testing.T) {
	// Default passthrough behavior
	s := New()
	input := "abc\x00xyz"
	assert.Equal(t, input, s.Sanitize(input), "default sanitizer should pass through all characters")
}

func TestSingleRule(t *testing.T) {
	t.Run("strip non-printable", func(t *testing.T) {
		s := New().Rule(FilterNonPrintable, TransformStrip)
		assert.Equal(t, "ab", s.Sanitize("a\x00b"))
		assert.Equal(t, "test", s.Sanitize("test\x01\x02\x03"))
	})

	t.Run("hex encode non-printable", func(t *testing.T) {
		s := New().Rule(FilterNonPrintable, TransformHexEncode)
		assert.Equal(t, "a<00>b", s.Sanitize("a\x00b"))
		assert.Equal(t, "bell<07>tab<09>", s.Sanitize("bell\x07tab\x09"))
	})

	t.Run("JSON escape control", func(t *testing.T) {
		s := New().Rule(FilterControl, TransformJSONEscape)
		assert.Equal(t, "line1\\nline2", s.Sanitize("line1\nline2"))
		assert.Equal(t, "tab\\there", s.Sanitize("tab\there"))
		assert.Equal(t, "null\\u0000byte", s.Sanitize("null\x00byte"))
	})

	t.Run("strip whitespace", func(t *testing.T) {
		s := New().Rule(FilterWhitespace, TransformStrip)
		assert.Equal(t, "nospaceshere", s.Sanitize("no spaces here"))
		assert.Equal(t, "tabsgone", s.Sanitize("tabs\t\tgone"))
	})

	t.Run("strip shell special", func(t *testing.T) {
		s := New().Rule(FilterShellSpecial, TransformStrip)
		assert.Equal(t, "cmd echo test", s.Sanitize("cmd; echo test"))
		assert.Equal(t, "no  pipes", s.Sanitize("no | pipes"))
		assert.Equal(t, "var", s.Sanitize("$var"))
	})
}

func TestPolicy(t *testing.T) {
	t.Run("PolicyTxt", func(t *testing.T) {
		s := New().Policy(PolicyTxt)
		assert.Equal(t, "hello<07>world", s.Sanitize("hello\x07world"))
		assert.Equal(t, "clean text", s.Sanitize("clean text"))
	})

	t.Run("PolicyJSON", func(t *testing.T) {
		s := New().Policy(PolicyJSON)
		assert.Equal(t, "line1\\nline2", s.Sanitize("line1\nline2"))
		assert.Equal(t, "\\ttab", s.Sanitize("\ttab"))
	})

	t.Run("PolicyShellArg", func(t *testing.T) {
		s := New().Policy(PolicyShell)
		assert.Equal(t, "cmdecho", s.Sanitize("cmd; echo"))
		assert.Equal(t, "nospaces", s.Sanitize("no spaces"))
	})
}

func TestRulePrecedence(t *testing.T) {
	// With append + forward iteration: Policy is checked before Rule
	s := New().Policy(PolicyTxt).Rule(FilterControl, TransformStrip)

	// \x07 is both control AND non-printable - matches PolicyTxt first
	// \x00 is both control AND non-printable - matches PolicyTxt first
	input := "a\x07b\x00c"
	expected := "a<07>b<00>c" // FIXED: Policy wins now
	result := s.Sanitize(input)

	assert.Equal(t, expected, result,
		"Policy() is now checked before Rule() - non-printable chars get hex encoded")
}

func TestCompositeFilter(t *testing.T) {
	s := New().Rule(FilterShellSpecial|FilterWhitespace, TransformStrip)
	assert.Equal(t, "cmdechohello", s.Sanitize("cmd; echo hello"))
	assert.Equal(t, "nopipesnospaces", s.Sanitize("no |pipes| no spaces"))
}

func TestChaining(t *testing.T) {
	s := New().
		Rule(FilterWhitespace, TransformStrip).
		Rule(FilterShellSpecial, TransformHexEncode)

	// Shell special chars are checked first (prepended), get hex encoded
	// Whitespace rule is second, strips spaces
	assert.Equal(t, "cmd<3b>echohello", s.Sanitize("cmd; echo hello"))
}

func TestMultipleRulesOrder(t *testing.T) {
	// Test that first matching rule wins
	s := New().
		Rule(FilterControl, TransformStrip).
		Rule(FilterControl, TransformHexEncode) // This should never match

	assert.Equal(t, "ab", s.Sanitize("a\x00b"), "first rule should win")
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		s := New().Rule(FilterNonPrintable, TransformStrip)
		assert.Equal(t, "", s.Sanitize(""))
	})

	t.Run("only sanitizable characters", func(t *testing.T) {
		s := New().Rule(FilterNonPrintable, TransformStrip)
		assert.Equal(t, "", s.Sanitize("\x00\x01\x02\x03"))
	})

	t.Run("multi-byte UTF-8", func(t *testing.T) {
		s := New().Rule(FilterNonPrintable, TransformHexEncode)
		input := "Hello 世界 ✓"
		assert.Equal(t, input, s.Sanitize(input), "UTF-8 should pass through")
	})

	t.Run("multi-byte control character", func(t *testing.T) {
		s := New().Rule(FilterNonPrintable, TransformHexEncode)
		// NEL (Next Line) is U+0085, encoded as C2 85 in UTF-8
		assert.Equal(t, "line1<c285>line2", s.Sanitize("line1\u0085line2"))
	})
}

func TestSerializer(t *testing.T) {
	t.Run("raw format with sanitizer", func(t *testing.T) {
		san := New().Rule(FilterNonPrintable, TransformHexEncode)
		handler := NewSerializer("raw", san)

		var buf []byte
		handler.WriteString(&buf, "test\x00data")
		assert.Equal(t, "test<00>data", string(buf))
	})

	t.Run("txt format with quotes", func(t *testing.T) {
		san := New() // No sanitization
		handler := NewSerializer("txt", san)

		var buf []byte
		handler.WriteString(&buf, "hello world")
		assert.Equal(t, `"hello world"`, string(buf))

		buf = nil
		handler.WriteString(&buf, "nospace")
		assert.Equal(t, "nospace", string(buf))
	})

	t.Run("json format escaping", func(t *testing.T) {
		san := New() // JSON handler does its own escaping
		handler := NewSerializer("json", san)

		var buf []byte
		handler.WriteString(&buf, "line1\nline2\t\"quoted\"")
		assert.Equal(t, `"line1\nline2\t\"quoted\""`, string(buf))

		buf = nil
		handler.WriteString(&buf, "null\x00byte")
		assert.Equal(t, `"null\u0000byte"`, string(buf))
	})

	t.Run("complex value handling", func(t *testing.T) {
		san := New()
		handler := NewSerializer("raw", san)

		var buf []byte
		handler.WriteComplex(&buf, map[string]int{"a": 1})
		assert.Contains(t, string(buf), "map[")
	})

	t.Run("nil handling", func(t *testing.T) {
		san := New()

		rawHandler := NewSerializer("raw", san)
		var buf []byte
		rawHandler.WriteNil(&buf)
		assert.Equal(t, "nil", string(buf))

		jsonHandler := NewSerializer("json", san)
		buf = nil
		jsonHandler.WriteNil(&buf)
		assert.Equal(t, "null", string(buf))
	})
}

func TestPolicyWithCustomRules(t *testing.T) {
	s := New().
		Policy(PolicyTxt).
		Rule(FilterControl, TransformStrip).
		Rule(FilterWhitespace, TransformJSONEscape)

	// \x07 is non-printable AND control - matches PolicyTxt first (hex encode)
	// \x7F is non-printable but NOT control - matches PolicyTxt (hex encode)
	input := "a\x07b c\x7Fd"
	result := s.Sanitize(input)

	assert.Equal(t, "a<07>b c<7f>d", result) // FIXED: \x07 now hex encoded
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
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = bm.sanitizer.Sanitize(input)
			}
		})
	}
}

func TestTransformPriority(t *testing.T) {
	// Test that only one transform is applied per rule
	s := New().Rule(FilterControl, TransformStrip|TransformHexEncode)

	// Should strip (first flag checked), not hex encode
	assert.Equal(t, "ab", s.Sanitize("a\x00b"))
}