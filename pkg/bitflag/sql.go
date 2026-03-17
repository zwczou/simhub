package bitflag

import (
	"database/sql/driver"
	"fmt"
	"strconv"
)

// Value 实现 driver.Valuer，便于写入数据库。
func (f Flags[T]) Value() (driver.Value, error) {
	return int64(f), nil
}

// Scan 实现 sql.Scanner，便于从数据库读取。
func (f *Flags[T]) Scan(src any) error {
	if f == nil {
		return fmt.Errorf("bitflag: Scan on nil pointer")
	}

	switch v := src.(type) {
	case nil:
		*f = 0
		return nil
	case int64:
		*f = Flags[T](normalize(v))
		return nil
	case int32:
		*f = Flags[T](normalize(int64(v)))
		return nil
	case int:
		*f = Flags[T](normalize(int64(v)))
		return nil
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return fmt.Errorf("bitflag: uint64 value %d overflows int64", v)
		}
		*f = Flags[T](normalize(int64(v)))
		return nil
	case []byte:
		n, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return fmt.Errorf("bitflag: parse []byte %q as int64 failed: %w", string(v), err)
		}
		*f = Flags[T](normalize(n))
		return nil
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return fmt.Errorf("bitflag: parse string %q as int64 failed: %w", v, err)
		}
		*f = Flags[T](normalize(n))
		return nil
	default:
		return fmt.Errorf("bitflag: unsupported Scan type %T", src)
	}
}
