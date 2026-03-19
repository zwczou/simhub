package utils

import (
	"testing"
)

func TestReplacer(t *testing.T) {
	tests := []struct {
		name     string
		kv       []any
		template string
		expected string
	}{
		{
			name:     "single replace",
			kv:       []any{"name", "world"},
			template: "hello {name}",
			expected: "hello world",
		},
		{
			name:     "multiple replace",
			kv:       []any{"user", "alice", "action", "login"},
			template: "{user} did {action}",
			expected: "alice did login",
		},
		{
			name:     "missing bracket",
			kv:       []any{"key", "val"},
			template: "hello {key",
			expected: "hello {key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewReplacer(tt.kv...)
			if got := r.Replace(tt.template); got != tt.expected {
				t.Errorf("Replacer() = %v, want %v", got, tt.expected)
			}
		})
	}

	t.Run("panic on odd args", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("The code did not panic on odd number of args")
			}
		}()
		NewReplacer("key")
	})

	t.Run("map constructor", func(t *testing.T) {
		data := map[string]string{"foo": "bar"}
		r := NewReplacerFromMap(data)
		r.With("baz", "qux")
		got := r.Replace("{foo} and {baz}")
		if got != "bar and qux" {
			t.Errorf("expected 'bar and qux', got %v", got)
		}
	})

	t.Run("global FormatTemplate", func(t *testing.T) {
		got := FormatTemplate("Hi {user}, code is {code}", "user", "Bob", "code", "200")
		if got != "Hi Bob, code is 200" {
			t.Errorf("FormatTemplate failed, got %v", got)
		}
	})
}
