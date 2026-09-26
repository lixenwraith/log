package compat

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/lixenwraith/log"
)

// Only unindexed, single-argument verbs are eligible for extraction. Mixed
// printf expressions fall back intact, rather than attributing an argument to
// the wrong key (width, precision and explicit indexes may consume arguments).
var keyValuePattern = regexp.MustCompile(`(\w+)\s*[:=]\s*%[vsdqxXeEfFgGpbcU]`)

func parseFormat(format string, args []any) []any {
	fallback := func() []any { return []any{"msg", fmt.Sprintf(format, args...)} }
	matches := keyValuePattern.FindAllStringSubmatchIndex(format, -1)
	if len(matches) == 0 || len(matches) != len(args) {
		return fallback()
	}
	fields := make([]any, 0, len(matches)*2+2)
	var text []string
	end := 0
	for i, m := range matches {
		gap := format[end:m[0]]
		if strings.Contains(gap, "%") {
			return fallback()
		}
		if part := strings.TrimSpace(gap); part != "" {
			text = append(text, part)
		}
		fields = append(fields, format[m[2]:m[3]], args[i])
		end = m[1]
	}
	tail := format[end:]
	if strings.Contains(tail, "%") {
		return fallback()
	}
	if part := strings.TrimSpace(tail); part != "" {
		text = append(text, part)
	}
	if len(text) > 0 {
		fields = append([]any{"msg", strings.Join(text, " ")}, fields...)
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
	if !a.logger.Enabled(log.LevelDebug) {
		return
	}
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Debug(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Debugf(format, args...)
	}
}

// Infof logs with structured field extraction
func (a *StructuredGnetAdapter) Infof(format string, args ...any) {
	if !a.logger.Enabled(log.LevelInfo) {
		return
	}
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Info(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Infof(format, args...)
	}
}

// Warnf logs with structured field extraction
func (a *StructuredGnetAdapter) Warnf(format string, args ...any) {
	if !a.logger.Enabled(log.LevelWarn) {
		return
	}
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Warn(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Warnf(format, args...)
	}
}

// Errorf logs with structured field extraction
func (a *StructuredGnetAdapter) Errorf(format string, args ...any) {
	if !a.logger.Enabled(log.LevelError) {
		return
	}
	if a.extractFields {
		fields := parseFormat(format, args)
		a.logger.Error(append(fields, "source", "gnet")...)
	} else {
		a.GnetAdapter.Errorf(format, args...)
	}
}
