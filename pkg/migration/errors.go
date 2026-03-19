package migration

import "errors"

var (
	// ErrEmptyName 表示注册 migration 时数据库名称为空。
	ErrEmptyName = errors.New("migration: empty name")
	// ErrNilMigrations 表示注册 migration 时未提供 migration 集合。
	ErrNilMigrations = errors.New("migration: nil migrations")
	// ErrDuplicateRegistry 表示同一数据库名称重复注册 migration。
	ErrDuplicateRegistry = errors.New("migration: duplicate registry")
	// ErrNilDbStore 表示 Runner 未提供数据库存储容器。
	ErrNilDbStore = errors.New("migration: nil db store")
)
