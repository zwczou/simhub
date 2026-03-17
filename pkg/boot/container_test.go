package boot

import (
	"errors"
	"reflect"
	"testing"
)

type testPayload struct {
	Name string
}

type tester interface {
	Value() string
}

type valueTester struct {
	Name string
}

type pointerTester struct {
	Name string
}

// Value 返回值接收者测试实现的名称。
func (v valueTester) Value() string {
	return v.Name
}

// Value 返回指针接收者测试实现的名称。
func (p *pointerTester) Value() string {
	return p.Name
}

// TestNewContainer 验证容器可以被正常创建。
func TestNewContainer(t *testing.T) {
	container := NewContainer()
	if container == nil {
		t.Fatal("expected container to be created")
	}
	if container.index == nil {
		t.Fatal("expected container index to be initialized")
	}
}

// TestContainerProvideSingleAndMultiple 验证可一次注册单个或多个依赖。
func TestContainerProvideSingleAndMultiple(t *testing.T) {
	container := NewContainer()

	first := testPayload{Name: "first"}
	second := 123
	third := "demo"

	if err := container.Provide(first); err != nil {
		t.Fatalf("provide first dependency: %v", err)
	}
	if err := container.Provide(second, third); err != nil {
		t.Fatalf("provide multiple dependencies: %v", err)
	}

	if got := len(container.deps); got != 3 {
		t.Fatalf("expected 3 dependencies, got %d", got)
	}
}

// TestContainerProvideNilDependency 验证注册 nil 依赖会返回错误。
func TestContainerProvideNilDependency(t *testing.T) {
	container := NewContainer()

	var ptr *testPayload
	if err := container.Provide(nil); !errors.Is(err, ErrNilDependency) {
		t.Fatalf("expected ErrNilDependency for nil, got %v", err)
	}
	if err := container.Provide(ptr); !errors.Is(err, ErrNilDependency) {
		t.Fatalf("expected ErrNilDependency for typed nil, got %v", err)
	}
}

// TestContainerProvideDuplicateDependency 验证相同精确类型不能重复注册。
func TestContainerProvideDuplicateDependency(t *testing.T) {
	container := NewContainer()

	if err := container.Provide(testPayload{Name: "first"}); err != nil {
		t.Fatalf("provide first dependency: %v", err)
	}

	err := container.Provide(testPayload{Name: "second"})
	if !errors.Is(err, ErrDuplicateDependency) {
		t.Fatalf("expected ErrDuplicateDependency, got %v", err)
	}
}

// TestContainerInvokeConcreteTarget 验证可按具体类型注入目标变量。
func TestContainerInvokeConcreteTarget(t *testing.T) {
	container := NewContainer()
	want := testPayload{Name: "demo"}

	if err := container.Provide(want); err != nil {
		t.Fatalf("provide dependency: %v", err)
	}

	var got testPayload
	if err := container.Invoke(&got); err != nil {
		t.Fatalf("invoke dependency: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected value %+v, got %+v", want, got)
	}
}

// TestContainerInvokeMultipleTargets 验证可一次填充多个目标变量。
func TestContainerInvokeMultipleTargets(t *testing.T) {
	container := NewContainer()
	wantPayload := testPayload{Name: "demo"}
	wantNumber := 7
	wantString := "boot"

	if err := container.Provide(wantPayload, wantNumber, wantString); err != nil {
		t.Fatalf("provide dependencies: %v", err)
	}

	var gotPayload testPayload
	var gotNumber int
	var gotString string
	if err := container.Invoke(&gotPayload, &gotNumber, &gotString); err != nil {
		t.Fatalf("invoke dependencies: %v", err)
	}

	if !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf("expected payload %+v, got %+v", wantPayload, gotPayload)
	}
	if gotNumber != wantNumber {
		t.Fatalf("expected number %d, got %d", wantNumber, gotNumber)
	}
	if gotString != wantString {
		t.Fatalf("expected string %q, got %q", wantString, gotString)
	}
}

// TestContainerInvokeInvalidTargets 验证非法目标变量会返回错误。
func TestContainerInvokeInvalidTargets(t *testing.T) {
	container := NewContainer()

	value := testPayload{Name: "demo"}
	if err := container.Provide(value); err != nil {
		t.Fatalf("provide dependency: %v", err)
	}

	var nilPtr *testPayload
	if err := container.Invoke(value); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget for non-pointer, got %v", err)
	}
	if err := container.Invoke(nilPtr); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget for nil pointer, got %v", err)
	}

	var invalid *tester
	if err := container.Invoke(&invalid); !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("expected ErrInvalidTarget for pointer-to-interface target, got %v", err)
	}
}

// TestContainerInvokeMissingDependency 验证缺失依赖时会返回错误。
func TestContainerInvokeMissingDependency(t *testing.T) {
	container := NewContainer()

	var got testPayload
	err := container.Invoke(&got)
	if !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("expected ErrMissingDependency, got %v", err)
	}
}

// TestContainerInvokeSupportsPointerLevels 验证已注册值可注入多层指针目标。
func TestContainerInvokeSupportsPointerLevels(t *testing.T) {
	container := NewContainer()
	want := testPayload{Name: "demo"}

	if err := container.Provide(want); err != nil {
		t.Fatalf("provide dependency: %v", err)
	}

	var gotValue testPayload
	var gotPtr *testPayload
	var gotPtrPtr **testPayload
	var gotPtrPtrPtr ***testPayload
	if err := container.Invoke(&gotValue, &gotPtr, &gotPtrPtr, &gotPtrPtrPtr); err != nil {
		t.Fatalf("invoke pointer levels: %v", err)
	}

	if !reflect.DeepEqual(gotValue, want) {
		t.Fatalf("expected value %+v, got %+v", want, gotValue)
	}
	if gotPtr == nil || !reflect.DeepEqual(*gotPtr, want) {
		t.Fatalf("expected pointer to match %+v, got %+v", want, gotPtr)
	}
	if gotPtrPtr == nil || *gotPtrPtr != gotPtr {
		t.Fatal("expected double pointer to reference the same single pointer")
	}
	if gotPtrPtrPtr == nil || **gotPtrPtrPtr != gotPtr {
		t.Fatal("expected triple pointer to reference the same single pointer")
	}
}

// TestContainerInvokeSupportsValueImplementedInterface 验证值类型实现接口时可注入接口变量。
func TestContainerInvokeSupportsValueImplementedInterface(t *testing.T) {
	container := NewContainer()
	want := valueTester{Name: "value"}

	if err := container.Provide(want); err != nil {
		t.Fatalf("provide dependency: %v", err)
	}

	var got tester
	if err := container.Invoke(&got); err != nil {
		t.Fatalf("invoke interface dependency: %v", err)
	}

	if got == nil || got.Value() != want.Name {
		t.Fatalf("expected interface value %q, got %#v", want.Name, got)
	}
}

// TestContainerInvokeSupportsPointerImplementedInterface 验证指针类型实现接口时也可注入接口变量。
func TestContainerInvokeSupportsPointerImplementedInterface(t *testing.T) {
	container := NewContainer()
	want := pointerTester{Name: "pointer"}

	if err := container.Provide(want); err != nil {
		t.Fatalf("provide dependency: %v", err)
	}

	var got tester
	if err := container.Invoke(&got); err != nil {
		t.Fatalf("invoke interface dependency: %v", err)
	}

	if got == nil || got.Value() != want.Name {
		t.Fatalf("expected interface value %q, got %#v", want.Name, got)
	}
}

// TestContainerInvokeAmbiguousInterface 验证同一接口有多个候选时会返回错误。
func TestContainerInvokeAmbiguousInterface(t *testing.T) {
	container := NewContainer()

	if err := container.Provide(valueTester{Name: "one"}, pointerTester{Name: "two"}); err != nil {
		t.Fatalf("provide dependencies: %v", err)
	}

	var got tester
	err := container.Invoke(&got)
	if !errors.Is(err, ErrAmbiguousDependency) {
		t.Fatalf("expected ErrAmbiguousDependency, got %v", err)
	}
}

// TestContainerInvokeDoesNotAutoDereference 验证指针依赖不会反向自动解引用成值类型。
func TestContainerInvokeDoesNotAutoDereference(t *testing.T) {
	container := NewContainer()
	value := &testPayload{Name: "pointer"}

	if err := container.Provide(value); err != nil {
		t.Fatalf("provide dependency: %v", err)
	}

	var got testPayload
	err := container.Invoke(&got)
	if !errors.Is(err, ErrMissingDependency) {
		t.Fatalf("expected ErrMissingDependency, got %v", err)
	}
}

// TestContainerInvokeAmbiguousPointerTarget 验证具体目标类型匹配到多个依赖时会返回错误。
func TestContainerInvokeAmbiguousPointerTarget(t *testing.T) {
	container := NewContainer()
	value := testPayload{Name: "value"}
	ptr := &testPayload{Name: "pointer"}

	if err := container.Provide(value, ptr); err != nil {
		t.Fatalf("provide dependencies: %v", err)
	}

	var got *testPayload
	err := container.Invoke(&got)
	if !errors.Is(err, ErrAmbiguousDependency) {
		t.Fatalf("expected ErrAmbiguousDependency, got %v", err)
	}
}
