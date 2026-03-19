package redlock

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var (
	// ErrLockFailed 表示获取锁失败（重试次数耗尽）
	ErrLockFailed = errors.New("redlock: lock failed after max retries")
)

// Locker 定义了分布式锁的接口
type Locker interface {
	TryLock(ctx context.Context) (bool, error) // 尝试获取锁，不重试
	Lock(ctx context.Context) error            // 获取锁，带重试机制
	Unlock(ctx context.Context) (bool, error)  // 释放锁
	Value() string                             // 返回锁的值
	Key() string                               // 返回锁的键
}

// luaUnlock 解锁 Lua 脚本：仅当值匹配时才删除 key，防止误解他人的锁
// KEYS[1] = lock key, ARGV[1] = value
const luaUnlock = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
`

// RedLock 是 Redis 分布式锁管理器
type RedLock struct {
	rdb redis.UniversalClient
	opt options
}

// NewRedLock 创建分布式锁管理器
//
//	rl := redlock.NewRedLock(rdb, redlock.WithPrefix("myapp"))
func NewRedLock(rdb redis.UniversalClient, opts ...Option) *RedLock {
	o := defaultOptions
	for _, fn := range opts {
		fn(&o)
	}
	return &RedLock{rdb: rdb, opt: o}
}

// Locker 创建一个分布式锁实例，支持通过 Option 覆盖默认配置
//
//	locker := rl.Locker("order:123", redlock.WithTtl(5*time.Second))
func (rl *RedLock) Locker(key string, opts ...Option) Locker {
	o := rl.opt
	for _, fn := range opts {
		fn(&o)
	}

	value := o.value
	if value == "" {
		value = generateValue()
	}

	return &lock{
		rdb:   rl.rdb,
		key:   o.prefix + ":" + key,
		value: value,
		opt:   o,
	}
}

// lock 是 Locker 的内部实现
type lock struct {
	rdb   redis.UniversalClient
	key   string
	value string
	opt   options
}

// Key 返回锁的 Redis key
func (l *lock) Key() string { return l.key }

// Value 返回锁的随机值
func (l *lock) Value() string { return l.value }

// TryLock 尝试获取锁，不重试
// 返回 (true, nil) 表示成功获取，(false, nil) 表示锁被占用
func (l *lock) TryLock(ctx context.Context) (bool, error) {
	ok, err := l.rdb.SetNX(ctx, l.key, l.value, l.opt.ttl).Result()
	if err != nil {
		return false, err
	}
	if ok {
		log.Ctx(ctx).Debug().Str("key", l.key).Msg("lock acquired")
	}
	return ok, nil
}

// Lock 获取锁，失败时按配置的重试次数和间隔重试
// ctx 取消时立即返回
func (l *lock) Lock(ctx context.Context) error {
	for i := 0; i <= l.opt.retries; i++ {
		ok, err := l.TryLock(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		// 最后一次尝试失败，不再等待
		if i == l.opt.retries {
			break
		}

		// 等待重试，ctx 取消时立即返回
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.opt.retryDelay):
		}
	}

	log.Ctx(ctx).Warn().Str("key", l.key).Int("retries", l.opt.retries).Msg("lock failed after retries")
	return ErrLockFailed
}

// Unlock 释放锁，通过 Lua 脚本原子比较 value 后删除
// 返回 (true, nil) 表示成功释放，(false, nil) 表示锁不属于当前持有者
func (l *lock) Unlock(ctx context.Context) (bool, error) {
	result, err := l.rdb.Eval(ctx, luaUnlock, []string{l.key}, l.value).Int64()
	if err != nil {
		return false, err
	}
	if result == 1 {
		log.Ctx(ctx).Debug().Str("key", l.key).Msg("lock released")
		return true, nil
	}
	log.Ctx(ctx).Warn().Str("key", l.key).Msg("lock release failed: not owner")
	return false, nil
}

// generateValue 生成 32 字符的随机 hex 字符串
func generateValue() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
