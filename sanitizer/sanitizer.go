// FILE: lixenwraith/log/sanitizer/sanitizer.go
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

// Mode controls how non-printable characters are handled
type Mode int

// Sanitization modes
const (
	None      Mode = iota // No sanitization
	HexEncode             // Encode as <hex> (current default)
	Strip                 // Remove control characters
	Escape                // JSON-style escaping
)

// Sanitizer provides centralized sanitization logic
type Sanitizer struct {
	mode Mode
	buf  []byte // Reusable buffer
}

func New(mode Mode) *Sanitizer {
	return &Sanitizer{
		mode: mode,
		buf:  make([]byte, 0, 256),
	}
}

func (s *Sanitizer) Reset() {
	s.buf = s.buf[:0]
}

func (s *Sanitizer) Sanitize(data string) string {
	if s.mode == None {
		return data
	}

	s.Reset()

	for _, r := range data {
		if strconv.IsPrint(r) {
			s.buf = utf8.AppendRune(s.buf, r)
			continue
		}

		switch s.mode {
		case HexEncode:
			var runeBytes [utf8.UTFMax]byte
			n := utf8.EncodeRune(runeBytes[:], r)
			s.buf = append(s.buf, '<')
			s.buf = append(s.buf, hex.EncodeToString(runeBytes[:n])...)
			s.buf = append(s.buf, '>')

		case Strip:
			// Skip non-printable
			continue

		case Escape:
			switch r {
			case '\n':
				s.buf = append(s.buf, '\\', 'n')
			case '\r':
				s.buf = append(s.buf, '\\', 'r')
			case '\t':
				s.buf = append(s.buf, '\\', 't')
			case '\b':
				s.buf = append(s.buf, '\\', 'b')
			case '\f':
				s.buf = append(s.buf, '\\', 'f')
			default:
				// Unicode escape for other control chars
				s.buf = append(s.buf, '\\', 'u')
				s.buf = append(s.buf, fmt.Sprintf("%04x", r)...)
			}
		}
	}

	return string(s.buf)
}

// UnifiedHandler implements all format behaviors in a single struct
type UnifiedHandler struct {
	format    string
	sanitizer *Sanitizer
}

func NewUnifiedHandler(format string, san *Sanitizer) *UnifiedHandler {
	return &UnifiedHandler{
		format:    format,
		sanitizer: san,
	}
}

func (h *UnifiedHandler) WriteString(buf *[]byte, s string) {
	switch h.format {
	case "raw":
		*buf = append(*buf, h.sanitizer.Sanitize(s)...)

	case "txt":
		sanitized := h.sanitizer.Sanitize(s)
		if h.NeedsQuotes(sanitized) {
			*buf = append(*buf, '"')
			// Escape quotes within quoted strings
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
		// Direct JSON escaping without pre-sanitization
		for i := 0; i < len(s); {
			c := s[i]
			if c >= ' ' && c != '"' && c != '\\' {
				start := i
				for i < len(s) && s[i] >= ' ' && s[i] != '"' && s[i] != '\\' {
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

func (h *UnifiedHandler) WriteNumber(buf *[]byte, n string) {
	*buf = append(*buf, n...)
}

func (h *UnifiedHandler) WriteBool(buf *[]byte, b bool) {
	*buf = strconv.AppendBool(*buf, b)
}

func (h *UnifiedHandler) WriteNil(buf *[]byte) {
	switch h.format {
	case "raw":
		*buf = append(*buf, "nil"...)
	default: // txt, json
		*buf = append(*buf, "null"...)
	}
}

func (h *UnifiedHandler) WriteComplex(buf *[]byte, v any) {
	switch h.format {
	case "raw":
		// Use spew for complex types in raw mode, DEBUG use
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

	default: // txt, json
		str := fmt.Sprintf("%+v", v)
		h.WriteString(buf, str)
	}
}

func (h *UnifiedHandler) NeedsQuotes(s string) bool {
	switch h.format {
	case "json":
		return true // JSON always quotes
	case "txt":
		// Quote strings that:
		// 1. Are empty
		if len(s) == 0 {
			return true
		}
		for _, r := range s {
			// 2. Contain whitespace (space, tab, newline, etc.)
			if unicode.IsSpace(r) {
				return true
			}
			// 3. Contain shell special characters (POSIX + common extensions)
			switch r {
			case '"', '\'', '\\', '$', '`', '!', '&', '|', ';',
				'(', ')', '<', '>', '*', '?', '[', ']', '{', '}',
				'~', '#', '%', '=', '\n', '\r', '\t':
				return true
			}
			// 4. Non-print
			if !unicode.IsPrint(r) {
				return true
			}
		}
		return false
	default: // raw
		return false
	}
}