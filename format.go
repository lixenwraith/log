// FILE: lixenwraith/log/format.go
package log

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lixenwraith/log/sanitizer"
)

// Formatter manages the buffered writing and formatting of log entries
type Formatter struct {
	format          string
	buf             []byte
	timestampFormat string
	sanitizer       *sanitizer.Sanitizer
}

// NewFormatter creates a formatter instance
func NewFormatter(format string, bufferSize int64, timestampFormat string, sanitizationPolicy sanitizer.PolicyPreset) *Formatter {
	if timestampFormat == "" {
		timestampFormat = time.RFC3339Nano
	}
	if format == "" {
		format = "txt"
	}
	if sanitizationPolicy == "" {
		sanitizationPolicy = "raw"
	}

	s := (sanitizer.New()).Policy(sanitizationPolicy)
	return &Formatter{
		format:          format,
		buf:             make([]byte, 0, bufferSize),
		timestampFormat: timestampFormat,
		sanitizer:       s,
	}
}

// Reset clears the formatter buffer for reuse
func (f *Formatter) Reset() {
	f.buf = f.buf[:0]
}

// Format converts log entries to the configured format
func (f *Formatter) Format(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	f.Reset()

	// FlagRaw completely bypasses formatting and sanitization
	if flags&FlagRaw != 0 {
		for i, arg := range args {
			if i > 0 {
				f.buf = append(f.buf, ' ')
			}
			// Direct conversion without sanitization
			switch v := arg.(type) {
			case string:
				f.buf = append(f.buf, v...)
			case []byte:
				f.buf = append(f.buf, v...)
			case fmt.Stringer:
				f.buf = append(f.buf, v.String()...)
			case error:
				f.buf = append(f.buf, v.Error()...)
			default:
				f.buf = append(f.buf, fmt.Sprint(v)...)
			}
		}
		return f.buf
	}

	// Create the serializer based on the effective format
	serializer := sanitizer.NewSerializer(format, f.sanitizer)

	switch format {
	case "raw":
		// Raw formatting serializes the arguments and adds NO metadata or newlines
		for i, arg := range args {
			f.convertValue(&f.buf, arg, serializer, i > 0)
		}
		return f.buf

	case "json":
		return f.formatJSON(flags, timestamp, level, trace, args, serializer)

	case "txt":
		return f.formatTxt(flags, timestamp, level, trace, args, serializer)
	}

	return nil // forcing panic on unrecognized format
}

// FormatValue formats a single value according to the formatter's configuration
func (f *Formatter) FormatValue(v any) []byte {
	f.Reset()
	serializer := sanitizer.NewSerializer(f.format, f.sanitizer)
	f.convertValue(&f.buf, v, serializer, false)
	return f.buf
}

// FormatArgs formats multiple arguments as space-separated values
func (f *Formatter) FormatArgs(args ...any) []byte {
	f.Reset()
	serializer := sanitizer.NewSerializer(f.format, f.sanitizer)
	for i, arg := range args {
		f.convertValue(&f.buf, arg, serializer, i > 0)
	}
	return f.buf
}

// convertValue provides unified type conversion
func (f *Formatter) convertValue(buf *[]byte, v any, serializer *sanitizer.Serializer, needsSpace bool) {
	if needsSpace && len(*buf) > 0 {
		*buf = append(*buf, ' ')
	}

	switch val := v.(type) {
	case string:
		serializer.WriteString(buf, val)

	case []byte:
		serializer.WriteString(buf, string(val))

	case rune:
		var runeStr [utf8.UTFMax]byte
		n := utf8.EncodeRune(runeStr[:], val)
		serializer.WriteString(buf, string(runeStr[:n]))

	case int:
		num := strconv.AppendInt(nil, int64(val), 10)
		serializer.WriteNumber(buf, string(num))

	case int64:
		num := strconv.AppendInt(nil, val, 10)
		serializer.WriteNumber(buf, string(num))

	case uint:
		num := strconv.AppendUint(nil, uint64(val), 10)
		serializer.WriteNumber(buf, string(num))

	case uint64:
		num := strconv.AppendUint(nil, val, 10)
		serializer.WriteNumber(buf, string(num))

	case float32:
		num := strconv.AppendFloat(nil, float64(val), 'f', -1, 32)
		serializer.WriteNumber(buf, string(num))

	case float64:
		num := strconv.AppendFloat(nil, val, 'f', -1, 64)
		serializer.WriteNumber(buf, string(num))

	case bool:
		serializer.WriteBool(buf, val)

	case nil:
		serializer.WriteNil(buf)

	case time.Time:
		timeStr := val.Format(f.timestampFormat)
		serializer.WriteString(buf, timeStr)

	case error:
		serializer.WriteString(buf, val.Error())

	case fmt.Stringer:
		serializer.WriteString(buf, val.String())

	default:
		serializer.WriteComplex(buf, val)
	}
}

// formatJSON unifies JSON output
func (f *Formatter) formatJSON(flags int64, timestamp time.Time, level int64, trace string, args []any, serializer *sanitizer.Serializer) []byte {
	f.buf = append(f.buf, '{')
	needsComma := false

	if flags&FlagShowTimestamp != 0 {
		f.buf = append(f.buf, `"time":"`...)
		f.buf = timestamp.AppendFormat(f.buf, f.timestampFormat)
		f.buf = append(f.buf, '"')
		needsComma = true
	}

	if flags&FlagShowLevel != 0 {
		if needsComma {
			f.buf = append(f.buf, ',')
		}
		f.buf = append(f.buf, `"level":"`...)
		f.buf = append(f.buf, levelToString(level)...)
		f.buf = append(f.buf, '"')
		needsComma = true
	}

	if trace != "" {
		if needsComma {
			f.buf = append(f.buf, ',')
		}
		f.buf = append(f.buf, `"trace":`...)
		serializer.WriteString(&f.buf, trace)
		needsComma = true
	}

	// Handle structured JSON if flag is set and args match pattern
	if flags&FlagStructuredJSON != 0 && len(args) >= 2 {
		if message, ok := args[0].(string); ok {
			if fields, ok := args[1].(map[string]any); ok {
				if needsComma {
					f.buf = append(f.buf, ',')
				}
				f.buf = append(f.buf, `"message":`...)
				serializer.WriteString(&f.buf, message)

				f.buf = append(f.buf, ',')
				f.buf = append(f.buf, `"fields":`...)

				marshaledFields, err := json.Marshal(fields)
				if err != nil {
					f.buf = append(f.buf, `{"_marshal_error":"`...)
					serializer.WriteString(&f.buf, err.Error())
					f.buf = append(f.buf, `"}`...)
				} else {
					f.buf = append(f.buf, marshaledFields...)
				}

				f.buf = append(f.buf, '}', '\n')
				return f.buf
			}
		}
	}

	// Regular JSON with fields array
	if len(args) > 0 {
		if needsComma {
			f.buf = append(f.buf, ',')
		}
		f.buf = append(f.buf, `"fields":[`...)
		for i, arg := range args {
			if i > 0 {
				f.buf = append(f.buf, ',')
			}
			f.convertValue(&f.buf, arg, serializer, false)
		}
		f.buf = append(f.buf, ']')
	}

	f.buf = append(f.buf, '}', '\n')
	return f.buf
}

// formatTxt handles txt format output
func (f *Formatter) formatTxt(flags int64, timestamp time.Time, level int64, trace string, args []any, serializer *sanitizer.Serializer) []byte {
	needsSpace := false

	if flags&FlagShowTimestamp != 0 {
		f.buf = timestamp.AppendFormat(f.buf, f.timestampFormat)
		needsSpace = true
	}

	if flags&FlagShowLevel != 0 {
		if needsSpace {
			f.buf = append(f.buf, ' ')
		}
		f.buf = append(f.buf, levelToString(level)...)
		needsSpace = true
	}

	if trace != "" {
		if needsSpace {
			f.buf = append(f.buf, ' ')
		}
		// Sanitize trace to prevent terminal control sequence injection
		traceHandler := sanitizer.NewSerializer("txt", f.sanitizer)
		tempBuf := make([]byte, 0, len(trace)*2)
		traceHandler.WriteString(&tempBuf, trace)
		// Extract content without quotes if added by txt serializer
		if len(tempBuf) > 2 && tempBuf[0] == '"' && tempBuf[len(tempBuf)-1] == '"' {
			f.buf = append(f.buf, tempBuf[1:len(tempBuf)-1]...)
		} else {
			f.buf = append(f.buf, tempBuf...)
		}
		needsSpace = true
	}

	for _, arg := range args {
		f.convertValue(&f.buf, arg, serializer, needsSpace)
		needsSpace = true
	}

	f.buf = append(f.buf, '\n')
	return f.buf
}