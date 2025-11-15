// FILE: lixenwraith/log/sanitizer/sanitizer.go
// Package sanitizer provides a fluent and composable interface for sanitizing
// strings based on configurable rules using bitwise filter flags and transforms.
package sanitizer

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
)

// Filter flags for character matching
const (
	FilterNonPrintable uint64 = 1 << iota // Matches runes not classified as printable by strconv.IsPrint
	FilterControl                         // Matches control characters (unicode.IsControl)
	FilterWhitespace                      // Matches whitespace characters (unicode.IsSpace)
	FilterShellSpecial                    // Matches common shell metacharacters: '`', '$', ';', '|', '&', '>', '<', '(', ')', '#'
)

// Transform flags for character transformation
const (
	TransformStrip      uint64 = 1 << iota // Removes the character
	TransformHexEncode                     // Encodes the character's UTF-8 bytes as "<XXYY>"
	TransformJSONEscape                    // Escapes the character with JSON-style backslashes (e.g., '\n', '\u0000')
)

// PolicyPreset defines pre-configured sanitization policies
type PolicyPreset string

const (
	PolicyRaw   PolicyPreset = "raw"   // Raw is a no-op (passthrough)
	PolicyJSON  PolicyPreset = "json"  // Policy for sanitizing strings to be embedded in JSON
	PolicyTxt   PolicyPreset = "txt"   // Policy for sanitizing text written to log files
	PolicyShell PolicyPreset = "shell" // Policy for sanitizing arguments passed to shell commands
)

// rule represents a single sanitization rule
type rule struct {
	filter    uint64
	transform uint64
}

// policyRules contains pre-configured rules for each policy
var policyRules = map[PolicyPreset][]rule{
	PolicyRaw:   {},
	PolicyTxt:   {{filter: FilterNonPrintable, transform: TransformHexEncode}},
	PolicyJSON:  {{filter: FilterControl, transform: TransformJSONEscape}},
	PolicyShell: {{filter: FilterShellSpecial | FilterWhitespace, transform: TransformStrip}},
}

// filterCheckers maps individual filter flags to their check functions
var filterCheckers = map[uint64]func(rune) bool{
	FilterNonPrintable: func(r rune) bool { return !strconv.IsPrint(r) },
	FilterControl:      unicode.IsControl,
	FilterWhitespace:   unicode.IsSpace,
	FilterShellSpecial: func(r rune) bool {
		switch r {
		case '`', '$', ';', '|', '&', '>', '<', '(', ')', '#':
			return true
		}
		return false
	},
}

// Sanitizer provides chainable text sanitization
type Sanitizer struct {
	rules []rule
	buf   []byte
}

// New creates a new Sanitizer instance
func New() *Sanitizer {
	return &Sanitizer{
		rules: []rule{},
		buf:   make([]byte, 0, 256),
	}
}

// Rule adds a custom rule to the sanitizer (appended, earliest rule applies first)
func (s *Sanitizer) Rule(filter uint64, transform uint64) *Sanitizer {
	// Append rule in natural order
	s.rules = append(s.rules, rule{filter: filter, transform: transform})
	return s
}

// Policy applies a pre-configured policy to the sanitizer (appended)
func (s *Sanitizer) Policy(preset PolicyPreset) *Sanitizer {
	if rules, ok := policyRules[preset]; ok {
		s.rules = append(s.rules, rules...)
	}
	return s
}

// Sanitize applies all configured rules to the input string
func (s *Sanitizer) Sanitize(data string) string {
	// Reset buffer
	s.buf = s.buf[:0]

	// Process each rune
	for _, r := range data {
		matched := false
		// Check rules in order (first match wins)
		for _, rl := range s.rules {
			if matchesFilter(r, rl.filter) {
				applyTransform(&s.buf, r, rl.transform)
				matched = true
				break
			}
		}
		// If no rule matched, append original rune
		if !matched {
			s.buf = utf8.AppendRune(s.buf, r)
		}
	}

	return string(s.buf)
}

// matchesFilter checks if a rune matches any filter in the mask
func matchesFilter(r rune, filterMask uint64) bool {
	for flag, checker := range filterCheckers {
		if (filterMask&flag) != 0 && checker(r) {
			return true
		}
	}
	return false
}

// applyTransform applies the specified transform to the buffer
func applyTransform(buf *[]byte, r rune, transformMask uint64) {
	switch {
	case (transformMask & TransformStrip) != 0:
		// Do nothing (strip)

	case (transformMask & TransformHexEncode) != 0:
		var runeBytes [utf8.UTFMax]byte
		n := utf8.EncodeRune(runeBytes[:], r)
		*buf = append(*buf, '<')
		*buf = append(*buf, hex.EncodeToString(runeBytes[:n])...)
		*buf = append(*buf, '>')

	case (transformMask & TransformJSONEscape) != 0:
		switch r {
		case '\n':
			*buf = append(*buf, '\\', 'n')
		case '\r':
			*buf = append(*buf, '\\', 'r')
		case '\t':
			*buf = append(*buf, '\\', 't')
		case '\b':
			*buf = append(*buf, '\\', 'b')
		case '\f':
			*buf = append(*buf, '\\', 'f')
		case '"':
			*buf = append(*buf, '\\', '"')
		case '\\':
			*buf = append(*buf, '\\', '\\')
		default:
			if r < 0x20 || r == 0x7f {
				*buf = append(*buf, fmt.Sprintf("\\u%04x", r)...)
			} else {
				*buf = utf8.AppendRune(*buf, r)
			}
		}
	}
}

// Serializer implements format-specific output behaviors
type Serializer struct {
	format    string
	sanitizer *Sanitizer
}

// NewSerializer creates a handler with format-specific behavior
func NewSerializer(format string, san *Sanitizer) *Serializer {
	return &Serializer{
		format:    format,
		sanitizer: san,
	}
}

// WriteString writes a string with format-specific handling
func (se *Serializer) WriteString(buf *[]byte, s string) {
	switch se.format {
	case "raw":
		*buf = append(*buf, se.sanitizer.Sanitize(s)...)

	case "txt":
		sanitized := se.sanitizer.Sanitize(s)
		if se.NeedsQuotes(sanitized) {
			*buf = append(*buf, '"')
			for i := 0; i < len(sanitized); i++ {
				if sanitized[i] == '"' || sanitized[i] == '\\' {
					*buf = append(*buf, '\\')
				}
				*buf = append(*buf, sanitized[i])
			}
			*buf = append(*buf, '"')
		} else {
			*buf = append(*buf, sanitized...)
		}

	case "json":
		*buf = append(*buf, '"')
		// Direct JSON escaping
		for i := 0; i < len(s); {
			c := s[i]
			if c >= ' ' && c != '"' && c != '\\' && c < 0x7f {
				start := i
				for i < len(s) && s[i] >= ' ' && s[i] != '"' && s[i] != '\\' && s[i] < 0x7f {
					i++
				}
				*buf = append(*buf, s[start:i]...)
			} else {
				switch c {
				case '\\', '"':
					*buf = append(*buf, '\\', c)
				case '\n':
					*buf = append(*buf, '\\', 'n')
				case '\r':
					*buf = append(*buf, '\\', 'r')
				case '\t':
					*buf = append(*buf, '\\', 't')
				case '\b':
					*buf = append(*buf, '\\', 'b')
				case '\f':
					*buf = append(*buf, '\\', 'f')
				default:
					*buf = append(*buf, fmt.Sprintf("\\u%04x", c)...)
				}
				i++
			}
		}
		*buf = append(*buf, '"')
	}
}

// WriteNumber writes a number value
func (se *Serializer) WriteNumber(buf *[]byte, n string) {
	*buf = append(*buf, n...)
}

// WriteBool writes a boolean value
func (se *Serializer) WriteBool(buf *[]byte, b bool) {
	*buf = strconv.AppendBool(*buf, b)
}

// WriteNil writes a nil value
func (se *Serializer) WriteNil(buf *[]byte) {
	switch se.format {
	case "raw":
		*buf = append(*buf, "nil"...)
	default:
		*buf = append(*buf, "null"...)
	}
}

// WriteComplex writes complex types
func (se *Serializer) WriteComplex(buf *[]byte, v any) {
	switch se.format {
	// For debugging
	case "raw":
		var b bytes.Buffer
		dumper := &spew.ConfigState{
			Indent:                  " ",
			MaxDepth:                10,
			DisablePointerAddresses: true,
			DisableCapacities:       true,
			SortKeys:                true,
		}
		dumper.Fdump(&b, v)
		*buf = append(*buf, bytes.TrimSpace(b.Bytes())...)

	default:
		str := fmt.Sprintf("%+v", v)
		se.WriteString(buf, str)
	}
}

// NeedsQuotes determines if quoting is needed
func (se *Serializer) NeedsQuotes(s string) bool {
	switch se.format {
	case "json":
		return true
	case "txt":
		if len(s) == 0 {
			return true
		}
		for _, r := range s {
			if unicode.IsSpace(r) {
				return true
			}
			switch r {
			case '"', '\'', '\\', '$', '`', '!', '&', '|', ';',
				'(', ')', '<', '>', '*', '?', '[', ']', '{', '}',
				'~', '#', '%', '=', '\n', '\r', '\t':
				return true
			}
			if !unicode.IsPrint(r) {
				return true
			}
		}
		return false
	default:
		return false
	}
}