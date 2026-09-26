# Formatting and Sanitization

The logger package exports standalone `formatter` and `sanitizer` packages that can be used independently for text formatting and sanitization needs beyond logging.

## Formatter Package

The `formatter` package provides buffered writing and formatting of log entries with support for txt, json, and raw output formats.

### Standalone Usage

```go
import (
    "time"
    "github.com/lixenwraith/log/formatter"
    "github.com/lixenwraith/log/sanitizer"
)

// Create formatter with optional sanitizer
s := sanitizer.New().Policy(sanitizer.PolicyTxt)
f := formatter.New(s)

// Configure formatter
f.Type("json").
  TimestampFormat(time.RFC3339).
  ShowLevel(true).
  ShowTimestamp(true)

// Format a log entry
data := f.Format(
    formatter.FlagDefault,
    time.Now(),
    0,  // Info level
    "", // No trace
    []any{"User logged in", "user_id", 42},
)
```

### Formatter Methods and Concurrency

The formatter provides two classes of methods. **You must understand the concurrency contract** when using these standalone:

**1. Buffered Methods (Single Goroutine Only)**
These methods reuse an internal buffer to prevent allocations. **The returned byte slice is valid ONLY until the next buffered call.** You must copy the result (`bytes.Clone()`) before retention or async hand-off.
* `Format(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte`
* `FormatWithOptions(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte`
* `FormatValue(v any) []byte`
* `FormatArgs(args ...any) []byte`

**2. Append Methods (Thread-Safe)**
These methods write to a caller-provided destination buffer. Once the formatter is configured, these are **safe for concurrent use** and are preferred for async sinks.
* `AppendFormat(dst []byte, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte`
* `AppendFormatWithOptions(dst []byte, format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte`
* `AppendValue(dst []byte, v any) []byte`
* `AppendArgs(dst []byte, args ...any) []byte`

### Format Flags

Flags use an additive resolution system. `No*` flags suppress output, `Show*` flags force output, and if neither is specified, the configured default applies. (`No*` flags win on conflicts).

```go
const (
    FlagRaw            int64 = 0b0001    // Bypass formatter and sanitizer completely
    FlagShowTimestamp  int64 = 0b0010    // Force include timestamp
    FlagShowLevel      int64 = 0b0100    // Force include level
    FlagStructuredJSON int64 = 0b1000    // Use structured JSON with message/fields
    FlagNoTimestamp    int64 = 0b010000  // Suppress timestamp
    FlagNoLevel        int64 = 0b100000  // Suppress level
    FlagKV             int64 = 0b1000000 // Alternating string keys and values
    FlagDefault              = FlagShowTimestamp | FlagShowLevel
)
```
*Note: `FormatWithOptions` and `AppendFormatWithOptions` bypass configured defaults entirely. Unset `Show*` bits in these methods mean the feature is off.*

### Level Constants

```go
// Use formatter.LevelToString() to convert levels
formatter.LevelToString(0)  // "INFO"
formatter.LevelToString(4)  // "WARN"
formatter.LevelToString(8)  // "ERROR"
```

## Sanitizer Package

The `sanitizer` package provides fluent and composable string sanitization based on configurable rules using bitwise filter flags and transforms.

### Standalone Usage

```go
import "github.com/lixenwraith/log/sanitizer"

// Create sanitizer with predefined policy
s := sanitizer.New().Policy(sanitizer.PolicyJSON)
clean := s.Sanitize("hello\nworld")  // "hello\\nworld"

// Custom rules
s = sanitizer.New().
    Rule(sanitizer.FilterControl, sanitizer.TransformHexEncode).
    Rule(sanitizer.FilterShellSpecial, sanitizer.TransformStrip)

clean = s.Sanitize("cmd; echo test")  // "cmd echo test"
```

### Predefined Policies

```go
const (
    PolicyRaw   PolicyPreset = "raw"   // No-op passthrough
    PolicyJSON  PolicyPreset = "json"  // Visible control-character escapes
    PolicyTxt   PolicyPreset = "txt"   // Text file safe
    PolicyShell PolicyPreset = "shell" // Lossy filter, not shell quoting
)
```

- **PolicyRaw**: Pass through all characters unchanged
- **PolicyTxt**: Hex-encode non-printable characters as `<XX>`
- **PolicyJSON**: Escape control characters with JSON-style backslashes
- **PolicyShell**: Strips shell metacharacters (``` ` $ ; | & > < ( ) # ' " \ * ? [ ] { } ~ ! ```), whitespace, and control characters. *Note: Used for defense-in-depth logging, NOT for safely constructing executable shell commands.*

### Filter Flags

```go
const (
    FilterNonPrintable uint64 = 1 << iota  // Non-printable runes
    FilterControl                          // Control characters
    FilterWhitespace                       // Whitespace characters
    FilterShellSpecial                     // Shell metacharacters
)
```

### Transform Flags

```go
const (
    TransformStrip      uint64 = 1 << iota  // Remove character
    TransformHexEncode                      // Encode as <XX>
    TransformJSONEscape                     // JSON backslash escape
)
```

### Custom Rules

Combine filters and transforms for custom sanitization:

```go
// Remove control characters, hex-encode non-printable
s := sanitizer.New().
    Rule(sanitizer.FilterControl, sanitizer.TransformStrip).
    Rule(sanitizer.FilterNonPrintable, sanitizer.TransformHexEncode)

// Apply multiple policies
s = sanitizer.New().
    Policy(sanitizer.PolicyTxt).
    Rule(sanitizer.FilterWhitespace, sanitizer.TransformJSONEscape)
```

### Serializer

The sanitizer includes a `Serializer` for type-aware sanitization:

```go
serializer := sanitizer.NewSerializer("json", s)

var buf []byte
serializer.WriteString(&buf, "hello\nworld")  // Adds quotes and escapes
serializer.WriteNumber(&buf, "123.45")        // No quotes for numbers
serializer.WriteBool(&buf, true)              // "true"
serializer.WriteNil(&buf)                     // "null"
```

## JSON Escaping Layers

The sanitizer is a content transform; JSON string escaping is transport
encoding applied afterward, unconditionally. Output is valid JSON for any
sanitization policy. Multi-byte UTF-8 passes through unescaped. Malformed UTF-8
is replaced before rule-based sanitization, preventing stripped text from joining
invalid byte fragments into control characters. A sanitizer with no rules remains
byte-preserving; the JSON transport layer always replaces malformed UTF-8.

JSON keys and custom timestamp layouts are escaped. Non-finite scalar floats
are emitted as strings (`"NaN"`, `"+Inf"`, `"-Inf"`); non-finite values in structured
maps follow `encoding/json` and produce a marshal error. As in the existing API,
`int32` values are treated as runes; cast to `int64` when numeric output is needed.
Choose unique context and KV keys; avoid context keys named `time`, `level`,
`trace`, `message`, or `fields`. Content policies can make distinct keys identical.

`PolicyTxt` covers text timestamps, trace, context, KV keys, and values. `PolicyRaw`
is intentionally unfiltered, and `FlagRaw` bypasses formatting and sanitization.
Generic complex-value text formatting is bounded to 64 levels and 10,000 visited
values; cycles and larger graphs produce `<cyclic or oversized value>`. This also
applies to complex values under `FlagRaw`, which use `%+v` text formatting.
Custom formatting and marshal callbacks must terminate and be safe for shared use.
A custom rule with no recognized transform preserves its matched rune. Predicate
callbacks must be safe for concurrent use when the sanitizer is shared.

- `format=json` + `sanitization=raw`: recommended; transport escaping only.
- `format=json` + `sanitization=txt`: non-printables appear as `<XX>` inside
  JSON strings.
- `format=json` + `sanitization=json`: redundant; produces visible `\\n`
  double escapes. Use `raw` instead.
- Structured JSON (`FlagStructuredJSON`) marshals the fields map via
  `encoding/json`, then applies the content policy to every string, including
  nested values and object keys. Value types and deterministic map order are preserved.
  Marshal failures produce a valid `fields._marshal_error` string.

## PolicyShell Scope

`PolicyShell` strips metacharacters, whitespace, and control characters as
defense-in-depth for logged values. It is not sufficient for constructing
shell commands from untrusted input; pass arguments via exec argv.

## Hex Marker Integrity

`PolicyTxt` hex-encodes literal `<` as `<3c>`. Every `<` in sanitized output
therefore starts a genuine marker; encoded sequences cannot be spoofed by
input containing literal `<XX>` text.

## Format Flags

| Flag | Effect |
|---|---|
| `FlagShowTimestamp` / `FlagShowLevel` | Force display on |
| `FlagNoTimestamp` / `FlagNoLevel` | Force display off (wins over Show) |
| neither | Configured default applies (`Format`/`AppendFormat` only) |

`FormatWithOptions`/`AppendFormatWithOptions` ignore configured defaults:
unset Show bits mean off. Unknown format strings fall back to `"txt"`.

## Integration with Logger

The logger uses these packages internally but configuration remains simple:

```go
logger := log.NewLogger()

// Configure sanitization policy
logger.ApplyConfigString(
    "format=json",
    "sanitization=raw",  // JSON transport escaping remains enabled
)

// Or with custom formatter (advanced)
s := sanitizer.New().Policy(sanitizer.PolicyShell)
customFormatter := formatter.New(s).Type("txt")
// Use customFormatter directly; Logger has no formatter injection API.
```

## Common Patterns

### Security-Focused Sanitization

```go
// For user input that will be logged
userInput := getUserInput()
s := sanitizer.New().
    Policy(sanitizer.PolicyShell).
    Rule(sanitizer.FilterControl, sanitizer.TransformStrip)

safeLogs := s.Sanitize(userInput)
logger.Info("User input", "data", safeLogs)
```

### Custom Log Formatting

```go
// Format logs for external system
f := formatter.New()
f.Type("json").ShowTimestamp(false).ShowLevel(false)

// Create custom log entry
entry := f.Format(formatter.FlagKV, time.Now(), 0, "", []any{"action", "purchase", "amount", 99.99})
sendToExternalSystem(entry)
```

### Multi-Target Output

```go
// Different sanitization for different outputs
jsonSanitizer := sanitizer.New().Policy(sanitizer.PolicyJSON)
shellSanitizer := sanitizer.New().Policy(sanitizer.PolicyShell)

// For JSON API
jsonFormatter := formatter.New(jsonSanitizer).Type("json")
apiLog := jsonFormatter.Format(...)

// For a filtered text log
txtFormatter := formatter.New(shellSanitizer).Type("txt")
textLog := txtFormatter.Format(...)
```

## Performance Considerations

- Formatter append methods reuse caller-provided buffers
- Sanitizer avoids allocations when no rule matches valid UTF-8
- Formatter reuses internal buffers via `Reset()`
- Scalar formatting avoids reflection; complex values and structured marshaling may use it

## Ownership and Thread Safety

- Configuration (`Type`, `ShowLevel`, `Rule`, `RuleFunc`, `Policy`, ...) must
  complete before an instance is shared between goroutines.
- `Formatter` buffered methods (`Format`, `FormatWithOptions`, `FormatValue`,
  `FormatArgs`) reuse an internal buffer. The returned slice is valid only
  until the next buffered call. Copy (`bytes.Clone`) before retaining or
  handing off to async queues. Single goroutine only.
- `Formatter` append methods (`AppendFormat`, `AppendFormatWithOptions`,
  `AppendValue`, `AppendArgs`) write to a caller-provided buffer and are safe
  for concurrent use. Preferred for async sinks and multi-goroutine callers.
- `Sanitizer` is immutable after configuration; `Sanitize`/`AppendSanitize`
  are safe for concurrent use. `Sanitize` returns the input unchanged
  (allocation-free) when no rule matches.
