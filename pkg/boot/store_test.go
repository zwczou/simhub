package boot

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
)

// ---------- Store[T] 通用测试 ----------

// TestStoreSetAndGet 测试基本的 Set/Get 存取
func TestStoreSetAndGet(t *testing.T) {
	s := NewStore[string]()
	s.Set("user", "user-db")
	s.Set("order", "order-db")

	got, ok := s.Get("user")
	if !ok {
		t.Fatal("expected to find 'user', got not found")
	}
	if got != "user-db" {
		t.Fatalf("expected 'user-db', got '%s'", got)
	}

	got, ok = s.Get("order")
	if !ok {
		t.Fatal("expected to find 'order', got not found")
	}
	if got != "order-db" {
		t.Fatalf("expected 'order-db', got '%s'", got)
	}
}

// TestStoreGetNotFound 测试获取不存在的 name 返回 false
func TestStoreGetNotFound(t *testing.T) {
	s := NewStore[string]()
	s.Set("user", "user-db")

	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for 'nonexistent', got found")
	}
}

// TestStoreMustGetPanic 测试 MustGet 不存在时 panic
func TestStoreMustGetPanic(t *testing.T) {
	s := NewStore[string]()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for MustGet on non-existent key")
		}
	}()
	s.MustGet("nonexistent")
}

// TestStoreMustGetSuccess 测试 MustGet 存在时正常返回
func TestStoreMustGetSuccess(t *testing.T) {
	s := NewStore[string]()
	s.Set("user", "user-db")

	got := s.MustGet("user")
	if got != "user-db" {
		t.Fatalf("expected 'user-db', got '%s'", got)
	}
}

// TestStoreDefaultAutoSet 测试首次 Set 自动成为默认实例
func TestStoreDefaultAutoSet(t *testing.T) {
	s := NewStore[string]()
	s.Set("first", "first-db")
	s.Set("second", "second-db")

	got := s.Default()
	if got != "first-db" {
		t.Fatalf("expected default to be 'first-db', got '%s'", got)
	}
}

// TestStoreDefaultExplicit 测试通过构造函数指定默认 name
func TestStoreDefaultExplicit(t *testing.T) {
	s := NewStore[string]("order")
	s.Set("user", "user-db")
	s.Set("order", "order-db")

	got := s.Default()
	if got != "order-db" {
		t.Fatalf("expected default to be 'order-db', got '%s'", got)
	}
}

// TestStoreSetDefault 测试调用 SetDefault 更改默认实例
func TestStoreSetDefault(t *testing.T) {
	s := NewStore[string]()
	s.Set("user", "user-db")
	s.Set("order", "order-db")

	// 默认应为 "user"（首次 Set）
	if got := s.Default(); got != "user-db" {
		t.Fatalf("expected default 'user-db', got '%s'", got)
	}

	// 更改默认为 "order"
	if err := s.SetDefault("order"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if got := s.Default(); got != "order-db" {
		t.Fatalf("expected default 'order-db', got '%s'", got)
	}
}

// TestStoreSetDefaultNotFound 测试设置不存在的默认实例会返回错误。
func TestStoreSetDefaultNotFound(t *testing.T) {
	s := NewStore[string]()
	s.Set("user", "user-db")

	err := s.SetDefault("missing")
	if !errors.Is(err, ErrStoreInstanceNotFound) {
		t.Fatalf("expected ErrStoreInstanceNotFound, got %v", err)
	}
}

// TestStoreDefaultPanicIfEmpty 测试空容器调用 Default() 时 panic
func TestStoreDefaultPanicIfEmpty(t *testing.T) {
	s := NewStore[string]()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Default on empty store")
		}
	}()
	s.Default()
}

// TestStoreOverwrite 测试重复 Set 同名 key 覆盖旧值
func TestStoreOverwrite(t *testing.T) {
	s := NewStore[string]()
	s.Set("user", "old-db")
	s.Set("user", "new-db")

	got := s.MustGet("user")
	if got != "new-db" {
		t.Fatalf("expected 'new-db' (overwritten), got '%s'", got)
	}
}

// TestStoreNames 测试 Names() 返回所有已注册 name（排序后）
func TestStoreNames(t *testing.T) {
	s := NewStore[string]()
	s.Set("order", "order-db")
	s.Set("user", "user-db")
	s.Set("admin", "admin-db")

	names := s.Names()
	expected := []string{"admin", "order", "user"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("expected names[%d] = '%s', got '%s'", i, expected[i], name)
		}
	}
}

// TestStoreLen 测试 Len() 返回正确的数量
func TestStoreLen(t *testing.T) {
	s := NewStore[string]()
	if s.Len() != 0 {
		t.Fatalf("expected len 0, got %d", s.Len())
	}

	s.Set("a", "1")
	s.Set("b", "2")
	s.Set("c", "3")
	if s.Len() != 3 {
		t.Fatalf("expected len 3, got %d", s.Len())
	}

	// 覆盖不增加数量
	s.Set("a", "11")
	if s.Len() != 3 {
		t.Fatalf("expected len 3 after overwrite, got %d", s.Len())
	}
}

// TestStoreItems 测试 Items 迭代器遍历所有实例
func TestStoreItems(t *testing.T) {
	s := NewStore[string]()
	s.Set("a", "1")
	s.Set("b", "2")
	s.Set("c", "3")

	visited := make(map[string]string)
	for name, value := range s.Items() {
		visited[name] = value
	}

	if len(visited) != 3 {
		t.Fatalf("expected 3 visited, got %d", len(visited))
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, ok := visited[name]; !ok {
			t.Fatalf("expected '%s' to be visited", name)
		}
	}
}

// TestStoreItemsBreak 测试 Items 迭代器 break 提前停止
func TestStoreItemsBreak(t *testing.T) {
	s := NewStore[string]()
	s.Set("a", "1")
	s.Set("b", "2")
	s.Set("c", "3")

	count := 0
	for range s.Items() {
		count++
		break // 第一次就停止
	}

	if count != 1 {
		t.Fatalf("expected 1 visit (break), got %d", count)
	}
}

// TestStoreConcurrent 测试并发读写安全性
func TestStoreConcurrent(t *testing.T) {
	s := NewStore[int]()
	var wg sync.WaitGroup

	// 并发写入
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Set("key", n)
		}(i)
	}

	// 并发读取
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Get("key")
			s.Len()
			s.Names()
		}()
	}

	wg.Wait()

	// 验证最终状态合理
	if s.Len() != 1 {
		t.Fatalf("expected len 1, got %d", s.Len())
	}
}

// ---------- RedisStore 测试 ----------

// mockUniversalClient 是一个最小化的 redis.UniversalClient mock，用于区分不同 Redis 实例
type mockUniversalClient struct {
	redis.UniversalClient
	id string
}

// TestRedisStoreSetAndGet 测试基本的 Set(name, db, value) / Get(name, db)
func TestRedisStoreSetAndGet(t *testing.T) {
	rs := NewRedisStore()
	mock := &mockUniversalClient{id: "user-0"}
	rs.Set("user", 0, mock)

	got, ok := rs.Get("user", 0)
	if !ok {
		t.Fatal("expected to find 'user' db=0, got not found")
	}
	if got.(*mockUniversalClient).id != "user-0" {
		t.Fatalf("expected id 'user-0', got '%s'", got.(*mockUniversalClient).id)
	}
}

// TestRedisStoreGetDefaultDB 测试 Get(name) 不传 db 默认使用 0
func TestRedisStoreGetDefaultDB(t *testing.T) {
	rs := NewRedisStore()
	mock0 := &mockUniversalClient{id: "user-0"}
	mock1 := &mockUniversalClient{id: "user-1"}
	rs.Set("user", 0, mock0)
	rs.Set("user", 1, mock1)

	// 不传 db，默认获取 db=0
	got, ok := rs.Get("user")
	if !ok {
		t.Fatal("expected to find 'user' default db=0, got not found")
	}
	if got.(*mockUniversalClient).id != "user-0" {
		t.Fatalf("expected id 'user-0', got '%s'", got.(*mockUniversalClient).id)
	}
}

// TestRedisStoreMultipleDBs 测试同 name 不同 db 存取不同实例
func TestRedisStoreMultipleDBs(t *testing.T) {
	rs := NewRedisStore()
	rs.Set("cache", 0, &mockUniversalClient{id: "cache-0"})
	rs.Set("cache", 1, &mockUniversalClient{id: "cache-1"})
	rs.Set("cache", 2, &mockUniversalClient{id: "cache-2"})

	tests := []struct {
		db   int
		want string
	}{
		{0, "cache-0"},
		{1, "cache-1"},
		{2, "cache-2"},
	}
	for _, tt := range tests {
		got, ok := rs.Get("cache", tt.db)
		if !ok {
			t.Fatalf("expected to find 'cache' db=%d", tt.db)
		}
		if got.(*mockUniversalClient).id != tt.want {
			t.Fatalf("db=%d: expected id '%s', got '%s'", tt.db, tt.want, got.(*mockUniversalClient).id)
		}
	}
}

// TestRedisStoreMustGet 测试 MustGet 正常返回和 panic
func TestRedisStoreMustGet(t *testing.T) {
	rs := NewRedisStore()
	rs.Set("user", 0, &mockUniversalClient{id: "user-0"})

	// 正常获取
	got := rs.MustGet("user")
	if got.(*mockUniversalClient).id != "user-0" {
		t.Fatalf("expected id 'user-0', got '%s'", got.(*mockUniversalClient).id)
	}

	// 不存在时 panic
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for MustGet on non-existent key")
		}
	}()
	rs.MustGet("nonexistent")
}

// TestRedisStoreDefault 测试 Default() 返回默认实例
func TestRedisStoreDefault(t *testing.T) {
	rs := NewRedisStore("user")
	rs.Set("user", 0, &mockUniversalClient{id: "user-0"})
	rs.Set("order", 0, &mockUniversalClient{id: "order-0"})

	got := rs.Default()
	if got.(*mockUniversalClient).id != "user-0" {
		t.Fatalf("expected default id 'user-0', got '%s'", got.(*mockUniversalClient).id)
	}
}

// ---------- DbStore 测试 ----------

// mockIDB 是一个最小化的 bun.IDB mock，用于区分不同数据库实例
type mockIDB struct {
	bun.IDB
	id string
}

// TestDbStoreSetAndGet 测试基本的 Set/Get 存取
func TestDbStoreSetAndGet(t *testing.T) {
	ds := NewDbStore()
	ds.Set("user", &mockIDB{id: "user-db"})
	ds.Set("order", &mockIDB{id: "order-db"})

	got, ok := ds.Get("user")
	if !ok {
		t.Fatal("expected to find 'user', got not found")
	}
	if got.(*mockIDB).id != "user-db" {
		t.Fatalf("expected id 'user-db', got '%s'", got.(*mockIDB).id)
	}

	got, ok = ds.Get("order")
	if !ok {
		t.Fatal("expected to find 'order', got not found")
	}
	if got.(*mockIDB).id != "order-db" {
		t.Fatalf("expected id 'order-db', got '%s'", got.(*mockIDB).id)
	}
}

// TestDbStoreGetNotFound 测试获取不存在的 name 返回 false
func TestDbStoreGetNotFound(t *testing.T) {
	ds := NewDbStore()
	ds.Set("user", &mockIDB{id: "user-db"})

	_, ok := ds.Get("nonexistent")
	if ok {
		t.Fatal("expected not found for 'nonexistent', got found")
	}
}

// TestDbStoreMustGetPanic 测试 MustGet 不存在时 panic
func TestDbStoreMustGetPanic(t *testing.T) {
	ds := NewDbStore()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for MustGet on non-existent key")
		}
	}()
	ds.MustGet("nonexistent")
}

// TestDbStoreMustGetSuccess 测试 MustGet 存在时正常返回
func TestDbStoreMustGetSuccess(t *testing.T) {
	ds := NewDbStore()
	ds.Set("user", &mockIDB{id: "user-db"})

	got := ds.MustGet("user")
	if got.(*mockIDB).id != "user-db" {
		t.Fatalf("expected id 'user-db', got '%s'", got.(*mockIDB).id)
	}
}

// TestDbStoreDefaultAutoSet 测试首次 Set 自动成为默认实例
func TestDbStoreDefaultAutoSet(t *testing.T) {
	ds := NewDbStore()
	ds.Set("first", &mockIDB{id: "first-db"})
	ds.Set("second", &mockIDB{id: "second-db"})

	got := ds.Default()
	if got.(*mockIDB).id != "first-db" {
		t.Fatalf("expected default to be 'first-db', got '%s'", got.(*mockIDB).id)
	}
}

// TestDbStoreDefaultExplicit 测试通过构造函数指定默认 name
func TestDbStoreDefaultExplicit(t *testing.T) {
	ds := NewDbStore("order")
	ds.Set("user", &mockIDB{id: "user-db"})
	ds.Set("order", &mockIDB{id: "order-db"})

	got := ds.Default()
	if got.(*mockIDB).id != "order-db" {
		t.Fatalf("expected default to be 'order-db', got '%s'", got.(*mockIDB).id)
	}
}

// TestDbStoreSetDefault 测试调用 SetDefault 更改默认实例
func TestDbStoreSetDefault(t *testing.T) {
	ds := NewDbStore()
	ds.Set("user", &mockIDB{id: "user-db"})
	ds.Set("order", &mockIDB{id: "order-db"})

	if got := ds.Default(); got.(*mockIDB).id != "user-db" {
		t.Fatalf("expected default 'user-db', got '%s'", got.(*mockIDB).id)
	}

	if err := ds.SetDefault("order"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	if got := ds.Default(); got.(*mockIDB).id != "order-db" {
		t.Fatalf("expected default 'order-db', got '%s'", got.(*mockIDB).id)
	}
}

// TestDbStoreSetDefaultNotFound 测试设置不存在的数据库默认实例会返回错误。
func TestDbStoreSetDefaultNotFound(t *testing.T) {
	ds := NewDbStore()
	ds.Set("user", &mockIDB{id: "user-db"})

	err := ds.SetDefault("missing")
	if !errors.Is(err, ErrStoreInstanceNotFound) {
		t.Fatalf("expected ErrStoreInstanceNotFound, got %v", err)
	}
}

// TestDbStoreDefaultPanicIfEmpty 测试空容器调用 Default() 时 panic
func TestDbStoreDefaultPanicIfEmpty(t *testing.T) {
	ds := NewDbStore()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for Default on empty store")
		}
	}()
	ds.Default()
}

// TestDbStoreOverwrite 测试重复 Set 同名 key 覆盖旧值
func TestDbStoreOverwrite(t *testing.T) {
	ds := NewDbStore()
	ds.Set("user", &mockIDB{id: "old-db"})
	ds.Set("user", &mockIDB{id: "new-db"})

	got := ds.MustGet("user")
	if got.(*mockIDB).id != "new-db" {
		t.Fatalf("expected 'new-db' (overwritten), got '%s'", got.(*mockIDB).id)
	}
}

// TestDbStoreNames 测试 Names() 返回所有已注册 name（排序后）
func TestDbStoreNames(t *testing.T) {
	ds := NewDbStore()
	ds.Set("order", &mockIDB{id: "order-db"})
	ds.Set("user", &mockIDB{id: "user-db"})
	ds.Set("admin", &mockIDB{id: "admin-db"})

	names := ds.Names()
	expected := []string{"admin", "order", "user"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d names, got %d", len(expected), len(names))
	}
	for i, name := range names {
		if name != expected[i] {
			t.Fatalf("expected names[%d] = '%s', got '%s'", i, expected[i], name)
		}
	}
}

// TestDbStoreLen 测试 Len() 返回正确的数量
func TestDbStoreLen(t *testing.T) {
	ds := NewDbStore()
	if ds.Len() != 0 {
		t.Fatalf("expected len 0, got %d", ds.Len())
	}

	ds.Set("a", &mockIDB{id: "a"})
	ds.Set("b", &mockIDB{id: "b"})
	ds.Set("c", &mockIDB{id: "c"})
	if ds.Len() != 3 {
		t.Fatalf("expected len 3, got %d", ds.Len())
	}

	// 覆盖不增加数量
	ds.Set("a", &mockIDB{id: "a2"})
	if ds.Len() != 3 {
		t.Fatalf("expected len 3 after overwrite, got %d", ds.Len())
	}
}

// TestDbStoreItems 测试 Items 迭代器遍历所有实例
func TestDbStoreItems(t *testing.T) {
	ds := NewDbStore()
	ds.Set("a", &mockIDB{id: "a"})
	ds.Set("b", &mockIDB{id: "b"})
	ds.Set("c", &mockIDB{id: "c"})

	visited := make(map[string]string)
	for name, value := range ds.Items() {
		visited[name] = value.(*mockIDB).id
	}

	if len(visited) != 3 {
		t.Fatalf("expected 3 visited, got %d", len(visited))
	}
}

// TestDbStoreItemsBreak 测试 Items 迭代器 break 提前停止
func TestDbStoreItemsBreak(t *testing.T) {
	ds := NewDbStore()
	ds.Set("a", &mockIDB{id: "a"})
	ds.Set("b", &mockIDB{id: "b"})
	ds.Set("c", &mockIDB{id: "c"})

	count := 0
	for range ds.Items() {
		count++
		break
	}

	if count != 1 {
		t.Fatalf("expected 1 visit (break), got %d", count)
	}
}

// TestDbStoreConcurrent 测试并发读写安全性
func TestDbStoreConcurrent(t *testing.T) {
	ds := NewDbStore()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ds.Set("key", &mockIDB{id: fmt.Sprintf("db-%d", n)})
		}(i)
	}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ds.Get("key")
			ds.Len()
			ds.Names()
		}()
	}

	wg.Wait()

	if ds.Len() != 1 {
		t.Fatalf("expected len 1, got %d", ds.Len())
	}
}
