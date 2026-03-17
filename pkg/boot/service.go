package boot

import "context"

// Service 定义服务接口，服务管理器通过此接口管理服务的生命周期
type Service interface {
	// Name 返回服务名称，用于去重和日志标识
	Name() string
	// Load 加载服务，返回 error 表示加载失败
	Load(ctx context.Context) error
	// Unload 卸载服务，返回 error 表示卸载失败
	Unload(ctx context.Context) error
}
