package migration

// Option 定义 Runner 的可选配置项。
type Option func(*options)

type options struct {
	registry    *Registry
	shouldRun   func(name string) bool
	newMigrator migratorFactory
}

// defaultOptions 返回 Runner 的默认配置。
func defaultOptions() *options {
	return &options{
		registry:    NewRegistry(),
		newMigrator: defaultMigratorFactory,
	}
}

// WithRegistry 指定 Runner 使用的 migration 注册表。
func WithRegistry(registry *Registry) Option {
	return func(opts *options) {
		if registry != nil {
			opts.registry = registry
		}
	}
}

// WithShouldRun 指定数据库是否参与 migration 的判断函数。
func WithShouldRun(fn func(name string) bool) Option {
	return func(opts *options) {
		opts.shouldRun = fn
	}
}

// withMigratorFactory 为测试注入自定义 migrator 构造器。
func withMigratorFactory(factory migratorFactory) Option {
	return func(opts *options) {
		if factory != nil {
			opts.newMigrator = factory
		}
	}
}
