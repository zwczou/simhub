package utils

import (
	"context"
	"database/sql"

	"github.com/uptrace/bun"
)

// Scan 逐行遍历查询结果并应用回调函数，避免一次性加载大量数据占用过多内存。
// T 应该是一个指针类型，例如 *User。
func Scan[T any](ctx context.Context, db *bun.DB, rows *sql.Rows, fn func(dst T) error) error {
	defer rows.Close()
	for rows.Next() {
		// New 实例化泛型指针所指向的具体类型
		var t T
		if err := db.ScanRow(ctx, rows, &t); err != nil {
			return err
		}
		if err := fn(t); err != nil {
			return err
		}
	}
	return rows.Err()
}
