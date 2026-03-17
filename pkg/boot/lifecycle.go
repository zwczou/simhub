package boot

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// Lifecycle 负责管理一组服务的注册、加载和卸载。
type Lifecycle struct {
	mu       sync.Mutex
	services []Service
	index    map[string]Service
	loaded   bool
}

// NewLifeCycle 创建一个新的 Lifecycle 实例。
func NewLifeCycle() *Lifecycle {
	return &Lifecycle{
		index: make(map[string]Service),
	}
}

// Register 将服务注册到生命周期中，并按名称去重。
func (lc *Lifecycle) Register(svc Service) error {
	if isNilService(svc) {
		return ErrNilService
	}

	name := svc.Name()
	if name == "" {
		return ErrEmptyServiceName
	}

	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.loaded {
		return ErrLifecycleLoaded
	}
	if lc.index == nil {
		lc.index = make(map[string]Service)
	}
	if _, ok := lc.index[name]; ok {
		return ErrDuplicateServiceName
	}

	lc.services = append(lc.services, svc)
	lc.index[name] = svc
	return nil
}

// Load 按注册顺序依次加载服务。
// 如果某个服务加载失败，会将之前已成功加载的服务按逆序回滚卸载。
func (lc *Lifecycle) Load(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if lc.loaded {
		return nil
	}

	loaded := make([]Service, 0, len(lc.services))
	for _, svc := range lc.services {
		if err := svc.Load(ctx); err != nil {
			loadErr := fmt.Errorf("boot: load service %q: %w", svc.Name(), err)
			// 发生加载失败时，尽量把之前已成功加载的服务恢复到未加载状态。
			rollbackErr := unloadReverse(ctx, loaded, "rollback unload")
			if rollbackErr != nil {
				return errors.Join(loadErr, rollbackErr)
			}
			return loadErr
		}
		loaded = append(loaded, svc)
	}

	lc.loaded = true
	return nil
}

// Unload 按注册逆序卸载所有已加载服务。
func (lc *Lifecycle) Unload(ctx context.Context) error {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	if !lc.loaded {
		return nil
	}

	err := unloadReverse(ctx, lc.services, "unload")
	lc.loaded = false
	return err
}

// unloadReverse 按逆序执行服务卸载，并聚合所有卸载错误。
func unloadReverse(ctx context.Context, services []Service, action string) error {
	var errs []error

	// 卸载顺序与加载顺序相反，确保后加载的服务优先释放依赖资源。
	for i := len(services) - 1; i >= 0; i-- {
		svc := services[i]
		if err := svc.Unload(ctx); err != nil {
			errs = append(errs, fmt.Errorf("boot: %s service %q: %w", action, svc.Name(), err))
		}
	}

	return errors.Join(errs...)
}

// isNilService 判断接口值或其底层指针是否为 nil。
func isNilService(svc Service) bool {
	if svc == nil {
		return true
	}

	v := reflect.ValueOf(svc)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
