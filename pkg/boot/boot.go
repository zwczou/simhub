package boot

import "context"

// bootContextKey 用于在 context.Context 中存取 Boot 实例的 key
type bootContextKey struct{}

// Boot 是服务生命周期管理器
// 组合 Container（依赖注入）、PubSub（发布订阅）和 Lifecycle（服务生命周期）
type Boot struct {
	*Container
	*PubSub
	*Lifecycle
}

// NewBoot 创建并返回一个初始化好的服务管理器
func NewBoot() *Boot {
	return &Boot{
		Container: NewContainer(),
		PubSub:    NewPubSub(WithRecovery()),
		Lifecycle: NewLifecycle(),
	}
}

// Context 将当前 Boot 实例存入 ctx 中并返回新的 context
//
//	ctx = boot.Context(ctx)
func (b *Boot) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, bootContextKey{}, b)
}

// FromContext 从 ctx 中提取 Boot 实例，如果不存在则返回 nil
//
//	b := boot.FromContext(ctx)
func FromContext(ctx context.Context) *Boot {
	if b, ok := ctx.Value(bootContextKey{}).(*Boot); ok {
		return b
	}
	return GetBoot()
}
