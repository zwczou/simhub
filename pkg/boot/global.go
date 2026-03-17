package boot

import (
	"context"
	"sync/atomic"
)

// defaultBoot 是包级别的默认全局实例
var defaultBoot atomic.Pointer[Boot]

func init() {
	SetBoot(NewBoot())
}

// SetBoot 允许外部覆盖全局的默认 Boot 实例
func SetBoot(b *Boot) {
	if b == nil {
		return
	}
	defaultBoot.Store(b)
}

// GetBoot 获取当前全局 Boot 实例
func GetBoot() *Boot {
	return defaultBoot.Load()
}

// ---------- DI 全局方法 ----------

// Provide 注册全局依赖
func Provide(values ...any) error {
	return GetBoot().Provide(values...)
}

// Invoke 获取全局依赖
func Invoke(targets ...any) error {
	return GetBoot().Invoke(targets...)
}

// ---------- PubSub 全局方法 ----------

// Publish 向指定 topic 发布消息
func Publish(ctx context.Context, topic string, args ...any) error {
	return GetBoot().Publish(ctx, topic, args...)
}

// TryPublish 向指定 topic 发布消息，所有参数通过反射传递给订阅者的 handler
// 当某个订阅者的 channel 满时会直接跳过该订阅者，不会阻塞
func TryPublish(ctx context.Context, topic string, args ...any) error {
	return GetBoot().TryPublish(ctx, topic, args...)
}

// Subscribe 订阅指定 topic 的消息
func Subscribe(topic string, handler any, opts ...SubOption) *Subscriber {
	return GetBoot().Subscribe(topic, handler, opts...)
}

// Close 关闭全局管理器的所有发布订阅及其他资源（如适用）
func Close() {
	GetBoot().PubSub.Close()
}

// ---------- Lifecycle 全局方法 ----------

// Register 注册服务到全局管理器
func Register(services ...Service) error {
	return GetBoot().Register(services...)
}

// Load 加载全局管理器中的所有服务
func Load(ctx context.Context) error {
	return GetBoot().Load(ctx)
}

// Unload 卸载全局管理器中的所有服务
func Unload(ctx context.Context) error {
	return GetBoot().Unload(ctx)
}
