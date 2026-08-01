// Package formatter provides buffered and append-style formatting of log
// entries in txt, json, and raw formats.
//
// Ownership and concurrency contract:
//   - Configure via the fluent API (Type, TimestampFormat, ShowLevel,
//     ShowTimestamp) before sharing an instance; configuration is not
//     synchronized.
//   - Buffered methods (Format, FormatWithOptions, FormatValue, FormatArgs)
//     reuse an internal buffer. The returned slice is valid only until the
//     next buffered call; copy before retention or async hand-off. Single
//     goroutine only.
//   - Append methods (AppendFormat, AppendFormatWithOptions, AppendValue,
//     AppendArgs) write to a caller-provided buffer and are safe for
//     concurrent use after configuration.
package formatter

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/lixenwraith/log/sanitizer"
)

// Format flags. Resolution in Format/AppendFormat: FlagNo* suppresses,
// FlagShow* enables, otherwise configured default applies; FlagNo* wins on conflict.
// FormatWithOptions/AppendFormatWithOptions are explicit: unset FlagShow* bits mean off.
const (
	FlagRaw            int64 = 0b0001
	FlagShowTimestamp  int64 = 0b0010
	FlagShowLevel      int64 = 0b0100
	FlagStructuredJSON int64 = 0b1000
	FlagNoTimestamp    int64 = 0b010000
	FlagNoLevel        int64 = 0b100000
	FlagKV             int64 = 0b1000000 // args are alternating string keys and values
	FlagDefault              = FlagShowTimestamp | FlagShowLevel
)

// Formatter manages formatting of log entries
type Formatter struct {
	sanitizer       *sanitizer.Sanitizer
	format          string
	timestampFormat string
	showTimestamp   bool
	showLevel       bool
	ctxKeys         ContextKeys
	buf             []byte

	// Serializers are stateless and format-fixed; built once to keep the
	// per-record path allocation-free
	serTxt  *sanitizer.Serializer
	serJSON *sanitizer.Serializer
	serRaw  *sanitizer.Serializer
}

// ContextSlots is the number of correlation values a Context carries
const ContextSlots = 3

// Context carries caller-stamped values emitted with a record.
// Tag names the source; Vals are correlation counters keyed by ContextKeys.
type Context struct {
	Tag  string
	Vals [ContextSlots]uint64
}

// ContextKeys names the record keys for Context fields; empty names are omitted.
// Names are emitted verbatim and must be plain identifiers.
type ContextKeys struct {
	Tag  string
	Vals [ContextSlots]string
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
		serTxt:          sanitizer.NewSerializer("txt", san),
		serJSON:         sanitizer.NewSerializer("json", san),
		serRaw:          sanitizer.NewSerializer("raw", san),
	}
}

// ContextKeys sets the record keys used for Context values
func (f *Formatter) ContextKeys(tag string, vals ...string) *Formatter {
	f.ctxKeys = ContextKeys{Tag: tag}
	for i := 0; i < len(vals) && i < ContextSlots; i++ {
		f.ctxKeys.Vals[i] = vals[i]
	}
	return f
}

// serializerFor returns the cached serializer for a normalized format name
func (f *Formatter) serializerFor(format string) *sanitizer.Serializer {
	switch format {
	case "json":
		return f.serJSON
	case "raw":
		return f.serRaw
	default:
		return f.serTxt
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

// Format formats using configured options resolved against explicit flags.
// Returned slice aliases the internal buffer.
func (f *Formatter) Format(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	return f.FormatCtx(Context{}, flags, timestamp, level, trace, args)
}

// FormatCtx is Format with caller-stamped context.
// Returned slice aliases the internal buffer.
func (f *Formatter) FormatCtx(ctx Context, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	f.buf = f.AppendFormatCtx(f.buf[:0], ctx, flags, timestamp, level, trace, args)
	return f.buf
}

// AppendFormat appends a formatted entry to dst using configured options
// resolved against explicit flags. Safe for concurrent use.
func (f *Formatter) AppendFormat(dst []byte, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	return f.AppendFormatCtx(dst, Context{}, flags, timestamp, level, trace, args)
}

// AppendFormatCtx is AppendFormat with caller-stamped context
func (f *Formatter) AppendFormatCtx(dst []byte, ctx Context, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	eff := flags &^ (FlagShowTimestamp | FlagShowLevel)
	if resolveShow(flags, FlagShowTimestamp, FlagNoTimestamp, f.showTimestamp) {
		eff |= FlagShowTimestamp
	}
	if resolveShow(flags, FlagShowLevel, FlagNoLevel, f.showLevel) {
		eff |= FlagShowLevel
	}
	return f.AppendFormatWithOptionsCtx(dst, f.format, ctx, eff, timestamp, level, trace, args)
}

// FormatWithOptions formats with explicit format and flags, ignoring
// configured display defaults. Returned slice aliases the internal buffer.
func (f *Formatter) FormatWithOptions(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	f.buf = f.AppendFormatWithOptionsCtx(f.buf[:0], format, Context{}, flags, timestamp, level, trace, args)
	return f.buf
}

// AppendFormatWithOptions is the compatibility wrapper over the context core
func (f *Formatter) AppendFormatWithOptions(dst []byte, format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	return f.AppendFormatWithOptionsCtx(dst, format, Context{}, flags, timestamp, level, trace, args)
}

// AppendFormatWithOptionsCtx is the allocation-explicit core. Safe for
// concurrent use. Unknown formats fall back to "txt".
func (f *Formatter) AppendFormatWithOptionsCtx(dst []byte, format string, ctx Context, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte {
	// FlagRaw completely bypasses formatting, context, and sanitization
	if flags&FlagRaw != 0 {
		for i, arg := range args {
			if i > 0 {
				dst = append(dst, ' ')
			}
			switch v := arg.(type) {
			case string:
				dst = append(dst, v...)
			case []byte:
				dst = append(dst, v...)
			case fmt.Stringer:
				dst = append(dst, v.String()...)
			case error:
				dst = append(dst, v.Error()...)
			default:
				dst = append(dst, fmt.Sprint(v)...)
			}
		}
		return dst
	}

	format = normalizeFormat(format)
	serializer := f.serializerFor(format)

	switch format {
	case "raw":
		for i, arg := range args {
			dst = f.appendValue(dst, arg, serializer, i > 0)
		}
		return dst
	case "json":
		return f.appendJSON(dst, ctx, flags, timestamp, level, trace, args, serializer)
	default: // "txt"
		return f.appendTxt(dst, ctx, flags, timestamp, level, trace, args, serializer)
	}
}

func resolveShow(flags, show, no int64, configured bool) bool {
	switch {
	case flags&no != 0:
		return false
	case flags&show != 0:
		return true
	default:
		return configured
	}
}

func normalizeFormat(format string) string {
	switch format {
	case "raw", "json", "txt":
		return format
	default:
		return "txt"
	}
}

// FormatValue formats a single value. Returned slice aliases the internal buffer.
func (f *Formatter) FormatValue(v any) []byte {
	f.buf = f.AppendValue(f.buf[:0], v)
	return f.buf
}

// AppendValue appends a single formatted value to dst. Safe for concurrent use.
func (f *Formatter) AppendValue(dst []byte, v any) []byte {
	return f.appendValue(dst, v, f.serializerFor(normalizeFormat(f.format)), false)
}

// FormatArgs formats multiple arguments. Returned slice aliases the internal buffer.
func (f *Formatter) FormatArgs(args ...any) []byte {
	f.buf = f.AppendArgs(f.buf[:0], args...)
	return f.buf
}

// AppendArgs appends multiple space-separated values to dst. Safe for
// concurrent use.
func (f *Formatter) AppendArgs(dst []byte, args ...any) []byte {
	serializer := f.serializerFor(normalizeFormat(f.format))
	for i, arg := range args {
		dst = f.appendValue(dst, arg, serializer, i > 0)
	}
	return dst
}

// appendValue provides unified type conversion (was convertValue; now
// value-return style over caller buffer). Type switch body unchanged except
// buffer plumbing — replace every `serializer.WriteX(buf, ...)` with
// `serializer.WriteX(&dst, ...)` and `return dst`.
func (f *Formatter) appendValue(dst []byte, v any, serializer *sanitizer.Serializer, needsSpace bool) []byte {
	if needsSpace && len(dst) > 0 {
		dst = append(dst, ' ')
	}
	switch val := v.(type) {
	case string:
		serializer.WriteString(&dst, val)
	case []byte:
		serializer.WriteString(&dst, string(val))
	case rune:
		var runeStr [utf8.UTFMax]byte
		n := utf8.EncodeRune(runeStr[:], val)
		serializer.WriteString(&dst, string(runeStr[:n]))
	case int:
		serializer.WriteNumber(&dst, string(strconv.AppendInt(nil, int64(val), 10)))
	case int64:
		serializer.WriteNumber(&dst, string(strconv.AppendInt(nil, val, 10)))
	case uint:
		serializer.WriteNumber(&dst, string(strconv.AppendUint(nil, uint64(val), 10)))
	case uint64:
		serializer.WriteNumber(&dst, string(strconv.AppendUint(nil, val, 10)))
	case float32:
		serializer.WriteNumber(&dst, string(strconv.AppendFloat(nil, float64(val), 'f', -1, 32)))
	case float64:
		serializer.WriteNumber(&dst, string(strconv.AppendFloat(nil, val, 'f', -1, 64)))
	case bool:
		serializer.WriteBool(&dst, val)
	case nil:
		serializer.WriteNil(&dst)
	case time.Time:
		serializer.WriteString(&dst, val.Format(f.timestampFormat))
	case error:
		serializer.WriteString(&dst, val.Error())
	case fmt.Stringer:
		serializer.WriteString(&dst, val.String())
	default:
		serializer.WriteComplex(&dst, val)
	}
	return dst
}

// LevelToString converts integer level values to string
func LevelToString(level int64) string {
	switch level {
	case -8:
		return "TRACE"
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

// appendJSONKey writes a quoted literal key followed by ':'
func appendJSONKey(dst []byte, key string) []byte {
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	return dst
}

// isKV reports whether args form an even-length list with string keys
func isKV(args []any) bool {
	if len(args) == 0 || len(args)%2 != 0 {
		return false
	}
	for i := 0; i < len(args); i += 2 {
		if _, ok := args[i].(string); !ok {
			return false
		}
	}
	return true
}

// appendJSON unifies JSON output over a caller-provided buffer
func (f *Formatter) appendJSON(dst []byte, ctx Context, flags int64, timestamp time.Time, level int64, trace string, args []any, serializer *sanitizer.Serializer) []byte {
	dst = append(dst, '{')
	needsComma := false

	if flags&FlagShowTimestamp != 0 {
		dst = append(dst, `"time":"`...)
		dst = timestamp.AppendFormat(dst, f.timestampFormat)
		dst = append(dst, '"')
		needsComma = true
	}

	if flags&FlagShowLevel != 0 {
		if needsComma {
			dst = append(dst, ',')
		}
		dst = append(dst, `"level":"`...)
		dst = append(dst, LevelToString(level)...)
		dst = append(dst, '"')
		needsComma = true
	}

	// Caller-stamped context, emitted as top-level keys
	if ctx.Tag != "" && f.ctxKeys.Tag != "" {
		if needsComma {
			dst = append(dst, ',')
		}
		dst = appendJSONKey(dst, f.ctxKeys.Tag)
		serializer.WriteString(&dst, ctx.Tag)
		needsComma = true
	}
	for i, key := range f.ctxKeys.Vals {
		if key == "" {
			continue
		}
		if needsComma {
			dst = append(dst, ',')
		}
		dst = appendJSONKey(dst, key)
		dst = strconv.AppendUint(dst, ctx.Vals[i], 10)
		needsComma = true
	}

	if trace != "" {
		if needsComma {
			dst = append(dst, ',')
		}
		dst = append(dst, `"trace":`...)
		serializer.WriteString(&dst, trace)
		needsComma = true
	}

	// Handle structured JSON if flag is set and args match pattern
	if flags&FlagStructuredJSON != 0 && len(args) >= 2 {
		if message, ok := args[0].(string); ok {
			if fields, ok := args[1].(map[string]any); ok {
				if needsComma {
					dst = append(dst, ',')
				}
				dst = append(dst, `"message":`...)
				serializer.WriteString(&dst, message)

				dst = append(dst, `,"fields":`...)

				marshaledFields, err := json.Marshal(fields)
				if err != nil {
					dst = append(dst, `{"_marshal_error":"`...)
					serializer.WriteString(&dst, err.Error())
					dst = append(dst, `"}`...)
				} else {
					dst = append(dst, marshaledFields...)
				}

				dst = append(dst, '}', '\n')
				return dst
			}
		}
	}

	if len(args) > 0 {
		if needsComma {
			dst = append(dst, ',')
		}
		// Keyed object when the caller declares k/v args, positional array otherwise
		if flags&FlagKV != 0 && isKV(args) {
			dst = append(dst, `"fields":{`...)
			for i := 0; i < len(args); i += 2 {
				if i > 0 {
					dst = append(dst, ',')
				}
				serializer.WriteString(&dst, args[i].(string))
				dst = append(dst, ':')
				dst = f.appendValue(dst, args[i+1], serializer, false)
			}
			dst = append(dst, '}')
		} else {
			dst = append(dst, `"fields":[`...)
			for i, arg := range args {
				if i > 0 {
					dst = append(dst, ',')
				}
				dst = f.appendValue(dst, arg, serializer, false)
			}
			dst = append(dst, ']')
		}
	}

	dst = append(dst, '}', '\n')
	return dst
}

// appendTxt handles txt format output over a caller-provided buffer
func (f *Formatter) appendTxt(dst []byte, ctx Context, flags int64, timestamp time.Time, level int64, trace string, args []any, serializer *sanitizer.Serializer) []byte {
	needsSpace := false

	if flags&FlagShowTimestamp != 0 {
		dst = timestamp.AppendFormat(dst, f.timestampFormat)
		needsSpace = true
	}

	if flags&FlagShowLevel != 0 {
		if needsSpace {
			dst = append(dst, ' ')
		}
		dst = append(dst, LevelToString(level)...)
		needsSpace = true
	}

	if ctx.Tag != "" && f.ctxKeys.Tag != "" {
		if needsSpace {
			dst = append(dst, ' ')
		}
		dst = append(dst, f.ctxKeys.Tag...)
		dst = append(dst, '=')
		dst = f.appendValue(dst, ctx.Tag, serializer, false)
		needsSpace = true
	}
	for i, key := range f.ctxKeys.Vals {
		if key == "" {
			continue
		}
		if needsSpace {
			dst = append(dst, ' ')
		}
		dst = append(dst, key...)
		dst = append(dst, '=')
		dst = strconv.AppendUint(dst, ctx.Vals[i], 10)
		needsSpace = true
	}

	if trace != "" {
		if needsSpace {
			dst = append(dst, ' ')
		}
		// Sanitize trace to prevent terminal control sequence injection
		tempBuf := make([]byte, 0, len(trace)*2)
		f.serTxt.WriteString(&tempBuf, trace)
		// Extract content without quotes if added by txt serializer
		if len(tempBuf) > 2 && tempBuf[0] == '"' && tempBuf[len(tempBuf)-1] == '"' {
			dst = append(dst, tempBuf[1:len(tempBuf)-1]...)
		} else {
			dst = append(dst, tempBuf...)
		}
		needsSpace = true
	}

	if flags&FlagKV != 0 && isKV(args) {
		for i := 0; i < len(args); i += 2 {
			if needsSpace {
				dst = append(dst, ' ')
			}
			dst = append(dst, args[i].(string)...)
			dst = append(dst, '=')
			dst = f.appendValue(dst, args[i+1], serializer, false)
			needsSpace = true
		}
	} else {
		for _, arg := range args {
			dst = f.appendValue(dst, arg, serializer, needsSpace)
			needsSpace = true
		}
	}

	dst = append(dst, '\n')
	return dst
}

// Reset clears the internal buffer for reuse
func (f *Formatter) Reset() {
	f.buf = f.buf[:0]
}
