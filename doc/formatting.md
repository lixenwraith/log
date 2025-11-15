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

### Formatter Methods

#### Format Configuration
- `Type(format string)` - Set output format: "txt", "json", or "raw"
- `TimestampFormat(format string)` - Set timestamp format (Go time format)
- `ShowLevel(show bool)` - Include level in output
- `ShowTimestamp(show bool)` - Include timestamp in output

#### Formatting Methods
- `Format(flags int64, timestamp time.Time, level int64, trace string, args []any) []byte`
- `FormatWithOptions(format string, flags int64, timestamp time.Time, level int64, trace string, args []any) []byte`
- `FormatValue(v any) []byte` - Format a single value
- `FormatArgs(args ...any) []byte` - Format multiple arguments

### Format Flags

```go
const (
    FlagRaw            int64 = 0b0001  // Bypass formatter and sanitizer
    FlagShowTimestamp  int64 = 0b0010  // Include timestamp
    FlagShowLevel      int64 = 0b0100  // Include level
    FlagStructuredJSON int64 = 0b1000  // Use structured JSON with message/fields
    FlagDefault              = FlagShowTimestamp | FlagShowLevel
)
```

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
    PolicyJSON  PolicyPreset = "json"  // JSON-safe strings
    PolicyTxt   PolicyPreset = "txt"   // Text file safe
    PolicyShell PolicyPreset = "shell" // Shell command safe
)
```

- **PolicyRaw**: Pass through all characters unchanged
- **PolicyTxt**: Hex-encode non-printable characters as `<XX>`
- **PolicyJSON**: Escape control characters with JSON-style backslashes
- **PolicyShell**: Strip shell metacharacters and whitespace

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

## Integration with Logger

The logger uses these packages internally but configuration remains simple:

```go
logger := log.NewLogger()

// Configure sanitization policy
logger.ApplyConfigString(
    "format=json",
    "sanitization=json",  // Uses PolicyJSON
)

// Or with custom formatter (advanced)
s := sanitizer.New().Policy(sanitizer.PolicyShell)
customFormatter := formatter.New(s).Type("txt")
// Note: Direct formatter injection requires using lower-level APIs
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
entry := f.FormatArgs("action", "purchase", "amount", 99.99)
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

// For shell script generation
txtFormatter := formatter.New(shellSanitizer).Type("txt")
scriptLog := txtFormatter.Format(...)
```

## Performance Considerations

- Both packages use pre-allocated buffers for efficiency
- Sanitizer rules are applied in a single pass
- Formatter reuses internal buffers via `Reset()`
- No regex or reflection in hot paths

## Thread Safety

- `Formatter` instances are **NOT** thread-safe (use separate instances per goroutine)
- `Sanitizer` instances **ARE** thread-safe (immutable after creation)
- For concurrent formatting, create a formatter per goroutine or use sync.Pool