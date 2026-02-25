package compat

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lixenwraith/log"
)

// parseFormat attempts to extract structured fields from printf-style format strings
// Useful for preserving structured logging semantics
func parseFormat(format string, args []any) []any {
	// Pattern to detect common structured patterns like "key=%v" or "key: %v"
	keyValuePattern := regexp.MustCompile(`(\w+)\s*[:=]\s*%[vsdqxXeEfFgGpbcU]`)

	matches := keyValuePattern.FindAllStringSubmatchIndex(format, -1)
	if len(matches) == 0 || len(matches) > len(args) {
		// Fallback to simple message if pattern doesn't match
		return []any{"msg", fmt.Sprintf(format, args...)}
	}

	// Build structured fields
	fields := make([]any, 0, len(matches)*2+2)
	lastEnd := 0
	argIndex := 0

	for _, match := range matches {
		// Add any text before this match as part of the message
		if match[0] > lastEnd {
			prefix := format[lastEnd:match[0]]
			if strings.TrimSpace(prefix) != "" {
				if len(fields) == 0 {
					fields = append(fields, "msg", strings.TrimSpace(prefix))
				}
			}
		}

		// Extract key name
		keyStart := match[2]
		keyEnd := match[3]
		key := format[keyStart:keyEnd]

		// Get corresponding value
		if argIndex < len(args) {
			fields = append(fields, key, args[argIndex])
			argIndex++
		}

		lastEnd = match[1]
	}

	// Handle remaining format string and args
	if lastEnd < len(format) {
		remainingFormat := format[lastEnd:]
		remainingArgs := args[argIndex:]
		if len(remainingArgs) > 0 {
			remaining := fmt.Sprintf(remainingFormat, remainingArgs...)
			if strings.TrimSpace(remaining) != "" {
				if len(fields) == 0 {
					fields = append(fields, "msg", strings.TrimSpace(remaining))
				} else {
					// Append to existing message
					for i := 0; i < len(fields); i += 2 {
						if fields[i] == "msg" {
							fields[i+1] = fmt.Sprintf("%v %s", fields[i+1], strings.TrimSpace(remaining))
							break
						}
					}
				}
			}
		}
	}

	return fields
}

// StructuredGnetAdapter provides enhanced structured logging for gnet
type StructuredGnetAdapter struct {
	*GnetAdapter
	extractFields bool
}

// NewStructuredGnetAdapter creates a gnet adapter with structured field extraction
func NewStructuredGnetAdapter(logger *log.Logger, opts ...GnetOption) *StructuredGnetAdapter {
	return &StructuredGnetAdapter{
		GnetAdapter:   NewGnetAdapter(logger, opts...),
		extractFields: true,
	}
}

// Debugf logs with structured field extraction
func (a *StructuredGnetAdapter) Debugf(format string, args ...any) {
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Debug(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Debugf(format, args...)
	}
}

// Infof logs with structured field extraction
func (a *StructuredGnetAdapter) Infof(format string, args ...any) {
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Info(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Infof(format, args...)
	}
}

// Warnf logs with structured field extraction
func (a *StructuredGnetAdapter) Warnf(format string, args ...any) {
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Warn(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Warnf(format, args...)
	}
}

// Errorf logs with structured field extraction
func (a *StructuredGnetAdapter) Errorf(format string, args ...any) {
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Error(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Errorf(format, args...)
	}
}