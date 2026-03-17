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
)
