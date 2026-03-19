package migration

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/iot/simhub/pkg/boot"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/migrate"
)

// TestRunnerUpSkipsUnregisteredAndDisabled 测试 Runner 只执行已注册且允许执行的数据库。
func TestRunnerUpSkipsUnregisteredAndDisabled(t *testing.T) {
	t.Parallel()

	dbStore := boot.NewDbStore()
	alphaDb := newTestBunDB()
	defer alphaDb.Close()
	betaDb := newTestBunDB()
	defer betaDb.Close()
	gammaDb := newTestBunDB()
	defer gammaDb.Close()

	dbStore.Set("alpha", alphaDb)
	dbStore.Set("beta", betaDb)
	dbStore.Set("gamma", gammaDb)

	alphaMs := migrate.NewMigrations()
	betaMs := migrate.NewMigrations()
	alphaMigrator := &fakeMigrator{}
	betaMigrator := &fakeMigrator{}

	runner := NewRunner(
		dbStore,
		WithShouldRun(func(name string) bool { return name != "beta" }),
		withMigratorFactory(newFakeMigratorFactory(map[*migrate.Migrations]*fakeMigrator{
			alphaMs: alphaMigrator,
			betaMs:  betaMigrator,
		})),
	)
	runner.MustRegister("alpha", alphaMs)
	runner.MustRegister("beta", betaMs)

	if err := runner.Up(context.Background()); err != nil {
		t.Fatalf("Up() error = %v", err)
	}

	if alphaMigrator.initCalls != 1 || alphaMigrator.lockCalls != 1 || alphaMigrator.migrateCalls != 1 || alphaMigrator.unlockCalls != 1 {
		t.Fatalf("alpha calls = %+v, want init/lock/migrate/unlock all 1", alphaMigrator)
	}
	if betaMigrator.initCalls != 0 || betaMigrator.lockCalls != 0 || betaMigrator.migrateCalls != 0 || betaMigrator.unlockCalls != 0 {
		t.Fatalf("beta calls = %+v, want all 0", betaMigrator)
	}
}

// TestRunnerInitDefaultsToRunAll 测试未提供 shouldRun 时默认执行全部已注册数据库。
func TestRunnerInitDefaultsToRunAll(t *testing.T) {
	t.Parallel()

	dbStore := boot.NewDbStore()
	mainDb := newTestBunDB()
	defer mainDb.Close()

	dbStore.Set("main", mainDb)

	ms := migrate.NewMigrations()
	migrator := &fakeMigrator{}
	runner := NewRunner(
		dbStore,
		withMigratorFactory(newFakeMigratorFactory(map[*migrate.Migrations]*fakeMigrator{
			ms: migrator,
		})),
	)
	runner.MustRegister("main", ms)

	if err := runner.Init(context.Background()); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if migrator.initCalls != 1 || migrator.lockCalls != 1 || migrator.unlockCalls != 1 {
		t.Fatalf("Init() calls = %+v, want init/lock/unlock all 1", migrator)
	}
	if migrator.migrateCalls != 0 || migrator.rollbackCalls != 0 {
		t.Fatalf("Init() should not call migrate or rollback, got %+v", migrator)
	}
}

// TestRunnerDown 测试 Down() 会调用 Rollback。
func TestRunnerDown(t *testing.T) {
	t.Parallel()

	dbStore := boot.NewDbStore()
	mainDb := newTestBunDB()
	defer mainDb.Close()

	dbStore.Set("main", mainDb)

	ms := migrate.NewMigrations()
	migrator := &fakeMigrator{}
	runner := NewRunner(
		dbStore,
		withMigratorFactory(newFakeMigratorFactory(map[*migrate.Migrations]*fakeMigrator{
			ms: migrator,
		})),
	)
	runner.MustRegister("main", ms)

	if err := runner.Down(context.Background()); err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	if migrator.rollbackCalls != 1 {
		t.Fatalf("rollbackCalls = %d, want 1", migrator.rollbackCalls)
	}
	if migrator.migrateCalls != 0 {
		t.Fatalf("migrateCalls = %d, want 0", migrator.migrateCalls)
	}
}

// TestRunnerReturnsTypeError 测试非 *bun.DB 的数据库实例会返回明确错误。
func TestRunnerReturnsTypeError(t *testing.T) {
	t.Parallel()

	dbStore := boot.NewDbStore()
	mainDb := newTestBunDB()
	defer mainDb.Close()

	conn, err := mainDb.Conn(context.Background())
	if err != nil {
		t.Fatalf("Conn() error = %v", err)
	}
	defer conn.Close()

	dbStore.Set("main", conn)

	runner := NewRunner(dbStore)
	runner.MustRegister("main", migrate.NewMigrations())

	err = runner.Up(context.Background())
	if err == nil {
		t.Fatal("Up() error = nil, want type assertion error")
	}
	if got, want := err.Error(), "database main is not *bun.DB"; got != want {
		t.Fatalf("Up() error = %q, want %q", got, want)
	}
}

// TestRunnerUpUnlocksAndJoinsErrors 测试操作失败时仍会解锁并合并错误。
func TestRunnerUpUnlocksAndJoinsErrors(t *testing.T) {
	t.Parallel()

	dbStore := boot.NewDbStore()
	mainDb := newTestBunDB()
	defer mainDb.Close()
	dbStore.Set("main", mainDb)

	ms := migrate.NewMigrations()
	runErr := errors.New("migrate failed")
	unlockErr := errors.New("unlock failed")
	migrator := &fakeMigrator{
		migrateErr: runErr,
		unlockErr:  unlockErr,
	}
	runner := NewRunner(
		dbStore,
		withMigratorFactory(newFakeMigratorFactory(map[*migrate.Migrations]*fakeMigrator{
			ms: migrator,
		})),
	)
	runner.MustRegister("main", ms)

	err := runner.Up(context.Background())
	if err == nil {
		t.Fatal("Up() error = nil, want joined error")
	}
	if !errors.Is(err, runErr) {
		t.Fatalf("errors.Is(err, runErr) = false, err = %v", err)
	}
	if !errors.Is(err, unlockErr) {
		t.Fatalf("errors.Is(err, unlockErr) = false, err = %v", err)
	}
	if migrator.unlockCalls != 1 {
		t.Fatalf("unlockCalls = %d, want 1", migrator.unlockCalls)
	}
}

// TestRunnerUpUsesSortedDbNames 测试多个数据库按名称排序后的顺序执行。
func TestRunnerUpUsesSortedDbNames(t *testing.T) {
	t.Parallel()

	dbStore := boot.NewDbStore()
	aDb := newTestBunDB()
	defer aDb.Close()
	bDb := newTestBunDB()
	defer bDb.Close()

	dbStore.Set("db_b", bDb)
	dbStore.Set("db_a", aDb)

	order := make([]string, 0, 2)
	aMs := migrate.NewMigrations()
	bMs := migrate.NewMigrations()
	runner := NewRunner(
		dbStore,
		withMigratorFactory(newFakeMigratorFactory(map[*migrate.Migrations]*fakeMigrator{
			aMs: &fakeMigrator{label: "db_a", order: &order},
			bMs: &fakeMigrator{label: "db_b", order: &order},
		})),
	)
	runner.MustRegister("db_a", aMs)
	runner.MustRegister("db_b", bMs)

	if err := runner.Up(context.Background()); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if !slices.Equal(order, []string{"db_a", "db_b"}) {
		t.Fatalf("execution order = %v, want [db_a db_b]", order)
	}
}

type fakeMigrator struct {
	label         string
	order         *[]string
	initErr       error
	lockErr       error
	unlockErr     error
	migrateErr    error
	rollbackErr   error
	initCalls     int
	lockCalls     int
	unlockCalls   int
	migrateCalls  int
	rollbackCalls int
}

// Init 记录 migration 初始化调用次数。
func (f *fakeMigrator) Init(context.Context) error {
	f.initCalls++
	return f.initErr
}

// Lock 记录 migration 加锁调用次数。
func (f *fakeMigrator) Lock(context.Context) error {
	f.lockCalls++
	return f.lockErr
}

// Unlock 记录 migration 解锁调用次数。
func (f *fakeMigrator) Unlock(context.Context) error {
	f.unlockCalls++
	return f.unlockErr
}

// Migrate 记录 migration 升级调用次数。
func (f *fakeMigrator) Migrate(context.Context, ...migrate.MigrationOption) (*migrate.MigrationGroup, error) {
	f.migrateCalls++
	if f.order != nil {
		*f.order = append(*f.order, f.label)
	}
	return &migrate.MigrationGroup{}, f.migrateErr
}

// Rollback 记录 migration 回滚调用次数。
func (f *fakeMigrator) Rollback(context.Context, ...migrate.MigrationOption) (*migrate.MigrationGroup, error) {
	f.rollbackCalls++
	return &migrate.MigrationGroup{}, f.rollbackErr
}

// newFakeMigratorFactory 构造测试用 migrator 工厂。
func newFakeMigratorFactory(items map[*migrate.Migrations]*fakeMigrator) migratorFactory {
	return func(db *bun.DB, ms *migrate.Migrations) migrator {
		return items[ms]
	}
}

// newTestBunDB 创建仅用于类型断言与工厂注入测试的 Bun DB。
func newTestBunDB() *bun.DB {
	return bun.NewDB(sql.OpenDB(testConnector{}), pgdialect.New())
}

type testConnector struct{}

// Connect 为 sql.OpenDB 提供测试连接。
func (testConnector) Connect(context.Context) (driver.Conn, error) {
	return testDriverConn{}, nil
}

// Driver 返回测试用 driver 实现。
func (testConnector) Driver() driver.Driver {
	return testDriver{}
}

type testDriver struct{}

// Open 创建一个测试用数据库连接。
func (testDriver) Open(string) (driver.Conn, error) {
	return testDriverConn{}, nil
}

type testDriverConn struct{}

// Prepare 返回一个不会真正执行 SQL 的测试语句。
func (testDriverConn) Prepare(string) (driver.Stmt, error) {
	return testStmt{}, nil
}

// Close 关闭测试连接。
func (testDriverConn) Close() error {
	return nil
}

// Begin 开启一个空实现的测试事务。
func (testDriverConn) Begin() (driver.Tx, error) {
	return testTx{}, nil
}

type testStmt struct{}

// Close 关闭测试语句。
func (testStmt) Close() error {
	return nil
}

// NumInput 返回未知数量的参数个数。
func (testStmt) NumInput() int {
	return -1
}

// Exec 返回一个空结果，用于满足 driver.Stmt 接口。
func (testStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}

// Query 返回一个空结果集，用于满足 driver.Stmt 接口。
func (testStmt) Query([]driver.Value) (driver.Rows, error) {
	return testRows{}, nil
}

type testRows struct{}

// Columns 返回空列集合。
func (testRows) Columns() []string {
	return nil
}

// Close 关闭测试结果集。
func (testRows) Close() error {
	return nil
}

// Next 立即返回 EOF，表示结果集为空。
func (testRows) Next([]driver.Value) error {
	return io.EOF
}

type testTx struct{}

// Commit 提交空事务。
func (testTx) Commit() error {
	return nil
}

// Rollback 回滚空事务。
func (testTx) Rollback() error {
	return nil
}
