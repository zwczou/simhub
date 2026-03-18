package meta

import (
	"context"
	"fmt"
	"strconv"

	"google.golang.org/grpc/metadata"
)

// metaContextKey 是 *Meta 存入 context 的 key。
type metaContextKey struct{}

// Meta 保存键值对，支持从 gRPC metadata 解析或手动设置。
//
// 使用示例：
//
//	m := meta.FromContext(ctx)             // 从 gRPC metadata 解析
//	m := meta.New()                    // 创建空 Meta
//	m.Set("uid", int64(123))               // 设置值
//	ctx = m.Context(ctx)                   // 写入 context
//	uid := meta.Get[int64](ctx, "uid")     // 泛型读取
type Meta struct {
	data map[string]any
}

// New 创建一个空的 Meta。
func New() *Meta {
	return &Meta{data: make(map[string]any, 8)}
}

// FromContext 从 context 中读取 *Meta；若不存在则尝试解析 gRPC metadata。
// gRPC metadata 中每个 key 只取第一个值（字符串）。
func FromContext(ctx context.Context) *Meta {
	if m, ok := ctx.Value(metaContextKey{}).(*Meta); ok && m != nil {
		return m
	}
	m := New()
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for k, vs := range md {
			if len(vs) > 0 {
				m.data[k] = vs[0]
			}
		}
	}
	return m
}

// Set 设置指定 key 的值，value 可以是任意类型。
func (m *Meta) Set(key string, value any) {
	m.data[key] = value
}

// Get 读取指定 key 的值，若不存在返回 nil。
func (m *Meta) Get(key string) any {
	return m.data[key]
}

// GetInt64 读取指定 key 的值并转为 int64。
// 支持 int64、float64、string 类型的自动转换，转换失败返回 0。
func (m *Meta) GetInt64(key string) int64 {
	v := m.data[key]
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		n, _ := strconv.ParseInt(val, 10, 64)
		return n
	}
	return 0
}

// GetString 读取指定 key 的值并转为 string。
// 非字符串类型使用 fmt.Sprint 转换。
func (m *Meta) GetString(key string) string {
	v := m.data[key]
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// Context 将当前 *Meta 写入 context，返回新的 context。
func (m *Meta) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, metaContextKey{}, m)
}

// Get 从 context 中读取 *Meta 并返回指定 key 的泛型值。
// 若 *Meta 不存在或类型不匹配，返回对应类型的零值。
//
// 使用示例：
//
//	uid := meta.Get[int64](ctx, "uid")
//	name := meta.Get[string](ctx, "name")
func Get[T any](ctx context.Context, key string) T {
	m := FromContext(ctx)
	v, _ := m.data[key].(T)
	return v
}
