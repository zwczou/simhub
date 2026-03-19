package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/iot/simhub/pkg/boot"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

// Runner 负责按数据库名称执行 migration 操作。
type Runner struct {
	dbs         *boot.DbStore
	registry    *Registry
	shouldRun   func(name string) bool
	newMigrator migratorFactory
}

type migrator interface {
	Init(ctx context.Context) error
	Lock(ctx context.Context) error
	Unlock(ctx context.Context) error
	Migrate(ctx context.Context, opts ...migrate.MigrationOption) (*migrate.MigrationGroup, error)
	Rollback(ctx context.Context, opts ...migrate.MigrationOption) (*migrate.MigrationGroup, error)
}

type migratorFactory func(db *bun.DB, ms *migrate.Migrations) migrator

// NewRunner 创建一个 migration 执行器。
func NewRunner(dbs *boot.DbStore, opts ...Option) *Runner {
	options := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}

	return &Runner{
		dbs:         dbs,
		registry:    options.registry,
		shouldRun:   options.shouldRun,
		newMigrator: options.newMigrator,
	}
}

// Register 向 Runner 的注册表中登记指定数据库的 migration 集合。
func (r *Runner) Register(name string, ms *migrate.Migrations) error {
	if r.registry == nil {
		r.registry = NewRegistry()
	}
	return r.registry.Register(name, ms)
}

// MustRegister 向 Runner 的注册表中登记 migration，失败时直接 panic。
func (r *Runner) MustRegister(name string, ms *migrate.Migrations) {
	if err := r.Register(name, ms); err != nil {
		panic(err)
	}
}

// Init 为所有符合条件的数据库初始化 migration 元数据表。
func (r *Runner) Init(ctx context.Context) error {
	return r.run(ctx, OpInit)
}

// Up 为所有符合条件的数据库执行 migration 升级。
func (r *Runner) Up(ctx context.Context) error {
	return r.run(ctx, OpUp)
}

// Down 为所有符合条件的数据库执行 migration 回滚。
func (r *Runner) Down(ctx context.Context) error {
	return r.run(ctx, OpDown)
}

// defaultMigratorFactory 创建默认的 bun migrator。
func defaultMigratorFactory(db *bun.DB, ms *migrate.Migrations) migrator {
	return migrate.NewMigrator(db, ms)
}

// run 执行指定类型的 migration 操作。
func (r *Runner) run(ctx context.Context, op Op) error {
	if r.dbs == nil {
		return ErrNilDbStore
	}
	if r.registry == nil {
		r.registry = NewRegistry()
	}
	if r.newMigrator == nil {
		r.newMigrator = defaultMigratorFactory
	}

	for _, name := range r.dbs.Names() {
		ms, ok := r.registry.Get(name)
		if !ok || !r.shouldRunName(name) {
			continue
		}

		db, ok := r.dbs.MustGet(name).(*bun.DB)
		if !ok {
			return fmt.Errorf("database %s is not *bun.DB", name)
		}
		if err := r.runOne(ctx, name, op, db, ms); err != nil {
			return err
		}
	}

	return nil
}

// shouldRunName 判断指定数据库是否需要参与当前 migration 流程。
func (r *Runner) shouldRunName(name string) bool {
	if r.shouldRun == nil {
		return true
	}
	return r.shouldRun(name)
}

// runOne 对单个数据库执行完整的 migration 生命周期。
func (r *Runner) runOne(
	ctx context.Context,
	name string,
	op Op,
	db *bun.DB,
	ms *migrate.Migrations,
) error {
	migrator := r.newMigrator(db, ms)
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("init migration for %s: %w", name, err)
	}
	if err := migrator.Lock(ctx); err != nil {
		return fmt.Errorf("lock migration for %s: %w", name, err)
	}

	err := r.runOp(ctx, name, op, migrator)
	unlockErr := migrator.Unlock(ctx)
	if err != nil {
		if unlockErr != nil {
			return fmt.Errorf("run migration for %s: %w", name, errors.Join(err, unlockErr))
		}
		return fmt.Errorf("run migration for %s: %w", name, err)
	}
	if unlockErr != nil {
		return fmt.Errorf("unlock migration for %s: %w", name, unlockErr)
	}
	return nil
}

// runOp 执行具体的 migration 动作并输出统一日志。
func (r *Runner) runOp(ctx context.Context, name string, op Op, migrator migrator) error {
	switch op {
	case OpInit:
		log.Info().Str("database_name", name).Msg("migration initialized")
		return nil
	case OpUp:
		group, err := migrator.Migrate(ctx)
		if err != nil {
			return err
		}
		log.Info().
			Str("database_name", name).
			Int("migration_count", migrationCount(group)).
			Msg("migration applied")
		return nil
	case OpDown:
		group, err := migrator.Rollback(ctx)
		if err != nil {
			return err
		}
		log.Info().
			Str("database_name", name).
			Int("migration_count", migrationCount(group)).
			Msg("migration rolled back")
		return nil
	default:
		return nil
	}
}

// migrationCount 返回 migration 分组内包含的 migration 数量。
func migrationCount(group *migrate.MigrationGroup) int {
	if group == nil {
		return 0
	}
	return len(group.Migrations)
}
