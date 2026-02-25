package formatter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lixenwraith/log/sanitizer"
)

// Format flags for controlling output structure
const (
	FlagRaw            int64 = 0b0001
	FlagShowTimestamp  int64 = 0b0010
	FlagShowLevel      int64 = 0b0100
	FlagStructuredJSON int64 = 0b1000
	FlagDefault              = FlagShowTimestamp | FlagShowLevel
)

// Formatter manages the buffered writing and formatting of log entries
type Formatter struct {
	sanitizer       *sanitizer.Sanitizer
	format          string
	timestampFormat string
	showTimestamp   bool
	showLevel       bool
	buf             []byte
}

// New creates a formatter with the provided sanitizer
func New(s ...*sanitizer.Sanitizer) *Formatter {
	var san *sanitizer.Sanitizer
	if len(s) > 0 && s[0] != nil {
		san = s[0]
	} else {
		san = sanitizer.New() // Default passthrough sanitizer
	}
	return &Formatter{
		sanitizer:       san,
		format:          "txt",
		timestampFormat: time.RFC3339Nano,
		showTimestamp:   true,
		showLevel:       true,
		buf:             make([]byte, 0, 1024),
	}
}

// Type sets the output format ("txt", "json", or "raw")
func (f *Formatter) Type(format string) *Formatter {
	f.format = format
	return f
}

// TimestampFormat sets the timestamp format string
func (f *Formatter) TimestampFormat(format string) *Formatter {
	if format != "" {
		f.timestampFormat = format
	}
	return f
}

// ShowLevel sets whether to include level in output
func (f *Formatter) ShowLevel(show bool) *Formatter {
	f.showLevel = show
	return f
}

// ShowTimestamp sets whether to include timestamp in output
func (f *Formatter) ShowTimestamp(show bool) *Formatter {
	f.showTimestamp = show
	return f
}

// Format formats a log entry using configured options and explicit flags
func (f *Formatter) Format(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	// Override configured values with explicit flags
	effectiveShowTimestamp := (flags&FlagShowTimestamp) != 0 || (flags == 0 && f.showTimestamp)
	effectiveShowLevel := (flags&FlagShowLevel) != 0 || (flags == 0 && f.showLevel)

	// Build effective flags
	effectiveFlags := flags
	if effectiveShowTimestamp {
		effectiveFlags |= FlagShowTimestamp
	}
	if effectiveShowLevel {
		effectiveFlags |= FlagShowLevel
	}

	return f.FormatWithOptions(f.format, effectiveFlags, timestamp, level, trace, args)
}

// FormatWithOptions formats with explicit format and flags, ignoring configured values
func (f *Formatter) FormatWithOptions(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
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

// Reset clears the formatter buffer for reuse
func (f *Formatter) Reset() {
	f.buf = f.buf[:0]
}

// LevelToString converts integer level values to string
func LevelToString(level int64) string {
	switch level {
	case -4:
		return "DEBUG"
	case 0:
		return "INFO"
	case 4:
		return "WARN"
	case 8:
		return "ERROR"
	case 12:
		return "PROC"
	case 16:
		return "DISK"
	case 20:
		return "SYS"
	default:
		return fmt.Sprintf("LEVEL(%d)", level)
	}
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
		f.buf = append(f.buf, LevelToString(level)...)
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
		f.buf = append(f.buf, LevelToString(level)...)
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