package bitflag

import (
	"fmt"
	"math/bits"
	"strconv"
	"strings"
)

// Enum 表示 protobuf enum 这类底层为 int32 的枚举类型。
// 合法值范围要求在 [1, 63]。
// 0 通常保留给 UNSPECIFIED，不参与 bit 位映射。
type Enum interface {
	~int32
}

// Flags 使用 int64 作为底层存储。
// 由于符号位不使用，因此仅允许 1..63 共 63 个标记位。
type Flags[T Enum] int64

const (
	// MaxBit 表示最大可用 bit 序号（从 1 开始）
	MaxBit = 63
	// usableMask 仅保留低 63 位（符号位不参与 bitflag）。
	usableMask = int64(^uint64(0) >> 1)
)

// New 创建一个新的 Flags，并将传入的枚举位全部置 1。
func New[T Enum](vals ...T) Flags[T] {
	var f Flags[T]
	return f.SetAll(vals...)
}

// FromInt64 从数据库等场景读取 int64 后构造 Flags。
// 仅保留低 63 位，最高符号位会被忽略。
// 不主动校验每一位是否合法，因为这是 bit 集合，不是单个枚举值。
func FromInt64[T Enum](v int64) Flags[T] {
	return Flags[T](normalize(v))
}

// Int64 返回底层 int64 值，适合存库。
func (f Flags[T]) Int64() int64 {
	return int64(f)
}

// IsZero 判断当前是否没有任何标记位被设置。
func (f Flags[T]) IsZero() bool {
	return f == 0
}

// Clone 返回当前 Flags 的副本。
// 由于 Flags 本身是值类型，这个方法主要用于语义表达。
func (f Flags[T]) Clone() Flags[T] {
	return f
}

// Len 返回当前被置 1 的标记位数量。
func (f Flags[T]) Len() int {
	return bits.OnesCount64(uint64(normalize(int64(f))))
}

// Enums 返回当前所有已设置的枚举值，按从小到大排序。
func (f Flags[T]) Enums() []T {
	x := uint64(normalize(int64(f)))
	if x == 0 {
		return nil
	}

	out := make([]T, 0, f.Len())
	for x != 0 {
		idx := bits.TrailingZeros64(x)
		out = append(out, T(int32(idx)+1))
		x &= x - 1
	}
	return out
}

// String 返回适合调试的字符串形式。
// 例如：bitflag(13:[1,3,4])
func (f Flags[T]) String() string {
	return f.format(func(v T) string {
		return strconv.FormatInt(int64(v), 10)
	})
}

// Format 使用外部提供的 mapper 将枚举值格式化为字符串。
// 例如可用于 protobuf enum 的 String() 输出。
func (f Flags[T]) Format(mapper func(T) string) string {
	return f.format(mapper)
}

func (f Flags[T]) format(mapper func(T) string) string {
	enums := f.Enums()
	if len(enums) == 0 {
		return "bitflag(0:[])"
	}

	parts := make([]string, 0, len(enums))
	for _, v := range enums {
		parts = append(parts, mapper(v))
	}
	return "bitflag(" + strconv.FormatInt(normalize(int64(f)), 10) + ":[" + strings.Join(parts, ",") + "])"
}

// Valid 判断单个 enum 值是否可作为 bit 位使用。
func Valid[T Enum](v T) bool {
	n := int32(v)
	return n >= 1 && n <= MaxBit
}

// MustValid 校验 enum 值是否合法；不合法则 panic。
func MustValid[T Enum](v T) {
	if !Valid(v) {
		panic(fmt.Sprintf("bitflag: enum value %d out of range, must be in [1,%d]", v, MaxBit))
	}
}

// bit 将枚举值映射为对应 bit。
// 例如：1 -> 1<<0, 2 -> 1<<1, 63 -> 1<<62
func bit[T Enum](v T) int64 {
	MustValid(v)
	return int64(1) << (int32(v) - 1)
}

// Has 判断是否包含某一个标记位。
func (f Flags[T]) Has(v T) bool {
	return normalize(int64(f))&bit(v) != 0
}

// HasAll 判断是否同时包含所有给定标记位。
// 空参数时返回 true。
func (f Flags[T]) HasAll(vals ...T) bool {
	for _, v := range vals {
		if !f.Has(v) {
			return false
		}
	}
	return true
}

// HasAny 判断是否包含给定标记位中的任意一个。
// 空参数时返回 false。
func (f Flags[T]) HasAny(vals ...T) bool {
	for _, v := range vals {
		if f.Has(v) {
			return true
		}
	}
	return false
}

// Set 将某一个标记位置为 1。
func (f Flags[T]) Set(v T) Flags[T] {
	return Flags[T](normalize(int64(f)) | bit(v))
}

// Clear 将某一个标记位置为 0。
func (f Flags[T]) Clear(v T) Flags[T] {
	return Flags[T](normalize(int64(f)) &^ bit(v))
}

// Toggle 翻转某一个标记位。
func (f Flags[T]) Toggle(v T) Flags[T] {
	return Flags[T](normalize(int64(f)) ^ bit(v))
}

// SetTo 按布尔值设置某一个标记位。
func (f Flags[T]) SetTo(v T, on bool) Flags[T] {
	if on {
		return f.Set(v)
	}
	return f.Clear(v)
}

// SetAll 批量将多个标记位置为 1。
func (f Flags[T]) SetAll(vals ...T) Flags[T] {
	x := normalize(int64(f))
	for _, v := range vals {
		x |= bit(v)
	}
	return Flags[T](normalize(x))
}

// ClearAll 批量将多个标记位置为 0。
func (f Flags[T]) ClearAll(vals ...T) Flags[T] {
	x := normalize(int64(f))
	for _, v := range vals {
		x &^= bit(v)
	}
	return Flags[T](normalize(x))
}

func normalize(v int64) int64 {
	return v & usableMask
}
