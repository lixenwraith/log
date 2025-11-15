// FILE: lixenwraith/log/sanitizer/sanitizer_test.go
package sanitizer

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizer(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		mode     Mode
		expected string
	}{
		// None mode tests
		{
			name:     "none mode passes through",
			input:    "hello\x00world\n",
			mode:     None,
			expected: "hello\x00world\n",
		},

		// HexEncode tests
		{
			name:     "hex encode null byte",
			input:    "test\x00data",
			mode:     HexEncode,
			expected: "test<00>data",
		},
		{
			name:     "hex encode control chars",
			input:    "bell\x07tab\x09form\x0c",
			mode:     HexEncode,
			expected: "bell<07>tab<09>form<0c>",
		},
		{
			name:     "hex encode preserves printable",
			input:    "Hello World 123!@#",
			mode:     HexEncode,
			expected: "Hello World 123!@#",
		},
		{
			name:     "hex encode multi-byte control",
			input:    "line1\u0085line2", // NEXT LINE (C2 85)
			mode:     HexEncode,
			expected: "line1<c285>line2",
		},
		{
			name:     "hex encode preserves UTF-8",
			input:    "Hello 世界 ✓",
			mode:     HexEncode,
			expected: "Hello 世界 ✓",
		},

		// Strip tests
		{
			name:     "strip removes control chars",
			input:    "clean\x00\x07\ntxt",
			mode:     Strip,
			expected: "cleantxt",
		},
		{
			name:     "strip preserves spaces",
			input:    "hello world",
			mode:     Strip,
			expected: "hello world",
		},

		// Escape tests
		{
			name:     "escape common control chars",
			input:    "line1\nline2\ttab\rreturn",
			mode:     Escape,
			expected: "line1\\nline2\\ttab\\rreturn",
		},
		{
			name:     "escape unicode control",
			input:    "text\x01\x1f",
			mode:     Escape,
			expected: "text\\u0001\\u001f",
		},
		{
			name:     "escape backspace and form feed",
			input:    "back\bspace form\ffeed",
			mode:     Escape,
			expected: "back\\bspace form\\ffeed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := New(tc.mode)
			result := s.Sanitize(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestUnifiedHandler(t *testing.T) {
	t.Run("raw format", func(t *testing.T) {
		san := New(HexEncode)
		handler := NewUnifiedHandler("raw", san)

		var buf []byte

		// String handling
		handler.WriteString(&buf, "test\x00data")
		assert.Equal(t, "test<00>data", string(buf))

		// Nil handling
		buf = nil
		handler.WriteNil(&buf)
		assert.Equal(t, "nil", string(buf))

		// No quotes needed
		assert.False(t, handler.NeedsQuotes("any string"))
	})

	t.Run("txt format", func(t *testing.T) {
		san := New(HexEncode)
		handler := NewUnifiedHandler("txt", san)

		var buf []byte

		// String with spaces gets quoted
		handler.WriteString(&buf, "hello world")
		assert.Equal(t, `"hello world"`, string(buf))

		// String without spaces unquoted
		buf = nil
		handler.WriteString(&buf, "single")
		assert.Equal(t, "single", string(buf))

		// Nil handling
		buf = nil
		handler.WriteNil(&buf)
		assert.Equal(t, "null", string(buf))

		// Quotes needed for empty or space-containing
		assert.True(t, handler.NeedsQuotes(""))
		assert.True(t, handler.NeedsQuotes("has space"))
		assert.False(t, handler.NeedsQuotes("nospace"))
	})

	t.Run("json format", func(t *testing.T) {
		san := New(Escape) // Not used for JSON, direct escaping
		handler := NewUnifiedHandler("json", san)

		var buf []byte

		// JSON escaping
		handler.WriteString(&buf, "line1\nline2\t\"quoted\"")
		assert.Equal(t, `"line1\nline2\t\"quoted\""`, string(buf))

		// Control char escaping
		buf = nil
		handler.WriteString(&buf, "null\x00byte")
		assert.Equal(t, `"null\u0000byte"`, string(buf))

		// Always quotes
		assert.True(t, handler.NeedsQuotes("anything"))
	})

	t.Run("complex value handling", func(t *testing.T) {
		san := New(HexEncode)

		// Raw uses spew
		rawHandler := NewUnifiedHandler("raw", san)
		var buf []byte
		rawHandler.WriteComplex(&buf, map[string]int{"a": 1})
		assert.Contains(t, string(buf), "map[")

		// Txt/JSON use fmt.Sprintf
		txtHandler := NewUnifiedHandler("txt", san)
		buf = nil
		txtHandler.WriteComplex(&buf, []int{1, 2, 3})
		assert.Contains(t, string(buf), "[1 2 3]")
	})
}

func BenchmarkSanitizer(b *testing.B) {
	input := strings.Repeat("normal text\x00\n\t", 100)

	benchmarks := []struct {
		name string
		mode Mode
	}{
		{"None", None},
		{"HexEncode", HexEncode},
		{"Strip", Strip},
		{"Escape", Escape},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			s := New(bm.mode)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = s.Sanitize(input)
			}
		})
	}
}