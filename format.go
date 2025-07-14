// FILE: format.go
package log

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// serializer manages the buffered writing of log entries.
type serializer struct {
	buf             []byte
	timestampFormat string
}

// newSerializer creates a serializer instance.
func newSerializer() *serializer {
	return &serializer{
		buf:             make([]byte, 0, 4096), // Initial reasonable capacity
		timestampFormat: time.RFC3339Nano,      // Default until configured
	}
}

// reset clears the serializer buffer for reuse.
func (s *serializer) reset() {
	s.buf = s.buf[:0]
}

// serialize converts log entries to the configured format, JSON, raw, or (default) text.
func (s *serializer) serialize(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	s.reset()

	// 1. Prioritize the on-demand flag from Write()
	if flags&FlagRaw != 0 {
		return s.serializeRaw(args)
	}

	// 2. Handle the instance-wide configuration setting
	if format == "raw" {
		return s.serializeRaw(args)
	}

	if format == "json" {
		return s.serializeJSON(flags, timestamp, level, trace, args)
	}
	return s.serializeText(flags, timestamp, level, trace, args)
}

// serializeRaw formats args as space-separated strings without metadata or newline.
// This is used for both format="raw" configuration and Logger.Write() calls.
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

// writeRawValue converts any value to its raw string representation.
// fallback to go-spew/spew with data structure information for types that are not explicitly supported.
func (s *serializer) writeRawValue(v any) {
	switch val := v.(type) {
	case string:
		s.buf = append(s.buf, val...)
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
		s.buf = append(s.buf, val.String()...)
	case []byte:
		s.buf = hex.AppendEncode(s.buf, val) // prevent special character corruption
	default:
		// For all other types (structs, maps, pointers, arrays, etc.), delegate to spew.
		// It is not the intended use of raw logging.
		// The output of such cases are structured and have type and size information set by spew.
		// Converting to string similar to non-raw logs is not used to avoid binary log corruption.
		var b bytes.Buffer

		// Use a custom dumper for log-friendly, compact output.
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

// This is the safe, dependency-free replacement for fmt.Sprintf.
func (s *serializer) reflectValue(v reflect.Value) {
	// Safely handle invalid, nil pointer, or nil interface values.
	if !v.IsValid() {
		s.buf = append(s.buf, "nil"...)
		return
	}
	// Dereference pointers and interfaces to get the concrete value.
	// Recurse to handle multiple levels of pointers.
	kind := v.Kind()
	if kind == reflect.Ptr || kind == reflect.Interface {
		if v.IsNil() {
			s.buf = append(s.buf, "nil"...)
			return
		}
		s.reflectValue(v.Elem())
		return
	}

	switch kind {
	case reflect.String:
		s.buf = append(s.buf, v.String()...)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		s.buf = strconv.AppendInt(s.buf, v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		s.buf = strconv.AppendUint(s.buf, v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		s.buf = strconv.AppendFloat(s.buf, v.Float(), 'f', -1, 64)
	case reflect.Bool:
		s.buf = strconv.AppendBool(s.buf, v.Bool())

	case reflect.Slice, reflect.Array:
		// Check if it's a byte slice ([]uint8) and hex-encode it for safety.
		if v.Type().Elem().Kind() == reflect.Uint8 {
			s.buf = append(s.buf, "0x"...)
			s.buf = hex.AppendEncode(s.buf, v.Bytes())
			return
		}
		s.buf = append(s.buf, '[')
		for i := 0; i < v.Len(); i++ {
			if i > 0 {
				s.buf = append(s.buf, ' ')
			}
			s.reflectValue(v.Index(i))
		}
		s.buf = append(s.buf, ']')

	case reflect.Struct:
		s.buf = append(s.buf, '{')
		for i := 0; i < v.NumField(); i++ {
			if !v.Type().Field(i).IsExported() {
				continue // Skip unexported fields
			}
			if i > 0 {
				s.buf = append(s.buf, ' ')
			}
			s.buf = append(s.buf, v.Type().Field(i).Name...)
			s.buf = append(s.buf, ':')
			s.reflectValue(v.Field(i))
		}
		s.buf = append(s.buf, '}')

	case reflect.Map:
		s.buf = append(s.buf, '{')
		for i, key := range v.MapKeys() {
			if i > 0 {
				s.buf = append(s.buf, ' ')
			}
			s.reflectValue(key)
			s.buf = append(s.buf, ':')
			s.reflectValue(v.MapIndex(key))
		}
		s.buf = append(s.buf, '}')

	default:
		// As a final fallback, use fmt, but this should rarely be hit.
		s.buf = append(s.buf, fmt.Sprint(v.Interface())...)
	}
}

// serializeJSON formats log entries as JSON (time, level, trace, fields).
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

// serializeText formats log entries as plain text (time, level, trace, fields).
func (s *serializer) serializeText(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
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

// Update cached format
func (s *serializer) setTimestampFormat(format string) {
	if format == "" {
		format = time.RFC3339Nano
	}
	s.timestampFormat = format
}

const hexChars = "0123456789abcdef"