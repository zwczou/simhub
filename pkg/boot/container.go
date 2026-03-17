package boot

import (
	"fmt"
	"reflect"
	"sync"
)

// Container 负责管理依赖注册和批量注入。
type Container struct {
	mu    sync.RWMutex
	deps  []dependency
	index map[reflect.Type]struct{}
}

type dependency struct {
	typ    reflect.Type
	holder reflect.Value
}

// NewContainer 创建一个新的 Container 实例。
func NewContainer() *Container {
	return &Container{
		index: make(map[reflect.Type]struct{}),
	}
}

// Provide 批量注册现成依赖值。
func (c *Container) Provide(vals ...any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.index == nil {
		c.index = make(map[reflect.Type]struct{})
	}

	for i, val := range vals {
		dep, err := newDependency(val)
		if err != nil {
			return fmt.Errorf("boot: provide arg %d: %w", i, err)
		}
		if _, ok := c.index[dep.typ]; ok {
			return fmt.Errorf("boot: provide type %s: %w", dep.typ, ErrDuplicateDependency)
		}

		c.deps = append(c.deps, dep)
		c.index[dep.typ] = struct{}{}
	}

	return nil
}

// Invoke 批量将已注册依赖填充到目标变量中。
func (c *Container) Invoke(targets ...any) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i, target := range targets {
		targetValue, targetType, err := resolveTarget(target)
		if err != nil {
			return fmt.Errorf("boot: invoke arg %d: %w", i, err)
		}

		value, err := c.resolve(targetType)
		if err != nil {
			return fmt.Errorf("boot: invoke arg %d type %s: %w", i, targetType, err)
		}

		targetValue.Set(value)
	}

	return nil
}

// resolve 根据目标类型查找唯一可用的依赖值。
func (c *Container) resolve(targetType reflect.Type) (reflect.Value, error) {
	var matched reflect.Value
	found := false

	for _, dep := range c.deps {
		value, ok := dep.resolve(targetType)
		if !ok {
			continue
		}
		if found {
			return reflect.Value{}, fmt.Errorf("multiple matches for %s: %w", targetType, ErrAmbiguousDependency)
		}

		matched = value
		found = true
	}

	if !found {
		return reflect.Value{}, fmt.Errorf("no match for %s: %w", targetType, ErrMissingDependency)
	}

	return matched, nil
}

// resolveTarget 校验 Invoke 目标，并返回可赋值的目标值和其类型。
func resolveTarget(target any) (reflect.Value, reflect.Type, error) {
	if isNilDependency(target) {
		return reflect.Value{}, nil, ErrInvalidTarget
	}

	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer {
		return reflect.Value{}, nil, ErrInvalidTarget
	}

	elem := value.Elem()
	if !elem.IsValid() || !elem.CanSet() {
		return reflect.Value{}, nil, ErrInvalidTarget
	}

	targetType := elem.Type()
	baseType, depth := pointerInfo(targetType)
	if depth > 0 && baseType.Kind() == reflect.Interface {
		return reflect.Value{}, nil, ErrInvalidTarget
	}

	return elem, targetType, nil
}

// newDependency 将注册值转换为稳定且可寻址的依赖表示。
func newDependency(val any) (dependency, error) {
	if isNilDependency(val) {
		return dependency{}, ErrNilDependency
	}

	value := reflect.ValueOf(val)
	holder := reflect.New(value.Type()).Elem()
	holder.Set(value)

	return dependency{
		typ:    value.Type(),
		holder: holder,
	}, nil
}

// resolve 为单个已注册依赖尝试构造目标类型对应的值。
func (d dependency) resolve(targetType reflect.Type) (reflect.Value, bool) {
	if targetType.Kind() == reflect.Interface {
		return d.resolveInterface(targetType)
	}
	return d.resolveConcrete(targetType)
}

// resolveConcrete 尝试按具体类型或更高层级指针解析依赖。
func (d dependency) resolveConcrete(targetType reflect.Type) (reflect.Value, bool) {
	depBaseType, depDepth := pointerInfo(d.typ)
	targetBaseType, targetDepth := pointerInfo(targetType)
	if depBaseType != targetBaseType || targetDepth < depDepth {
		return reflect.Value{}, false
	}

	current := d.holder
	for depth := depDepth; depth < targetDepth; depth++ {
		current = pointerize(current)
	}

	if current.Type() != targetType {
		return reflect.Value{}, false
	}

	return current, true
}

// resolveInterface 尝试按接口类型解析依赖。
func (d dependency) resolveInterface(targetType reflect.Type) (reflect.Value, bool) {
	if d.holder.Type().Implements(targetType) {
		return d.holder, true
	}

	if d.holder.Type().Kind() == reflect.Interface {
		return reflect.Value{}, false
	}

	candidate := pointerize(d.holder)
	if candidate.Type().Implements(targetType) {
		return candidate, true
	}

	return reflect.Value{}, false
}

// pointerize 将当前值提升一层指针类型。
func pointerize(value reflect.Value) reflect.Value {
	if value.CanAddr() {
		return value.Addr()
	}

	ptr := reflect.New(value.Type())
	ptr.Elem().Set(value)
	return ptr
}

// pointerInfo 返回类型的基础类型和指针层级。
func pointerInfo(typ reflect.Type) (reflect.Type, int) {
	depth := 0
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
		depth++
	}
	return typ, depth
}

// isNilDependency 判断依赖值或其底层是否为 nil。
func isNilDependency(val any) bool {
	if val == nil {
		return true
	}

	value := reflect.ValueOf(val)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
