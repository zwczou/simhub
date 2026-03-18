package meta

import (
	"context"
	"testing"

	"google.golang.org/grpc/metadata"
)

func TestNew(t *testing.T) {
	m := New()
	if m == nil || m.data == nil {
		t.Fatal("New should return an initialized Meta")
	}
}

func TestSetAndGet(t *testing.T) {
	m := New()

	m.Set("uid", int64(123))
	m.Set("name", "test_user")
	m.Set("flag", true)

	if got := m.Get("uid"); got != int64(123) {
		t.Errorf("Get(uid) = %v; want %v", got, int64(123))
	}
	if got := m.Get("name"); got != "test_user" {
		t.Errorf("Get(name) = %v; want %v", got, "test_user")
	}
	if got := m.Get("flag"); got != true {
		t.Errorf("Get(flag) = %v; want %v", got, true)
	}
	if got := m.Get("nonexistent"); got != nil {
		t.Errorf("Get(nonexistent) = %v; want %v", got, nil)
	}
}

func TestGetInt64(t *testing.T) {
	m := New()

	m.Set("i64", int64(123))
	m.Set("i", int(456))
	m.Set("f64", float64(789.0))
	m.Set("str", "321")
	m.Set("invalid_str", "abc")
	m.Set("bool", true)

	tests := []struct {
		key  string
		want int64
	}{
		{"i64", 123},
		{"i", 456},
		{"f64", 789},
		{"str", 321},
		{"invalid_str", 0},
		{"bool", 0},
		{"nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := m.GetInt64(tt.key); got != tt.want {
				t.Errorf("GetInt64(%q) = %v; want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	m := New()

	m.Set("str", "hello")
	m.Set("i64", int64(123))
	m.Set("bool", true)

	tests := []struct {
		key  string
		want string
	}{
		{"str", "hello"},
		{"i64", "123"},
		{"bool", "true"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := m.GetString(tt.key); got != tt.want {
				t.Errorf("GetString(%q) = %v; want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestFromContext(t *testing.T) {
	t.Run("Empty Context", func(t *testing.T) {
		ctx := context.Background()
		m := FromContext(ctx)
		if m == nil {
			t.Fatal("FromContext should return an initialized Meta even for empty context")
		}
	})

	t.Run("With Meta in Context", func(t *testing.T) {
		m1 := New()
		m1.Set("uid", int64(100))
		ctx := m1.Context(context.Background())

		m2 := FromContext(ctx)
		if m2 != m1 {
			t.Fatal("FromContext should return the exact Meta instance from context")
		}
		if got := m2.Get("uid"); got != int64(100) {
			t.Errorf("Get(uid) = %v; want %v", got, int64(100))
		}
	})

	t.Run("Parse gRPC Metadata", func(t *testing.T) {
		md := metadata.Pairs(
			"uid", "200",
			"name", "alice",
			"empty_key", "",
		)
		ctx := metadata.NewIncomingContext(context.Background(), md)

		m := FromContext(ctx)

		if got := m.GetString("uid"); got != "200" {
			t.Errorf("GetString(uid) = %v; want %v", got, "200")
		}
		if got := m.GetString("name"); got != "alice" {
			t.Errorf("GetString(name) = %v; want %v", got, "alice")
		}
		if got := m.GetString("empty_key"); got != "" {
			t.Errorf("GetString(empty_key) = %v; want %v", got, "")
		}
	})
}

func TestGetGeneric(t *testing.T) {
	t.Run("Types Match", func(t *testing.T) {
		m := New()
		m.Set("uid", int64(123))
		m.Set("name", "bob")
		ctx := m.Context(context.Background())

		if got := Get[int64](ctx, "uid"); got != 123 {
			t.Errorf("Get[int64](uid) = %v; want %v", got, 123)
		}
		if got := Get[string](ctx, "name"); got != "bob" {
			t.Errorf("Get[string](name) = %v; want %v", got, "bob")
		}
	})

	t.Run("Types Mismatch or Missing", func(t *testing.T) {
		m := New()
		m.Set("uid", int64(123))
		ctx := m.Context(context.Background())

		// Type mismatch returns zero value
		if got := Get[string](ctx, "uid"); got != "" {
			t.Errorf("Get[string](uid) = %v; want empty string", got)
		}
		// Missing key returns zero value
		if got := Get[int64](ctx, "nonexistent"); got != 0 {
			t.Errorf("Get[int64](nonexistent) = %v; want 0", got)
		}
	})

	t.Run("No Meta in Context", func(t *testing.T) {
		ctx := context.Background()
		if got := Get[int64](ctx, "any_key"); got != 0 {
			t.Errorf("Get[int64](any_key) = %v; want 0", got)
		}
	})
}
