package compat

import (
	"fmt"
	"reflect"
	"testing"
)

func TestParseFormatKeepsArgumentAssociation(t *testing.T) {
	for _, tc := range []struct {
		format string
		args   []any
		want   []any
	}{
		{"prefix %s id=%d", []any{"alice", 42}, []any{"msg", "prefix alice id=42"}},
		{"id=%[2]d name=%[1]s", []any{"alice", 42}, []any{"msg", "id=42 name=alice"}},
		{"id=%d suffix", []any{42}, []any{"msg", "suffix", "id", 42}},
		{"user=%s id=%d", []any{"alice", 42}, []any{"user", "alice", "id", 42}},
		{"id=%d %s", []any{42, "tail"}, []any{"msg", "id=42 tail"}},
		{"id=%*d", []any{4, 42}, []any{"msg", "id=  42"}},
		{"literal %% id=%d", []any{42}, []any{"msg", "literal % id=42"}},
	} {
		t.Run(tc.format, func(t *testing.T) {
			if got := parseFormat(tc.format, tc.args); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func FuzzParseFormat(f *testing.F) {
	for _, format := range []string{"id=%d", "%s id=%d", "%%id=%d", "id=%[1]s"} {
		f.Add(format, "value")
	}
	f.Fuzz(func(t *testing.T, format, value string) {
		if len(format) > 1024 {
			t.Skip()
		}
		fields := parseFormat(format, []any{value})
		if len(fields)%2 != 0 {
			t.Fatalf("odd fields %#v", fields)
		}
		for i := 0; i < len(fields); i += 2 {
			if _, ok := fields[i].(string); !ok {
				t.Fatalf("non-string key %#v", fields)
			}
		}
		if len(fields) == 2 && fields[0] == "msg" {
			if fields[1] != fmt.Sprintf(format, value) {
				t.Fatal("fallback changed message")
			}
		}
	})
}
