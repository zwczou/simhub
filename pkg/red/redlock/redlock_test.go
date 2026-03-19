package redlock

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// TestTryLockSuccess 测试 TryLock 成功获取锁
func TestTryLockSuccess(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb)

	locker := rl.Locker("test-key")
	ok, err := locker.TryLock(context.Background())
	if err != nil {
		t.Fatalf("trylock failed: %v", err)
	}
	if !ok {
		t.Fatal("expected lock to succeed")
	}
}

// TestTryLockFail 测试 TryLock 在锁被占用时失败
func TestTryLockFail(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb)

	l1 := rl.Locker("test-key")
	l1.TryLock(context.Background())

	// 第二个 locker 获取同一个 key 应失败
	l2 := rl.Locker("test-key")
	ok, err := l2.TryLock(context.Background())
	if err != nil {
		t.Fatalf("trylock failed: %v", err)
	}
	if ok {
		t.Fatal("expected lock to fail (already held)")
	}
}

// TestUnlockSuccess 测试成功解锁
func TestUnlockSuccess(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb)

	locker := rl.Locker("test-key")
	locker.TryLock(context.Background())

	ok, err := locker.Unlock(context.Background())
	if err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	if !ok {
		t.Fatal("expected unlock to succeed")
	}

	// 解锁后可以重新获取
	l2 := rl.Locker("test-key")
	ok2, _ := l2.TryLock(context.Background())
	if !ok2 {
		t.Fatal("expected re-lock to succeed after unlock")
	}
}

// TestUnlockSafety 测试解锁安全性：不能解他人的锁
func TestUnlockSafety(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb)

	l1 := rl.Locker("test-key", WithValue("owner-a"))
	l1.TryLock(context.Background())

	// 不同 value 的 locker 无法解锁
	l2 := rl.Locker("test-key", WithValue("owner-b"))
	ok, err := l2.Unlock(context.Background())
	if err != nil {
		t.Fatalf("unlock failed: %v", err)
	}
	if ok {
		t.Fatal("should not unlock someone else's lock")
	}

	// 原持有者可以解锁
	ok2, _ := l1.Unlock(context.Background())
	if !ok2 {
		t.Fatal("owner should be able to unlock")
	}
}

// TestLockWithRetry 测试 Lock 带重试在锁释放后成功
func TestLockWithRetry(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb, WithRetryDelay(50*time.Millisecond))

	l1 := rl.Locker("test-key")
	l1.TryLock(context.Background())

	// 后台 100ms 后释放锁
	go func() {
		time.Sleep(100 * time.Millisecond)
		l1.Unlock(context.Background())
	}()

	l2 := rl.Locker("test-key", WithRetries(5))
	err := l2.Lock(context.Background())
	if err != nil {
		t.Fatalf("lock with retry should succeed: %v", err)
	}
}

// TestLockMaxRetriesExceeded 测试 Lock 超过最大重试次数失败
func TestLockMaxRetriesExceeded(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb, WithRetryDelay(10*time.Millisecond))

	l1 := rl.Locker("test-key")
	l1.TryLock(context.Background())

	l2 := rl.Locker("test-key", WithRetries(2))
	err := l2.Lock(context.Background())
	if err != ErrLockFailed {
		t.Fatalf("expected ErrLockFailed, got=%v", err)
	}
}

// TestLockContextCancel 测试 Lock 在 ctx 取消时立即返回
func TestLockContextCancel(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb, WithRetryDelay(5*time.Second))

	l1 := rl.Locker("test-key")
	l1.TryLock(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	l2 := rl.Locker("test-key", WithRetries(100))
	err := l2.Lock(ctx)
	if err != context.DeadlineExceeded {
		t.Fatalf("expected context.DeadlineExceeded, got=%v", err)
	}
}

// TestTtlExpiry 测试 TTL 过期后锁自动释放
func TestTtlExpiry(t *testing.T) {
	mr, rdb := newTestRedis(t)
	rl := NewRedLock(rdb, WithTtl(5*time.Second))

	l1 := rl.Locker("test-key")
	l1.TryLock(context.Background())

	// 快进 6 秒，锁过期
	mr.FastForward(6 * time.Second)

	// 新 locker 应可以获取
	l2 := rl.Locker("test-key")
	ok, _ := l2.TryLock(context.Background())
	if !ok {
		t.Fatal("expected lock to succeed after TTL expiry")
	}
}

// TestKeyValue 测试 Key() 和 Value() 返回正确的值
func TestKeyValue(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb, WithPrefix("myapp"))

	locker := rl.Locker("order:123", WithValue("custom-val"))
	if locker.Key() != "myapp:order:123" {
		t.Fatalf("expected key=myapp:order:123, got=%s", locker.Key())
	}
	if locker.Value() != "custom-val" {
		t.Fatalf("expected value=custom-val, got=%s", locker.Value())
	}
}

// TestPrefixIsolation 测试不同前缀互不影响
func TestPrefixIsolation(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl1 := NewRedLock(rdb, WithPrefix("app1"))
	rl2 := NewRedLock(rdb, WithPrefix("app2"))

	l1 := rl1.Locker("shared-key")
	l1.TryLock(context.Background())

	// 不同前缀可以获取同名 key
	l2 := rl2.Locker("shared-key")
	ok, _ := l2.TryLock(context.Background())
	if !ok {
		t.Fatal("different prefix should allow same key")
	}
}

// TestUnlockNotHeld 测试解锁一个未持有的锁返回 false
func TestUnlockNotHeld(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb)

	locker := rl.Locker("never-locked")
	ok, err := locker.Unlock(context.Background())
	if err != nil {
		t.Fatalf("unlock should not error: %v", err)
	}
	if ok {
		t.Fatal("unlock of unheld lock should return false")
	}
}

// TestMultipleKeys 测试不同 key 互不阻塞
func TestMultipleKeys(t *testing.T) {
	_, rdb := newTestRedis(t)
	rl := NewRedLock(rdb)

	l1 := rl.Locker("key-a")
	l2 := rl.Locker("key-b")

	ok1, _ := l1.TryLock(context.Background())
	ok2, _ := l2.TryLock(context.Background())

	if !ok1 || !ok2 {
		t.Fatal("different keys should not block each other")
	}
}
