// FILE: format.go
package log

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// serializer manages the buffered writing of log entries.
type serializer struct {
	buf []byte
}

// newSerializer creates a serializer instance.
func newSerializer() *serializer {
	return &serializer{
		buf: make([]byte, 0, 4096), // Initial reasonable capacity
	}
}

// reset clears the serializer buffer for reuse.
func (s *serializer) reset() {
	s.buf = s.buf[:0]
}

// serialize converts log entries to the configured format, JSON or (default) text.
func (s *serializer) serialize(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	s.reset()

	if format == "json" {
		return s.serializeJSON(flags, timestamp, level, trace, args)
	}
	return s.serializeText(flags, timestamp, level, trace, args)
}

// serializeJSON formats log entries as JSON (time, level, trace, fields).
func (s *serializer) serializeJSON(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	s.buf = append(s.buf, '{')
	needsComma := false

	if flags&FlagShowTimestamp != 0 {
		s.buf = append(s.buf, `"time":"`...)
		s.buf = timestamp.AppendFormat(s.buf, time.RFC3339Nano)
		s.buf = append(s.buf, '"')
		needsComma = true
	}

	if flags&FlagShowLevel != 0 {
		if needsComma {
			s.buf = append(s.buf, ',')
		}
		s.buf = append(s.buf, `"level":"`...)
		s.buf = append(s.buf, levelToString(level)...)
		s.buf = append(s.buf, '"')
		needsComma = true
	}

	if trace != "" {
		if needsComma {
			s.buf = append(s.buf, ',')
		}
		s.buf = append(s.buf, `"trace":"`...)
		s.writeString(trace) // Ensure trace string is escaped
		s.buf = append(s.buf, '"')
		needsComma = true
	}

	if len(args) > 0 {
		if needsComma {
			s.buf = append(s.buf, ',')
		}
		s.buf = append(s.buf, `"fields":[`...)
		for i, arg := range args {
			if i > 0 {
				s.buf = append(s.buf, ',')
			}
			s.writeJSONValue(arg)
		}
		s.buf = append(s.buf, ']')
	}

	s.buf = append(s.buf, '}', '\n')
	return s.buf
}

// serializeText formats log entries as plain text (time, level, trace, fields).
func (s *serializer) serializeText(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	needsSpace := false

	if flags&FlagShowTimestamp != 0 {
		s.buf = timestamp.AppendFormat(s.buf, time.RFC3339Nano)
		needsSpace = true
	}

	if flags&FlagShowLevel != 0 {
		if needsSpace {
			s.buf = append(s.buf, ' ')
		}
		s.buf = append(s.buf, levelToString(level)...)
		needsSpace = true
	}

	if trace != "" {
		if needsSpace {
			s.buf = append(s.buf, ' ')
		}
		s.buf = append(s.buf, trace...)
		needsSpace = true
	}

	for _, arg := range args {
		if needsSpace {
			s.buf = append(s.buf, ' ')
		}
		s.writeTextValue(arg)
		needsSpace = true
	}

	s.buf = append(s.buf, '\n')
	return s.buf
}

// writeTextValue converts any value to its text representation.
func (s *serializer) writeTextValue(v any) {
	switch val := v.(type) {
	case string:
		if len(val) == 0 || strings.ContainsRune(val, ' ') {
			s.buf = append(s.buf, '"')
			s.writeString(val)
			s.buf = append(s.buf, '"')
		} else {
			s.buf = append(s.buf, val...)
		}
	case int:
		s.buf = strconv.AppendInt(s.buf, int64(val), 10)
	case int64:
		s.buf = strconv.AppendInt(s.buf, val, 10)
	case uint:
		s.buf = strconv.AppendUint(s.buf, uint64(val), 10)
	case uint64:
		s.buf = strconv.AppendUint(s.buf, val, 10)
	case float32:
		s.buf = strconv.AppendFloat(s.buf, float64(val), 'f', -1, 32)
	case float64:
		s.buf = strconv.AppendFloat(s.buf, val, 'f', -1, 64)
	case bool:
		s.buf = strconv.AppendBool(s.buf, val)
	case nil:
		s.buf = append(s.buf, "null"...)
	case time.Time:
		s.buf = val.AppendFormat(s.buf, time.RFC3339Nano)
	case error:
		str := val.Error()
		if len(str) == 0 || strings.ContainsRune(str, ' ') {
			s.buf = append(s.buf, '"')
			s.writeString(str)
			s.buf = append(s.buf, '"')
		} else {
			s.buf = append(s.buf, str...)
		}
	case fmt.Stringer:
		str := val.String()
		if len(str) == 0 || strings.ContainsRune(str, ' ') {
			s.buf = append(s.buf, '"')
			s.writeString(str)
			s.buf = append(s.buf, '"')
		} else {
			s.buf = append(s.buf, str...)
		}
	default:
		str := fmt.Sprintf("%+v", val)
		if len(str) == 0 || strings.ContainsRune(str, ' ') {
			s.buf = append(s.buf, '"')
			s.writeString(str)
			s.buf = append(s.buf, '"')
		} else {
			s.buf = append(s.buf, str...)
		}
	}
}

// writeJSONValue converts any value to its JSON representation.
func (s *serializer) writeJSONValue(v any) {
	switch val := v.(type) {
	case string:
		s.buf = append(s.buf, '"')
		s.writeString(val)
		s.buf = append(s.buf, '"')
	case int:
		s.buf = strconv.AppendInt(s.buf, int64(val), 10)
	case int64:
		s.buf = strconv.AppendInt(s.buf, val, 10)
	case uint:
		s.buf = strconv.AppendUint(s.buf, uint64(val), 10)
	case uint64:
		s.buf = strconv.AppendUint(s.buf, val, 10)
	case float32:
		s.buf = strconv.AppendFloat(s.buf, float64(val), 'f', -1, 32)
	case float64:
		s.buf = strconv.AppendFloat(s.buf, val, 'f', -1, 64)
	case bool:
		s.buf = strconv.AppendBool(s.buf, val)
	case nil:
		s.buf = append(s.buf, "null"...)
	case time.Time:
		s.buf = append(s.buf, '"')
		s.buf = val.AppendFormat(s.buf, time.RFC3339Nano)
		s.buf = append(s.buf, '"')
	case error:
		s.buf = append(s.buf, '"')
		s.writeString(val.Error())
		s.buf = append(s.buf, '"')
	case fmt.Stringer:
		s.buf = append(s.buf, '"')
		s.writeString(val.String())
		s.buf = append(s.buf, '"')
	default:
		s.buf = append(s.buf, '"')
		s.writeString(fmt.Sprintf("%+v", val))
		s.buf = append(s.buf, '"')
	}
}

// Update the levelToString function to include the new heartbeat levels
func levelToString(level int64) string {
	switch level {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelProc:
		return "PROC"
	case LevelDisk:
		return "DISK"
	case LevelSys:
		return "SYS"
	default:
		return fmt.Sprintf("LEVEL(%d)", level)
	}
}

// writeString appends a string to the buffer, escaping JSON special characters.
func (s *serializer) writeString(str string) {
	lenStr := len(str)
	for i := 0; i < lenStr; {
		if c := str[i]; c < ' ' || c == '"' || c == '\\' {
			switch c {
			case '\\', '"':
				s.buf = append(s.buf, '\\', c)
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
				s.buf = append(s.buf, `\u00`...)
				s.buf = append(s.buf, hexChars[c>>4], hexChars[c&0xF])
			}
			i++
		} else {
			start := i
			for i < lenStr && str[i] >= ' ' && str[i] != '"' && str[i] != '\\' {
				i++
			}
			s.buf = append(s.buf, str[start:i]...)
		}
	}
}

const hexChars = "0123456789abcdef"