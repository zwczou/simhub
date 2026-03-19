package migration

import (
	"errors"
	"testing"

	"github.com/uptrace/bun/migrate"
)

// TestRegistryRegisterAndGet 测试注册后可以按名称取回 migration 集合。
func TestRegistryRegisterAndGet(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	ms := migrate.NewMigrations()

	if err := registry.Register("main", ms); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	got, ok := registry.Get("main")
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got != ms {
		t.Fatal("Get() returned unexpected migrations pointer")
	}
}

// TestRegistryRegisterValidate 测试注册参数校验与重复注册错误。
func TestRegistryRegisterValidate(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	ms := migrate.NewMigrations()

	if err := registry.Register("", ms); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("Register(empty) error = %v, want ErrEmptyName", err)
	}
	if err := registry.Register("main", nil); !errors.Is(err, ErrNilMigrations) {
		t.Fatalf("Register(nil) error = %v, want ErrNilMigrations", err)
	}
	if err := registry.Register("main", ms); err != nil {
		t.Fatalf("Register(main) error = %v", err)
	}
	if err := registry.Register("main", migrate.NewMigrations()); !errors.Is(err, ErrDuplicateRegistry) {
		t.Fatalf("Register(duplicate) error = %v, want ErrDuplicateRegistry", err)
	}
}

// TestRegistryNames 测试 Names() 会返回排序后的数据库名称。
func TestRegistryNames(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	registry.MustRegister("db_b", migrate.NewMigrations())
	registry.MustRegister("db_a", migrate.NewMigrations())

	names := registry.Names()
	if len(names) != 2 {
		t.Fatalf("Names() len = %d, want 2", len(names))
	}
	if names[0] != "db_a" || names[1] != "db_b" {
		t.Fatalf("Names() = %v, want [db_a db_b]", names)
	}
}
