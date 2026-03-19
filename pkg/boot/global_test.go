package boot

import (
	"context"
	"errors"
	"testing"
)

type globalStubService struct {
	name     string
	loadFn   func(context.Context) error
	unloadFn func(context.Context) error
}

// Name 返回测试服务名称。
func (s *globalStubService) Name() string {
	return s.name
}

// Load 执行测试服务加载。
func (s *globalStubService) Load(ctx context.Context) error {
	if s.loadFn != nil {
		return s.loadFn(ctx)
	}
	return nil
}

// Unload 执行测试服务卸载。
func (s *globalStubService) Unload(ctx context.Context) error {
	if s.unloadFn != nil {
		return s.unloadFn(ctx)
	}
	return nil
}

// TestGlobalProvideReturnsError 验证全局 Provide 会透传底层错误。
func TestGlobalProvideReturnsError(t *testing.T) {
	old := GetBoot()
	SetBoot(NewBoot())
	t.Cleanup(func() {
		SetBoot(old)
	})

	if err := Provide(1); err != nil {
		t.Fatalf("provide first dependency: %v", err)
	}

	err := Provide(1)
	if !errors.Is(err, ErrDuplicateDependency) {
		t.Fatalf("expected ErrDuplicateDependency, got %v", err)
	}
}

// TestGlobalRegisterReturnsError 验证全局 Register 会透传底层错误。
func TestGlobalRegisterReturnsError(t *testing.T) {
	old := GetBoot()
	SetBoot(NewBoot())
	t.Cleanup(func() {
		SetBoot(old)
	})

	if err := Register(&globalStubService{name: "svc"}); err != nil {
		t.Fatalf("register first service: %v", err)
	}

	err := Register(&globalStubService{name: "svc"})
	if !errors.Is(err, ErrDuplicateServiceName) {
		t.Fatalf("expected ErrDuplicateServiceName, got %v", err)
	}
}

// TestGlobalUnloadReturnsError 验证全局 Unload 会透传底层错误。
func TestGlobalUnloadReturnsError(t *testing.T) {
	old := GetBoot()
	SetBoot(NewBoot())
	t.Cleanup(func() {
		SetBoot(old)
	})

	ctx := context.Background()
	unloadErr := errors.New("unload failed")
	svc := &globalStubService{
		name: "svc",
		unloadFn: func(context.Context) error {
			return unloadErr
		},
	}

	if err := Register(svc); err != nil {
		t.Fatalf("register service: %v", err)
	}
	if err := Load(ctx); err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}

	err := Unload(ctx)
	if !errors.Is(err, unloadErr) {
		t.Fatalf("expected unload error, got %v", err)
	}
}

// TestGlobalAddShutdownRunsOnShutdown 验证全局清理钩子会在 Shutdown 时执行。
func TestGlobalAddShutdownRunsOnShutdown(t *testing.T) {
	old := GetBoot()
	SetBoot(NewBoot())
	t.Cleanup(func() {
		SetBoot(old)
	})

	ctx := context.Background()
	called := false

	if err := AddShutdown(func(context.Context) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("add shutdown hook: %v", err)
	}

	if err := Load(ctx); err != nil {
		t.Fatalf("load lifecycle: %v", err)
	}
	if err := Unload(ctx); err != nil {
		t.Fatalf("unload lifecycle: %v", err)
	}
	if called {
		t.Fatal("expected shutdown hook not to run during unload")
	}
	if err := Shutdown(ctx); err != nil {
		t.Fatalf("shutdown lifecycle: %v", err)
	}

	if !called {
		t.Fatal("expected shutdown hook to be called")
	}
}
