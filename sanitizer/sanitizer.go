// Package sanitizer provides a fluent and composable interface for sanitizing
// strings based on configurable rules using bitwise filter flags and transforms.
//
// Concurrency contract: a Sanitizer is immutable after configuration.
// Configure via Rule/RuleFunc/Policy before sharing; Sanitize and
// AppendSanitize are then safe for concurrent use. Serializer is stateless
// and inherits the same contract.
package sanitizer

import (
	"encoding/hex"
	"encoding/json"
	"encoding/json/jsontext"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Filter flags for character matching
const (
	FilterNonPrintable uint64 = 1 << iota // Matches runes not classified as printable by strconv.IsPrint
	FilterControl                         // Matches control characters (unicode.IsControl)
	FilterWhitespace                      // Matches whitespace characters (unicode.IsSpace)
	FilterShellSpecial                    // Matches shell metacharacters including quotes, backslash, globs, and: '`', '$', ';', '|', '&', '>', '<', '(', ')', '#'
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
	PolicyRaw  PolicyPreset = "raw"  // Raw is a no-op (passthrough)
	PolicyJSON PolicyPreset = "json" // Policy for sanitizing strings to be embedded in JSON
	PolicyTxt  PolicyPreset = "txt"  // Policy for sanitizing text written to log files
	// PolicyShell strips shell metacharacters, whitespace, and control characters. NOT sufficient for safe shell construction. Pass arguments via exec argv instead.
	PolicyShell PolicyPreset = "shell" // Lossy text filter; not shell quoting
)

// rule represents a single sanitization rule
type rule struct {
	fn        func(rune) bool // predicate rules (RuleFunc)
	filter    uint64
	transform uint64
}

func (rl rule) matches(r rune) bool {
	if rl.fn != nil {
		return rl.fn(r)
	}
	return matchesFilter(r, rl.filter)
}

// policyRules contains pre-configured rules for each policy
var policyRules = map[PolicyPreset][]rule{
	PolicyRaw: {},
	PolicyTxt: {
		{fn: func(r rune) bool { return r == '<' }, transform: TransformHexEncode},
		{filter: FilterNonPrintable, transform: TransformHexEncode},
	},
	PolicyJSON:  {{filter: FilterControl, transform: TransformJSONEscape}},
	PolicyShell: {{filter: FilterShellSpecial | FilterWhitespace | FilterControl, transform: TransformStrip}},
}

// Sanitizer provides chainable text sanitization
type Sanitizer struct {
	rules []rule
}

// New creates a new Sanitizer instance
func New() *Sanitizer {
	return &Sanitizer{
		rules: []rule{},
	}
}

// Rule adds a custom rule to the sanitizer (appended, earliest rule applies first)
func (s *Sanitizer) Rule(filter uint64, transform uint64) *Sanitizer {
	// Append rule in natural order
	s.rules = append(s.rules, rule{filter: filter, transform: transform})
	return s
}

// RuleFunc adds a predicate-based rule (appended, earliest rule applies first)
func (s *Sanitizer) RuleFunc(fn func(rune) bool, transform uint64) *Sanitizer {
	s.rules = append(s.rules, rule{fn: fn, transform: transform})
	return s
}

// Policy applies a pre-configured policy (appended)
func (s *Sanitizer) Policy(preset PolicyPreset) *Sanitizer {
	if rules, ok := policyRules[preset]; ok {
		s.rules = append(s.rules, rules...)
	}
	return s
}

// Sanitize applies all configured rules. Returns the input unchanged (no
// allocation) when no rule matches. Safe for concurrent use.
func (s *Sanitizer) Sanitize(data string) string {
	if len(s.rules) == 0 {
		return data
	}
	data = normalizeUTF8(data)
	i := s.firstMatch(data)
	if i < 0 {
		return data
	}
	buf := make([]byte, 0, len(data)+16)
	buf = append(buf, data[:i]...)
	buf = s.appendSanitized(buf, data[i:])
	return string(buf)
}

// AppendSanitize appends the sanitized form of data to dst and returns the extended slice. Safe for concurrent use.
func (s *Sanitizer) AppendSanitize(dst []byte, data string) []byte {
	if len(s.rules) == 0 {
		return append(dst, data...)
	}
	return s.appendSanitized(dst, normalizeUTF8(data))
}

// Repair malformed sequences before removing characters: otherwise deleting
// separators can join invalid bytes into a new control rune or metacharacter.
func normalizeUTF8(data string) string {
	if utf8.ValidString(data) {
		return data
	}
	return strings.ToValidUTF8(data, "\ufffd")
}

// firstMatch returns the byte index of the first rune matching any rule, -1 if none
func (s *Sanitizer) firstMatch(data string) int {
	for i, r := range data {
		for _, rl := range s.rules {
			if rl.matches(r) {
				return i
			}
		}
	}
	return -1
}

func (s *Sanitizer) appendSanitized(dst []byte, data string) []byte {
	start := 0
	for i, r := range data {
		for _, rl := range s.rules { // first match wins
			if rl.matches(r) {
				// A rule without a known transform is a passthrough rule.
				if rl.transform&(TransformStrip|TransformHexEncode|TransformJSONEscape) != 0 {
					dst = append(dst, data[start:i]...)
					applyTransform(&dst, r, rl.transform)
					_, size := utf8.DecodeRuneInString(data[i:])
					start = i + size
				}
				break
			}
		}
	}
	dst = append(dst, data[start:]...)
	return dst
}

// matchesFilter checks if a rune matches any filter in the mask
func matchesFilter(r rune, filterMask uint64) bool {
	if filterMask&FilterNonPrintable != 0 && !strconv.IsPrint(r) ||
		filterMask&FilterControl != 0 && unicode.IsControl(r) ||
		filterMask&FilterWhitespace != 0 && unicode.IsSpace(r) {
		return true
	}
	if filterMask&FilterShellSpecial != 0 {
		switch r {
		case '`', '$', ';', '|', '&', '>', '<', '(', ')', '#',
			'\'', '"', '\\', '*', '?', '[', ']', '{', '}', '~', '!':
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
		*buf = hex.AppendEncode(*buf, runeBytes[:n])
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
			if r < 0x20 || r >= 0x7f && r <= 0x9f {
				*buf = append(*buf, fmt.Sprintf("\\u%04x", r)...)
			} else {
				*buf = utf8.AppendRune(*buf, r)
			}
		}
	}
}

// Serializer implements format-specific output behaviors
type Serializer struct {
	sanitizer *Sanitizer
	format    string
}

// NewSerializer creates a handler with format-specific behavior
func NewSerializer(format string, san *Sanitizer) *Serializer {
	if san == nil {
		san = New()
	}
	if format != "raw" && format != "json" && format != "txt" {
		format = "txt"
	}
	return &Serializer{
		format:    format,
		sanitizer: san,
	}
}

// WriteString writes a string with format-specific handling.
// Layering: the sanitizer runs first as a content transform;
// json transport escaping is always applied last, guaranteeing valid JSON output regardless of policy.
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
		// AppendQuote always produces valid UTF-8 JSON. Its error only reports
		// replacement of malformed UTF-8, which is intentional for log output.
		*buf, _ = jsontext.AppendQuote(*buf, se.sanitizer.Sanitize(s))

	}
}

// WriteNumber writes a number; invalid JSON numbers are represented as strings.
func (se *Serializer) WriteNumber(buf *[]byte, n string) {
	if se.format == "json" && (strings.TrimSpace(n) != n || len(n) == 0 || n[0] != '-' && (n[0] < '0' || n[0] > '9') || !json.Valid([]byte(n))) {
		se.WriteString(buf, n)
		return
	}
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
	// fmt recursively traverses maps and slices without cycle detection. Bound
	// that traversal before calling it; scalar and Stringer paths are separate.
	budget := 10000
	if !boundedValue(reflect.ValueOf(v), 0, &budget) {
		se.WriteString(buf, "<cyclic or oversized value>")
		return
	}
	str := fmt.Sprintf("%+v", v)
	se.WriteString(buf, str)
}

// boundedValue limits depth and total traversal work without allocating a
// visited map. A cycle necessarily exceeds the depth bound. Application-defined
// formatting/marshal methods remain responsible for their own termination.
func boundedValue(v reflect.Value, depth int, budget *int) bool {
	*budget -= 1
	if depth > 64 || *budget < 0 {
		return false
	}
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		return v.IsNil() || boundedValue(v.Elem(), depth+1, budget)
	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if !boundedValue(iter.Key(), depth+1, budget) || !boundedValue(iter.Value(), depth+1, budget) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !boundedValue(v.Index(i), depth+1, budget) {
				return false
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !boundedValue(v.Field(i), depth+1, budget) {
				return false
			}
		}
	}
	return true
}

// WriteJSON marshals v with encoding/json semantics, then applies the content
// policy to every JSON string (including nested keys). On error buf is unchanged.
func (se *Serializer) WriteJSON(buf *[]byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if len(se.sanitizer.rules) == 0 {
		*buf = append(*buf, data...)
		return nil
	}
	// Marshal has already validated the JSON. Only string tokens need rewriting;
	// copying the other bytes preserves numbers, booleans, and container shapes.
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] != '"' {
			continue
		}
		*buf = append(*buf, data[start:i]...)
		end := i + 1
		for data[end] != '"' {
			if data[end] == '\\' {
				end++
			}
			end++
		}
		var value string
		_ = json.Unmarshal(data[i:end+1], &value)
		quoted := Serializer{format: "json", sanitizer: se.sanitizer}
		quoted.WriteString(buf, value)
		i, start = end, end+1
	}
	*buf = append(*buf, data[start:]...)
	return nil
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
