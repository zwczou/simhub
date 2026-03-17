package boot

import (
	"context"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyMutex_BasicLockUnlock(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	// 基础加锁解锁
	if err := km.Lock(ctx, "a"); err != nil {
		t.Fatalf("lock failed: %v", err)
	}
	km.Unlock(ctx, "a")

	// 锁释放后应清理资源
	km.mu.Lock()
	if len(km.locks) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(km.locks))
	}
	km.mu.Unlock()
}

func TestKeyMutex_DifferentKeysNotBlocked(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	if err := km.Lock(ctx, "a"); err != nil {
		t.Fatal(err)
	}

	// 不同 key 应该不被阻塞
	done := make(chan struct{})
	go func() {
		if err := km.Lock(ctx, "b"); err != nil {
			t.Errorf("lock b failed: %v", err)
		}
		km.Unlock(ctx, "b")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("different key should not block")
	}

	km.Unlock(ctx, "a")
}

func TestKeyMutex_SameKeyBlocks(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	if err := km.Lock(ctx, "a"); err != nil {
		t.Fatal(err)
	}

	acquired := make(chan struct{})
	go func() {
		if err := km.Lock(ctx, "a"); err != nil {
			t.Errorf("lock failed: %v", err)
		}
		close(acquired)
		km.Unlock(ctx, "a")
	}()

	// 同 key 应该被阻塞
	select {
	case <-acquired:
		t.Fatal("same key should block")
	case <-time.After(100 * time.Millisecond):
	}

	km.Unlock(ctx, "a")

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("should acquire after unlock")
	}
}

func TestKeyMutex_ContextCancel(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	if err := km.Lock(ctx, "a"); err != nil {
		t.Fatal(err)
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	err := km.Lock(cancelCtx, "a")
	if err == nil {
		t.Fatal("expected context error")
	}

	km.Unlock(ctx, "a")

	// 超时后资源应被清理
	km.mu.Lock()
	if len(km.locks) != 0 {
		t.Fatalf("expected 0 entries after timeout, got %d", len(km.locks))
	}
	km.mu.Unlock()
}

func TestKeyMutex_MutualExclusion(t *testing.T) {
	km := NewKeyMutex[int]()
	ctx := context.Background()

	var counter int64
	var wg sync.WaitGroup
	n := 100

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := km.Lock(ctx, 42); err != nil {
				t.Errorf("lock failed: %v", err)
				return
			}
			defer km.Unlock(ctx, 42)

			// 非原子操作，如果互斥失败会导致竞争
			v := atomic.LoadInt64(&counter)
			time.Sleep(time.Microsecond)
			atomic.StoreInt64(&counter, v+1)
		}()
	}

	wg.Wait()

	if atomic.LoadInt64(&counter) != int64(n) {
		t.Fatalf("expected %d, got %d (race detected)", n, atomic.LoadInt64(&counter))
	}
}

func TestMultiKeyLocker_BasicLockUnlock(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	locker := km.Locker(ctx, "a", "b", "c")
	if err := locker.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	locker.Unlock(ctx)

	// 资源应被清理
	km.mu.Lock()
	if len(km.locks) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(km.locks))
	}
	km.mu.Unlock()
}

func TestMultiKeyLocker_DeduplicateKeys(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	locker := km.Locker(ctx, "a", "b", "a", "c", "b")
	if len(locker.keys) != 3 {
		t.Fatalf("expected 3 unique keys, got %d", len(locker.keys))
	}

	if err := locker.Lock(ctx); err != nil {
		t.Fatal(err)
	}
	locker.Unlock(ctx)
}

func TestMultiKeyLocker_ContextCancel(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	// 先锁住 b
	if err := km.Lock(ctx, "b"); err != nil {
		t.Fatal(err)
	}

	cancelCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	locker := km.Locker(ctx, "a", "b", "c")
	err := locker.Lock(cancelCtx)
	if err == nil {
		t.Fatal("expected context error")
	}

	// a 应该已被回滚释放
	acquired := make(chan struct{})
	go func() {
		if err := km.Lock(ctx, "a"); err != nil {
			t.Errorf("lock a failed: %v", err)
		}
		close(acquired)
		km.Unlock(ctx, "a")
	}()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("a should be available after rollback")
	}

	km.Unlock(ctx, "b")
}

func TestMultiKeyLocker_NoDeadlock(t *testing.T) {
	km := NewKeyMutex[string]()
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		// 两个 goroutine 用不同顺序请求相同 keys
		go func() {
			defer wg.Done()
			l := km.Locker(ctx, "x", "y", "z")
			if err := l.Lock(ctx); err != nil {
				t.Errorf("lock failed: %v", err)
				return
			}
			time.Sleep(time.Microsecond)
			l.Unlock(ctx)
		}()
		go func() {
			defer wg.Done()
			l := km.Locker(ctx, "z", "y", "x")
			if err := l.Lock(ctx); err != nil {
				t.Errorf("lock failed: %v", err)
				return
			}
			time.Sleep(time.Microsecond)
			l.Unlock(ctx)
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock detected")
	}
}

// BenchmarkKeyMutex_SingleKey 高并发场景下对同一个 key 加锁解锁（最高竞争）
func BenchmarkKeyMutex_SingleKey(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	key := 42

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := km.Lock(ctx, key); err == nil {
				km.Unlock(ctx, key)
			}
		}
	})
}

// BenchmarkKeyMutex_RandomKeys 中等竞争：多个 goroutine 随机访问一定数量的 keys
func BenchmarkKeyMutex_RandomKeys(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	numKeys := 100 // 限制在 100 个 key，确保存在一定的冲突率

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// 为了不让 rand.Intn 内部的全局锁影响性能，使用局部随机数生成器
		r := rand.New(rand.NewSource(int64(rand.Int())))
		for pb.Next() {
			key := r.Intn(numKeys)
			if err := km.Lock(ctx, key); err == nil {
				km.Unlock(ctx, key)
			}
		}
	})
}

// BenchmarkKeyMutex_NoConflict 无竞争场景：每个 goroutine 操作自己独占的 keys
func BenchmarkKeyMutex_NoConflict(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	var gCounter int32

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// 每个 goroutine 获取一段独占的 key 范围
		base := int(atomic.AddInt32(&gCounter, 1)) * 10000
		i := 0
		for pb.Next() {
			key := base + (i % 1000)
			i++
			if err := km.Lock(ctx, key); err == nil {
				km.Unlock(ctx, key)
			}
		}
	})
}

// BenchmarkMultiKeyLocker 测试获取多个 keys（存在竞争）
func BenchmarkMultiKeyLocker(b *testing.B) {
	km := NewKeyMutex[int]()
	ctx := context.Background()
	numKeys := 50 // 较少的 key 集合，保证有死锁检测/避让的压力

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(int64(rand.Int())))
		for pb.Next() {
			// 每个 goroutine 随机拿 3 个 key
			k1 := r.Intn(numKeys)
			k2 := r.Intn(numKeys)
			k3 := r.Intn(numKeys)

			locker := km.Locker(ctx, k1, k2, k3)
			if err := locker.Lock(ctx); err == nil {
				locker.Unlock(ctx)
			}
		}
	})
}

// BenchmarkSyncMutex_Compare 对比：标准的 sync.Mutex 的性能（最高竞争基准线）
func BenchmarkSyncMutex_Compare(b *testing.B) {
	var mu sync.Mutex
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var dummy int
		for pb.Next() {
			mu.Lock()
			dummy++
			mu.Unlock()
		}
		_ = dummy
	})
}

// BenchmarkSyncMap_Compare 对比：如果只用 sync.Map + sync.Mutex 组合的简单实现性能
// 这是一种常见的（但内存泄漏的）实现方式，用来对比说明 KeyMutex 的并发机制优势
func BenchmarkSyncMap_Compare(b *testing.B) {
	var m sync.Map

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(int64(rand.Int())))
		var dummy int
		for pb.Next() {
			key := r.Intn(100)
			v, _ := m.LoadOrStore(key, &sync.Mutex{})
			mu := v.(*sync.Mutex)
			mu.Lock()
			dummy++
			mu.Unlock()
		}
		_ = dummy
	})
}
