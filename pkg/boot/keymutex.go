package boot

import (
	"context"
	"sort"
	"sync"
	"unsafe"
)

// KeyMutex 基于 key 的互斥锁，每个 key 独立加锁，不同 key 之间互不阻塞
// 内部使用引用计数自动回收不再使用的锁，保持最小内存占用
type KeyMutex[T comparable] struct {
	mu    sync.Mutex
	locks map[T]*keyEntry
}

type keyEntry struct {
	ref int
	ch  chan struct{}
}

// NewKeyMutex 创建一个新的 KeyMutex 实例
func NewKeyMutex[T comparable]() *KeyMutex[T] {
	return &KeyMutex[T]{
		locks: make(map[T]*keyEntry),
	}
}

// Lock 对指定 key 加锁，如果该 key 已被锁定则阻塞等待
// ctx 取消时返回 context 错误
func (km *KeyMutex[T]) Lock(ctx context.Context, key T) error {
	km.mu.Lock()
	e, ok := km.locks[key]
	if !ok {
		// 无人持有，直接获取
		e = &keyEntry{ch: make(chan struct{}, 1)}
		km.locks[key] = e
	}
	e.ref++
	km.mu.Unlock()

	// 尝试获取锁
	select {
	case e.ch <- struct{}{}:
		return nil
	default:
	}

	select {
	case e.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		// 获取超时，减少引用计数并尝试清理
		km.mu.Lock()
		e.ref--
		if e.ref == 0 {
			delete(km.locks, key)
		}
		km.mu.Unlock()
		return ctx.Err()
	}
}

// Unlock 释放指定 key 的锁
// 当引用计数归零时自动回收该 key 的锁资源
func (km *KeyMutex[T]) Unlock(_ context.Context, key T) {
	km.mu.Lock()
	e, ok := km.locks[key]
	if !ok {
		km.mu.Unlock()
		return
	}
	e.ref--
	if e.ref == 0 {
		delete(km.locks, key)
	}
	km.mu.Unlock()
	<-e.ch
}

// Locker 创建一个多 key 锁，按固定顺序加锁以避免死锁
// 返回的 MultiKeyLocker 可对所有指定 key 进行原子性的批量加锁/解锁
func (km *KeyMutex[T]) Locker(_ context.Context, keys ...T) *MultiKeyLocker[T] {
	// 去重
	seen := make(map[T]struct{}, len(keys))
	unique := make([]T, 0, len(keys))
	for _, k := range keys {
		if _, ok := seen[k]; !ok {
			seen[k] = struct{}{}
			unique = append(unique, k)
		}
	}

	// 按内存表示排序以固定加锁顺序，避免死锁
	sort.Slice(unique, func(i, j int) bool {
		return lessComparable(unique[i], unique[j])
	})

	return &MultiKeyLocker[T]{
		km:   km,
		keys: unique,
	}
}

// MultiKeyLocker 多 key 锁，持有多个 key 的批量锁操作
// Lock 按固定顺序加锁，Unlock 按反序解锁，避免死锁
type MultiKeyLocker[T comparable] struct {
	km   *KeyMutex[T]
	keys []T
}

// Lock 按顺序对所有 key 加锁
// 如果中途 ctx 取消，会自动释放已获取的锁并返回错误
func (ml *MultiKeyLocker[T]) Lock(ctx context.Context) error {
	for i, key := range ml.keys {
		if err := ml.km.Lock(ctx, key); err != nil {
			// 回滚已加锁的 key
			for j := i - 1; j >= 0; j-- {
				ml.km.Unlock(ctx, ml.keys[j])
			}
			return err
		}
	}
	return nil
}

// Unlock 按反序释放所有 key 的锁
func (ml *MultiKeyLocker[T]) Unlock(ctx context.Context) {
	for i := len(ml.keys) - 1; i >= 0; i-- {
		ml.km.Unlock(ctx, ml.keys[i])
	}
}

// lessComparable 通过内存表示比较两个 comparable 值，用于固定排序顺序
func lessComparable[T comparable](a, b T) bool {
	sa := unsafe.Sizeof(a)
	pa := unsafe.String((*byte)(unsafe.Pointer(&a)), sa)
	pb := unsafe.String((*byte)(unsafe.Pointer(&b)), sa)
	return pa < pb
}
