package bitflag

// MaskOf 根据多个枚举值构造一个 mask。
func MaskOf[T Enum](vals ...T) Flags[T] {
	var f Flags[T]
	return f.SetAll(vals...)
}

// Contains 判断当前 flags 是否完整包含 mask 中的所有位。
// 等价于：(f & mask) == mask
func (f Flags[T]) Contains(mask Flags[T]) bool {
	x := normalize(int64(f))
	y := normalize(int64(mask))
	return x&y == y
}

// Intersects 判断当前 flags 是否与 mask 有任意交集。
// 等价于：(f & mask) != 0
func (f Flags[T]) Intersects(mask Flags[T]) bool {
	return normalize(int64(f))&normalize(int64(mask)) != 0
}
