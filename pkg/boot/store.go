package boot

import (
	"fmt"
	"iter"
	"sort"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
)

// Store 是一个泛型存储容器，支持按 name 存取多个同类型实例
// 典型用法：管理多个数据库连接（Store[bun.IDB]）或其他命名资源
type Store[T any] struct {
	mu          sync.RWMutex
	instances   map[string]T
	defaultName string
}

// NewStore 创建一个泛型存储容器
// 可选传入 defaultName 指定默认实例名称；若不传，首次 Set 的 name 会自动成为默认名称
func NewStore[T any](defaultName ...string) *Store[T] {
	s := &Store[T]{
		instances: make(map[string]T),
	}
	if len(defaultName) > 0 && defaultName[0] != "" {
		s.defaultName = defaultName[0]
	}
	return s
}

// Set 注册一个命名实例到容器中，若 name 已存在则覆盖
// 首次注册且未指定 defaultName 时，会自动将该 name 设为默认名称
func (s *Store[T]) Set(name string, value T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.defaultName == "" {
		s.defaultName = name
	}
	s.instances[name] = value
}

// Get 按 name 获取实例，第二个返回值表示是否存在
func (s *Store[T]) Get(name string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	v, ok := s.instances[name]
	return v, ok
}

// MustGet 按 name 获取实例，不存在时 panic
func (s *Store[T]) MustGet(name string) T {
	v, ok := s.Get(name)
	if !ok {
		panic(fmt.Sprintf("boot: store instance %q not found", name))
	}
	return v
}

// Default 获取默认实例，等同于 MustGet(defaultName)
func (s *Store[T]) Default() T {
	s.mu.RLock()
	name := s.defaultName
	s.mu.RUnlock()

	return s.MustGet(name)
}

// SetDefault 更改默认实例名称
func (s *Store[T]) SetDefault(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.defaultName = name
}

// Names 返回所有已注册的实例名称（排序后）
func (s *Store[T]) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.instances))
	for name := range s.instances {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Len 返回已注册实例的数量
func (s *Store[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.instances)
}

// Items 返回所有实例的迭代器，可直接用于 for range 遍历
func (s *Store[T]) Items() iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		s.mu.RLock()
		defer s.mu.RUnlock()

		for name, value := range s.instances {
			if !yield(name, value) {
				return
			}
		}
	}
}

// redisKey 根据 name 和 db 生成 Redis 实例的唯一标识
func redisKey(name string, db int) string {
	return fmt.Sprintf("%s:%d", name, db)
}

// RedisStore 是对 Store[redis.UniversalClient] 的封装，支持 (name, db) 二维索引
// db 参数可选，默认为 0
type RedisStore struct {
	store *Store[redis.UniversalClient]
}

// NewRedisStore 创建一个 Redis 存储容器
// 可选传入 defaultName 指定默认实例名称前缀
func NewRedisStore(defaultName ...string) *RedisStore {
	// 如果指定了 defaultName，内部默认 key 为 "name:0"
	var internalDefault string
	if len(defaultName) > 0 && defaultName[0] != "" {
		internalDefault = redisKey(defaultName[0], 0)
	}
	return &RedisStore{
		store: NewStore[redis.UniversalClient](internalDefault),
	}
}

// Set 注册一个 Redis 实例，通过 (name, db) 唯一确定
func (r *RedisStore) Set(name string, db int, value redis.UniversalClient) {
	r.store.Set(redisKey(name, db), value)
}

// Get 按 name 和可选的 db 获取 Redis 实例，db 默认为 0
func (r *RedisStore) Get(name string, db ...int) (redis.UniversalClient, bool) {
	d := 0
	if len(db) > 0 {
		d = db[0]
	}
	return r.store.Get(redisKey(name, d))
}

// MustGet 按 name 和可选的 db 获取 Redis 实例，不存在时 panic
func (r *RedisStore) MustGet(name string, db ...int) redis.UniversalClient {
	d := 0
	if len(db) > 0 {
		d = db[0]
	}
	return r.store.MustGet(redisKey(name, d))
}

// Default 获取默认 Redis 实例
func (r *RedisStore) Default() redis.UniversalClient {
	return r.store.Default()
}

// Items 返回所有 Redis 实例的迭代器，key 格式为 "name:db"
func (r *RedisStore) Items() iter.Seq2[string, redis.UniversalClient] {
	return r.store.Items()
}

// DbStore 是对 Store[bun.IDB] 的封装，按 name 管理多个数据库实例
type DbStore struct {
	store *Store[bun.IDB]
}

// NewDbStore 创建一个数据库存储容器
// 可选传入 defaultName 指定默认数据库名称；若不传，首次 Set 的 name 会自动成为默认名称
func NewDbStore(defaultName ...string) *DbStore {
	return &DbStore{
		store: NewStore[bun.IDB](defaultName...),
	}
}

// Set 注册一个命名数据库实例到容器中，若 name 已存在则覆盖
func (d *DbStore) Set(name string, value bun.IDB) {
	d.store.Set(name, value)
}

// Get 按 name 获取数据库实例，第二个返回值表示是否存在
func (d *DbStore) Get(name string) (bun.IDB, bool) {
	return d.store.Get(name)
}

// MustGet 按 name 获取数据库实例，不存在时 panic
func (d *DbStore) MustGet(name string) bun.IDB {
	return d.store.MustGet(name)
}

// Default 获取默认数据库实例
func (d *DbStore) Default() bun.IDB {
	return d.store.Default()
}

// SetDefault 更改默认数据库名称
func (d *DbStore) SetDefault(name string) {
	d.store.SetDefault(name)
}

// Names 返回所有已注册的数据库名称（排序后）
func (d *DbStore) Names() []string {
	return d.store.Names()
}

// Len 返回已注册数据库实例的数量
func (d *DbStore) Len() int {
	return d.store.Len()
}

// Items 返回所有数据库实例的迭代器，可直接用于 for range 遍历
func (d *DbStore) Items() iter.Seq2[string, bun.IDB] {
	return d.store.Items()
}
