package boot

import "errors"

var (
	// ErrNilService 表示注册时传入了 nil 服务。
	ErrNilService = errors.New("boot: nil service")
	// ErrEmptyServiceName 表示服务名称为空。
	ErrEmptyServiceName = errors.New("boot: empty service name")
	// ErrDuplicateServiceName 表示同名服务重复注册。
	ErrDuplicateServiceName = errors.New("boot: duplicate service name")
	// ErrLifecycleLoaded 表示生命周期已经加载完成，不能继续注册服务。
	ErrLifecycleLoaded = errors.New("boot: lifecycle already loaded")
	// ErrNilShutdownHook 表示注册卸载清理钩子时传入了 nil。
	ErrNilShutdownHook = errors.New("boot: nil shutdown hook")
	// ErrNilDependency 表示注册依赖时传入了 nil 值。
	ErrNilDependency = errors.New("boot: nil dependency")
	// ErrDuplicateDependency 表示相同精确类型的依赖重复注册。
	ErrDuplicateDependency = errors.New("boot: duplicate dependency")
	// ErrInvalidTarget 表示 Invoke 传入了非法目标变量。
	ErrInvalidTarget = errors.New("boot: invalid target")
	// ErrMissingDependency 表示目标类型缺少可用依赖。
	ErrMissingDependency = errors.New("boot: missing dependency")
	// ErrAmbiguousDependency 表示目标类型匹配到了多个依赖。
	ErrAmbiguousDependency = errors.New("boot: ambiguous dependency")
	// ErrPubSubClosed 表示 PubSub 已关闭，无法发布消息
	ErrPubSubClosed = errors.New("boot: pubsub closed")
	// ErrPubSubArgumentMismatch 表示发布参数与 topic 签名不匹配。
	ErrPubSubArgumentMismatch = errors.New("boot: pubsub argument mismatch")
	// ErrPubSubSignatureMismatch 表示同一 topic 下订阅者签名不一致。
	ErrPubSubSignatureMismatch = errors.New("boot: pubsub signature mismatch")
	// ErrStoreInstanceNotFound 表示指定名称的实例不存在。
	ErrStoreInstanceNotFound = errors.New("boot: store instance not found")
)
