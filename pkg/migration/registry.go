package migration

import (
	"fmt"
	"sort"
	"sync"

	"github.com/uptrace/bun/migrate"
)

// Registry 按数据库名称维护 migration 集合。
type Registry struct {
	mu    sync.RWMutex
	items map[string]*migrate.Migrations
}

// NewRegistry 创建一个空的 migration 注册表。
func NewRegistry() *Registry {
	return &Registry{
		items: make(map[string]*migrate.Migrations),
	}
}

// Register 注册指定数据库名称对应的 migration 集合。
func (r *Registry) Register(name string, ms *migrate.Migrations) error {
	if name == "" {
		return ErrEmptyName
	}
	if ms == nil {
		return ErrNilMigrations
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.items == nil {
		r.items = make(map[string]*migrate.Migrations)
	}
	if _, ok := r.items[name]; ok {
		return fmt.Errorf("register %s: %w", name, ErrDuplicateRegistry)
	}
	r.items[name] = ms
	return nil
}

// MustRegister 注册 migration，失败时直接 panic。
func (r *Registry) MustRegister(name string, ms *migrate.Migrations) {
	if err := r.Register(name, ms); err != nil {
		panic(err)
	}
}

// Get 返回指定数据库名称对应的 migration 集合。
func (r *Registry) Get(name string) (*migrate.Migrations, bool) {
	if r == nil {
		return nil, false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	ms, ok := r.items[name]
	return ms, ok
}

// Names 返回所有已注册的数据库名称，并按字典序排序。
func (r *Registry) Names() []string {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
