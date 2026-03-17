package boot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type stubService struct {
	name     string
	loadFn   func(context.Context) error
	unloadFn func(context.Context) error
}

// Name 返回测试服务名称。
func (s *stubService) Name() string {
	return s.name
}

// Load 执行测试服务的加载逻辑。
func (s *stubService) Load(ctx context.Context) error {
	if s.loadFn != nil {
		return s.loadFn(ctx)
	}
	return nil
}

// Unload 执行测试服务的卸载逻辑。
func (s *stubService) Unload(ctx context.Context) error {
	if s.unloadFn != nil {
		return s.unloadFn(ctx)
	}
	return nil
}

// TestLifecycleRegisterKeepsOrder 验证注册顺序会被保留。
func TestLifecycleRegisterKeepsOrder(t *testing.T) {
	lc := NewLifeCycle()

	first := &stubService{name: "first"}
	second := &stubService{name: "second"}

	if err := lc.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := lc.Register(second); err != nil {
		t.Fatalf("register second: %v", err)
	}

	if got := len(lc.services); got != 2 {
		t.Fatalf("expected 2 services, got %d", got)
	}
	if lc.services[0] != first {
		t.Fatal("expected first service to keep registration order")
	}
	if lc.services[1] != second {
		t.Fatal("expected second service to keep registration order")
	}
}

// TestLifecycleRegisterNilService 验证注册 nil 服务会返回错误。
func TestLifecycleRegisterNilService(t *testing.T) {
	lc := NewLifeCycle()

	var svc *stubService
	err := lc.Register(svc)
	if !errors.Is(err, ErrNilService) {
		t.Fatalf("expected ErrNilService, got %v", err)
	}
}

// TestLifecycleRegisterEmptyServiceName 验证空名称服务不可注册。
func TestLifecycleRegisterEmptyServiceName(t *testing.T) {
	lc := NewLifeCycle()

	err := lc.Register(&stubService{})
	if !errors.Is(err, ErrEmptyServiceName) {
		t.Fatalf("expected ErrEmptyServiceName, got %v", err)
	}
}

// TestLifecycleRegisterDuplicateServiceName 验证同名服务不能重复注册。
func TestLifecycleRegisterDuplicateServiceName(t *testing.T) {
	lc := NewLifeCycle()

	if err := lc.Register(&stubService{name: "same"}); err != nil {
		t.Fatalf("register first service: %v", err)
	}

	err := lc.Register(&stubService{name: "same"})
	if !errors.Is(err, ErrDuplicateServiceName) {
		t.Fatalf("expected ErrDuplicateServiceName, got %v", err)
	}
}

// TestLifecycleRegisterAfterLoad 验证加载完成后不能继续注册服务。
func TestLifecycleRegisterAfterLoad(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	if err := lc.Register(&stubService{name: "loaded"}); err != nil {
		t.Fatalf("register service: %v", err)
	}
	if err := lc.Load(ctx); err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}

	err := lc.Register(&stubService{name: "late"})
	if !errors.Is(err, ErrLifecycleLoaded) {
		t.Fatalf("expected ErrLifecycleLoaded, got %v", err)
	}
}

// TestLifecycleLoadOrderAndIdempotent 验证加载顺序和 Load 的幂等行为。
func TestLifecycleLoadOrderAndIdempotent(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	var calls []string
	first := &stubService{
		name: "first",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:first")
			return nil
		},
	}
	second := &stubService{
		name: "second",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:second")
			return nil
		},
	}

	if err := lc.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := lc.Register(second); err != nil {
		t.Fatalf("register second: %v", err)
	}

	if err := lc.Load(ctx); err != nil {
		t.Fatalf("first load: %v", err)
	}
	if err := lc.Load(ctx); err != nil {
		t.Fatalf("second load: %v", err)
	}

	want := []string{"load:first", "load:second"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
}

// TestLifecycleLoadFailureRollsBackInReverse 验证加载失败时会按逆序回滚。
func TestLifecycleLoadFailureRollsBackInReverse(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	var calls []string
	loadErr := errors.New("load failed")
	first := &stubService{
		name: "first",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:first")
			return nil
		},
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:first")
			return nil
		},
	}
	second := &stubService{
		name: "second",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:second")
			return nil
		},
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:second")
			return nil
		},
	}
	third := &stubService{
		name: "third",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:third")
			return loadErr
		},
	}

	for _, svc := range []*stubService{first, second, third} {
		if err := lc.Register(svc); err != nil {
			t.Fatalf("register %s: %v", svc.name, err)
		}
	}

	err := lc.Load(ctx)
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !errors.Is(err, loadErr) {
		t.Fatalf("expected load error to wrap original error, got %v", err)
	}

	want := []string{"load:first", "load:second", "load:third", "unload:second", "unload:first"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
	if lc.loaded {
		t.Fatal("expected lifecycle to remain unloaded after rollback")
	}
}

// TestLifecycleLoadFailureCanRetry 验证加载失败回滚后可以再次重试加载。
func TestLifecycleLoadFailureCanRetry(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	attempt := 0
	var calls []string

	first := &stubService{
		name: "first",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:first")
			return nil
		},
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:first")
			return nil
		},
	}
	second := &stubService{
		name: "second",
		loadFn: func(context.Context) error {
			attempt++
			calls = append(calls, fmt.Sprintf("load:second:%d", attempt))
			if attempt == 1 {
				return errors.New("first attempt failed")
			}
			return nil
		},
	}

	if err := lc.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := lc.Register(second); err != nil {
		t.Fatalf("register second: %v", err)
	}

	if err := lc.Load(ctx); err == nil {
		t.Fatal("expected first load attempt to fail")
	}
	if err := lc.Load(ctx); err != nil {
		t.Fatalf("expected second load attempt to succeed, got %v", err)
	}

	want := []string{"load:first", "load:second:1", "unload:first", "load:first", "load:second:2"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
}

// TestLifecycleUnloadReverseAndIdempotent 验证卸载顺序和 Unload 的幂等行为。
func TestLifecycleUnloadReverseAndIdempotent(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	var calls []string
	first := &stubService{
		name: "first",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:first")
			return nil
		},
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:first")
			return nil
		},
	}
	second := &stubService{
		name: "second",
		loadFn: func(context.Context) error {
			calls = append(calls, "load:second")
			return nil
		},
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:second")
			return nil
		},
	}

	if err := lc.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := lc.Register(second); err != nil {
		t.Fatalf("register second: %v", err)
	}
	if err := lc.Load(ctx); err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}

	if err := lc.Unload(ctx); err != nil {
		t.Fatalf("first unload: %v", err)
	}
	if err := lc.Unload(ctx); err != nil {
		t.Fatalf("second unload: %v", err)
	}

	want := []string{"load:first", "load:second", "unload:second", "unload:first"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
}

// TestLifecycleUnloadContinuesAndReturnsJoinedError 验证卸载会继续执行并聚合错误。
func TestLifecycleUnloadContinuesAndReturnsJoinedError(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	var calls []string
	errSecond := errors.New("unload second failed")
	errThird := errors.New("unload third failed")

	first := &stubService{
		name: "first",
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:first")
			return nil
		},
	}
	second := &stubService{
		name: "second",
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:second")
			return errSecond
		},
	}
	third := &stubService{
		name: "third",
		unloadFn: func(context.Context) error {
			calls = append(calls, "unload:third")
			return errThird
		},
	}

	for _, svc := range []*stubService{first, second, third} {
		if err := lc.Register(svc); err != nil {
			t.Fatalf("register %s: %v", svc.name, err)
		}
	}
	lc.loaded = true

	err := lc.Unload(ctx)
	if err == nil {
		t.Fatal("expected unload to return error")
	}
	if !errors.Is(err, errSecond) {
		t.Fatalf("expected joined error to contain second error, got %v", err)
	}
	if !errors.Is(err, errThird) {
		t.Fatalf("expected joined error to contain third error, got %v", err)
	}

	want := []string{"unload:third", "unload:second", "unload:first"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("expected calls %v, got %v", want, calls)
	}
	if lc.loaded {
		t.Fatal("expected lifecycle to be marked unloaded")
	}
}

// TestLifecycleLoadReturnsJoinedRollbackError 验证加载失败时会返回回滚错误。
func TestLifecycleLoadReturnsJoinedRollbackError(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	loadErr := errors.New("load failed")
	rollbackErr := errors.New("rollback failed")

	first := &stubService{
		name: "first",
		loadFn: func(context.Context) error {
			return nil
		},
		unloadFn: func(context.Context) error {
			return rollbackErr
		},
	}
	second := &stubService{
		name: "second",
		loadFn: func(context.Context) error {
			return loadErr
		},
	}

	if err := lc.Register(first); err != nil {
		t.Fatalf("register first: %v", err)
	}
	if err := lc.Register(second); err != nil {
		t.Fatalf("register second: %v", err)
	}

	err := lc.Load(ctx)
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !errors.Is(err, loadErr) {
		t.Fatalf("expected error to contain load error, got %v", err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("expected error to contain rollback error, got %v", err)
	}
}

// TestLifecycleErrorsContainServiceName 验证错误信息中包含服务名称，便于定位问题。
func TestLifecycleErrorsContainServiceName(t *testing.T) {
	lc := NewLifeCycle()
	ctx := context.Background()

	loadErr := errors.New("boom")
	if err := lc.Register(&stubService{
		name: "demo",
		loadFn: func(context.Context) error {
			return loadErr
		},
	}); err != nil {
		t.Fatalf("register service: %v", err)
	}

	err := lc.Load(ctx)
	if err == nil {
		t.Fatal("expected load to fail")
	}
	if !strings.Contains(err.Error(), "demo") {
		t.Fatalf("expected error to contain service name, got %v", err)
	}
}
