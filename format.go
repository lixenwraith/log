// FILE: lixenwraith/log/format.go
package log

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/davecgh/go-spew/spew"
)

// serializer manages the buffered writing of log entries
type serializer struct {
	buf             []byte
	timestampFormat string
}

// newSerializer creates a serializer instance
func newSerializer() *serializer {
	return &serializer{
		buf:             make([]byte, 0, 4096), // Initial reasonable capacity
		timestampFormat: time.RFC3339Nano,      // Default until configured
	}
}

// reset clears the serializer buffer for reuse
func (s *serializer) reset() {
	s.buf = s.buf[:0]
}

// serialize converts log entries to the configured format, JSON, raw, or (default) txt
func (s *serializer) serialize(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	s.reset()

	// 1. Prioritize the on-demand flag from Write()
	if flags&FlagRaw != 0 {
		return s.serializeRaw(args)
	}

	// 2. Check for structured JSON flag
	if flags&FlagStructuredJSON != 0 && format == "json" {
		return s.serializeStructuredJSON(flags, timestamp, level, trace, args)
	}

	// 3. Handle the instance-wide configuration setting
	if format == "raw" {
		return s.serializeRaw(args)
	}

	if format == "json" {
		return s.serializeJSON(flags, timestamp, level, trace, args)
	}
	return s.serializeTxt(flags, timestamp, level, trace, args)
}

// serializeRaw formats args as space-separated strings without metadata or newline
// This is used for both format="raw" configuration and Logger.Write() calls
func (s *serializer) serializeRaw(args []any) []byte {
	needsSpace := false

	for _, arg := range args {
		if needsSpace {
			s.buf = append(s.buf, ' ')
		}
		s.writeRawValue(arg)
		needsSpace = true
	}

	// No newline appended for raw format
	return s.buf
}

// writeRawValue converts any value to its raw string representation
// fallback to go-spew/spew with data structure information for types that are not explicitly supported
func (s *serializer) writeRawValue(v any) {
	switch val := v.(type) {
	case string:
		s.appendSanitized(val) // prevent special character corruption
	case rune:
		// Single rune should be sanitized if non-printable
		s.appendSanitizedRune(val)
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
		s.buf = append(s.buf, "nil"...)
	case time.Time:
		s.buf = val.AppendFormat(s.buf, s.timestampFormat)
	case error:
		s.buf = append(s.buf, val.Error()...)
	case fmt.Stringer:
		s.appendSanitized(val.String())
	case []byte:
		s.appendSanitized(string(val)) // prevent special character corruption
	default:
		// For all other types (structs, maps, pointers, arrays, etc.), delegate to spew
		// It is not the intended use of raw logging
		// The output of such cases are structured and have type and size information set by spew
		// Converting to string similar to non-raw logs is not used to avoid binary log corruption
		var b bytes.Buffer

		// Use a custom dumper for log-friendly compact output
		dumper := &spew.ConfigState{
			Indent:                  " ",
			MaxDepth:                10,
			DisablePointerAddresses: true, // Cleaner for logs
			DisableCapacities:       true, // Less noise
			SortKeys:                true, // Consistent map output
		}

		dumper.Fdump(&b, val)

		// Trim trailing new line added by spew
		s.buf = append(s.buf, bytes.TrimSpace(b.Bytes())...)
	}
}

// serializeJSON formats log entries as JSON (time, level, trace, fields)
func (s *serializer) serializeJSON(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	s.buf = append(s.buf, '{')
	needsComma := false

	if flags&FlagShowTimestamp != 0 {
		s.buf = append(s.buf, `"time":"`...)
		s.buf = timestamp.AppendFormat(s.buf, s.timestampFormat)
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

// serializeTxt formats log entries as plain txt (time, level, trace, fields)
func (s *serializer) serializeTxt(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	needsSpace := false

	if flags&FlagShowTimestamp != 0 {
		s.buf = timestamp.AppendFormat(s.buf, s.timestampFormat)
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
		s.writeTxtValue(arg)
		needsSpace = true
	}

	s.buf = append(s.buf, '\n')
	return s.buf
}

// writeTxtValue converts any value to its txt representation
func (s *serializer) writeTxtValue(v any) {
	switch val := v.(type) {
	case string:
		s.appendSanitized(val) // prevent special character corruption
	case rune:
		// Single rune should be sanitized if non-printable
		s.appendSanitizedRune(val)
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
		s.buf = val.AppendFormat(s.buf, s.timestampFormat)
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
			s.appendSanitized(str)
		}
	case []byte:
		s.appendSanitized(string(val)) // prevent special character corruption
	default:
		str := fmt.Sprintf("%+v", val)
		if len(str) == 0 || strings.ContainsRune(str, ' ') {
			s.buf = append(s.buf, '"')
			// Sanitize
			for _, r := range str {
				s.appendSanitizedRune(r)
			}
			s.buf = append(s.buf, '"')
		} else {
			// Sanitize non-quoted complex values
			s.appendSanitized(str)
		}
	}
}

// writeJSONValue converts any value to its JSON representation
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
		s.buf = val.AppendFormat(s.buf, s.timestampFormat)
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

// serializeStructuredJSON formats log entries as structured JSON with proper field marshaling
func (s *serializer) serializeStructuredJSON(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	// Validate args structure
	if len(args) < 2 {
		// Fallback to regular JSON if args are malformed
		return s.serializeJSON(flags, timestamp, level, trace, args)
	}

	message, ok := args[0].(string)
	if !ok {
		// Fallback if message is not a string
		return s.serializeJSON(flags, timestamp, level, trace, args)
	}

	fields, ok := args[1].(map[string]any)
	if !ok {
		// Fallback if fields is not a map
		return s.serializeJSON(flags, timestamp, level, trace, args)
	}

	s.buf = append(s.buf, '{')
	needsComma := false

	// Add timestamp
	if flags&FlagShowTimestamp != 0 {
		s.buf = append(s.buf, `"time":"`...)
		s.buf = timestamp.AppendFormat(s.buf, s.timestampFormat)
		s.buf = append(s.buf, '"')
		needsComma = true
	}

	// Add level
	if flags&FlagShowLevel != 0 {
		if needsComma {
			s.buf = append(s.buf, ',')
		}
		s.buf = append(s.buf, `"level":"`...)
		s.buf = append(s.buf, levelToString(level)...)
		s.buf = append(s.buf, '"')
		needsComma = true
	}

	// Add message
	if needsComma {
		s.buf = append(s.buf, ',')
	}
	s.buf = append(s.buf, `"message":"`...)
	s.writeString(message)
	s.buf = append(s.buf, '"')

	// Add trace if present
	if trace != "" {
		s.buf = append(s.buf, ',')
		s.buf = append(s.buf, `"trace":"`...)
		s.writeString(trace)
		s.buf = append(s.buf, '"')
	}

	// Marshal fields using encoding/json
	if len(fields) > 0 {
		s.buf = append(s.buf, ',')
		s.buf = append(s.buf, `"fields":`...)

		// Use json.Marshal for proper encoding
		marshaledFields, err := json.Marshal(fields)
		if err != nil {
			// SECURITY: Log marshaling error as a string to prevent log injection
			s.buf = append(s.buf, `{"_marshal_error":"`...)
			s.writeString(err.Error())
			s.buf = append(s.buf, `"}`...)
		} else {
			s.buf = append(s.buf, marshaledFields...)
		}
	}

	s.buf = append(s.buf, '}', '\n')
	return s.buf
}

// appendSanitized sanitizes a string by replacing non-printable runes with their hex representation
func (s *serializer) appendSanitized(data string) {
	var builder strings.Builder
	builder.Grow(len(data)) // Pre-allocate for efficiency

	for _, r := range data {
		// Use the standard library's definition of a printable character
		// This correctly handles Unicode, including high-bit characters like '│' and '世界'
		if strconv.IsPrint(r) {
			builder.WriteRune(r)
		} else {
			// For non-printable runes, encode them safely in a <hex> format
			// This handles multi-byte control characters correctly
			var runeBytes [utf8.UTFMax]byte
			n := utf8.EncodeRune(runeBytes[:], r)
			builder.WriteString("<")
			builder.WriteString(hex.EncodeToString(runeBytes[:n]))
			builder.WriteString(">")
		}
	}
	s.buf = append(s.buf, builder.String()...)
}

// appendSanitizedRune sanitizes a rune by replacing non-printable rune with its hex representation
func (s *serializer) appendSanitizedRune(data rune) {
	if strconv.IsPrint(data) {
		s.buf = utf8.AppendRune(s.buf, data)
	} else {
		var runeBytes [utf8.UTFMax]byte
		n := utf8.EncodeRune(runeBytes[:], data)
		s.buf = append(s.buf, '<')
		s.buf = append(s.buf, hex.EncodeToString(runeBytes[:n])...)
		s.buf = append(s.buf, '>')
	}
}

// levelToString converts integer level values to string
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

// writeString appends a string to the buffer, escaping JSON special characters
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

// setTimestampFormat updates the cached timestamp format in the serializer
func (s *serializer) setTimestampFormat(format string) {
	if format == "" {
		format = time.RFC3339Nano
	}
	s.timestampFormat = format
}