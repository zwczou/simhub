package simd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/iot/simhub/migrations/defaultmgt"
	"github.com/iot/simhub/pkg/boot"
	"github.com/iot/simhub/pkg/logger/bunlog"
	"github.com/iot/simhub/pkg/logger/otellog"
	"github.com/iot/simhub/pkg/migration"
	"github.com/iot/simhub/pkg/ratelimit"
	"github.com/iot/simhub/pkg/red/redlock"
	"github.com/iot/simhub/pkg/token"
	"github.com/iot/simhub/services/user"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bunotel"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
)

// init 初始化 simd 进程依赖的基础设施组件。
func (s *simServer) init() error {
	s.initZone()
	if err := s.initOtel(); err != nil {
		return err
	}
	if err := s.initTrace(); err != nil {
		return err
	}
	if err := s.initMetrics(); err != nil {
		return err
	}
	if err := s.initRedis(); err != nil {
		return err
	}
	if err := s.initRedisLock(); err != nil {
		return err
	}
	if err := s.initLimiter(); err != nil {
		return err
	}
	if err := s.initToken(); err != nil {
		return err
	}
	if err := s.initDatabase(); err != nil {
		return err
	}
	if err := s.initMigration(); err != nil {
		return err
	}
	return s.initServices()
}

// initZone 初始化进程默认时区。
func (s *simServer) initZone() {
	zone := viper.GetString("zone")
	if zone == "" {
		return
	}

	time.Local = lo.Must(time.LoadLocation(zone))
}

// initOtel 初始化 OpenTelemetry 的日志与错误处理器。
func (s *simServer) initOtel() error {
	otel.SetLogger(logr.New(otellog.New(8)))
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Error().Err(err).Msg("otel error")
	}))
	return nil
}

// initTrace 初始化链路追踪导出器与全局 TracerProvider。
func (s *simServer) initTrace() error {
	if !viper.GetBool("trace.enabled") {
		log.Info().Msg("trace disabled")
		return nil
	}

	endpoint := viper.GetString("trace.endpoint")
	if endpoint == "" {
		return fmt.Errorf("trace endpoint is empty")
	}

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithTimeout(5*time.Second),
	)
	if err != nil {
		return err
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(s.newOtelResource()),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	if err := s.boot.AddShutdown(provider.Shutdown); err != nil {
		_ = provider.Shutdown(context.Background())
		return err
	}
	log.Info().Str("trace_endpoint", endpoint).Msg("trace initialized")
	return nil
}

// initMetrics 初始化全局 MeterProvider，供 Redis 等组件注册指标。
func (s *simServer) initMetrics() error {
	if !viper.GetBool("metrics.enabled") {
		log.Info().Msg("metrics disabled")
		return nil
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(s.newOtelResource()),
	)
	otel.SetMeterProvider(provider)

	if err := s.boot.AddShutdown(provider.Shutdown); err != nil {
		_ = provider.Shutdown(context.Background())
		return err
	}
	log.Info().Msg("metrics initialized")
	return nil
}

// initRedis 初始化 Redis 客户端并注册到 RedisStore。
func (s *simServer) initRedis() error {
	for name := range viper.GetStringMap("redis") {
		sub := viper.Sub("redis." + name)
		if sub == nil {
			continue
		}

		addr := sub.GetString("addr")
		if addr == "" {
			return fmt.Errorf("redis %s addr is empty", name)
		}

		dbs := sub.GetIntSlice("dbs")
		if len(dbs) == 0 {
			dbs = []int{0}
		}

		for _, db := range dbs {
			client := redis.NewClient(&redis.Options{
				Addr:         addr,
				Password:     sub.GetString("password"),
				DB:           db,
				PoolSize:     sub.GetInt("pool_size"),
				DialTimeout:  3 * time.Second,
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
			})

			// 注册 tracing hook，确保 Redis 操作纳入链路追踪。
			if err := redisotel.InstrumentTracing(client); err != nil {
				_ = client.Close()
				return err
			}

			// 注册 metrics hook，便于后续统一采集连接池与命令指标。
			if err := redisotel.InstrumentMetrics(client); err != nil {
				_ = client.Close()
				return err
			}

			s.rdbs.Set(name, db, client)
			if err := s.boot.AddShutdown(func(context.Context) error {
				return client.Close()
			}); err != nil {
				_ = client.Close()
				return err
			}

			log.Info().
				Str("redis_name", name).
				Int("redis_db", db).
				Str("redis_addr", addr).
				Msg("redis initialized")
		}
	}
	return nil
}

// initRedisLock 初始化基于 Redis 的分布式锁管理器。
func (s *simServer) initRedisLock() error {
	rdb, ok := s.rdbs.Get("default")
	if !ok {
		return fmt.Errorf("redis default is not configured")
	}

	opts, err := s.newRedLockOptions()
	if err != nil {
		return err
	}

	s.rdlock = redlock.NewRedLock(rdb, opts...)
	log.Info().Msg("redis lock initialized")
	return nil
}

// initLimiter 初始化基于 Redis 的限流器并注册关闭钩子。
func (s *simServer) initLimiter() error {
	rdb, ok := s.rdbs.Get("limiter")
	if !ok {
		return fmt.Errorf("redis limiter is not configured")
	}

	opts, err := s.newLimiterOptions()
	if err != nil {
		return err
	}

	limiter, err := ratelimit.NewRedisRateLimiter(context.Background(), rdb, opts...)
	if err != nil {
		return err
	}

	if err := s.boot.AddShutdown(func(context.Context) error {
		limiter.Close()
		return nil
	}); err != nil {
		limiter.Close()
		return err
	}

	s.limiter = limiter
	log.Info().Msg("limiter initialized")
	return nil
}

// initToken 初始化前台与后台的 Token 管理器。
func (s *simServer) initToken() error {
	rdb, ok := s.rdbs.Get("user")
	if !ok {
		return fmt.Errorf("redis user is not configured")
	}

	userOpts, err := s.newTokenOptions("token.user")
	if err != nil {
		return err
	}
	manOpts, err := s.newTokenOptions("token.man")
	if err != nil {
		return err
	}

	s.userToken = token.NewUserToken(rdb, userOpts...)
	s.manToken = token.NewManToken(rdb, manOpts...)

	log.Info().Msg("token initialized")
	return nil
}

// newLimiterOptions 按配置构造限流器的可选项。
func (s *simServer) newLimiterOptions() ([]ratelimit.Option, error) {
	sub := viper.Sub("ratelimit")
	if sub == nil {
		return nil, nil
	}

	opts := make([]ratelimit.Option, 0, 3)
	if prefix := sub.GetString("prefix"); prefix != "" {
		opts = append(opts, ratelimit.WithPrefix(prefix))
	}

	reloadPeriod := sub.GetDuration("reload_period")
	if reloadPeriod > 0 {
		opts = append(opts, ratelimit.WithReloadPeriod(reloadPeriod))
	}

	if failurePolicy := sub.GetString("failure_policy"); failurePolicy != "" {
		opts = append(opts, ratelimit.WithFailurePolicy(ratelimit.FailurePolicy(failurePolicy)))
	}

	return opts, nil
}

// newRedLockOptions 按配置构造分布式锁的可选项。
func (s *simServer) newRedLockOptions() ([]redlock.Option, error) {
	sub := viper.Sub("redlock")
	if sub == nil {
		return nil, nil
	}

	opts := make([]redlock.Option, 0, 4)
	if prefix := sub.GetString("prefix"); prefix != "" {
		opts = append(opts, redlock.WithPrefix(prefix))
	}

	if sub.IsSet("ttl") {
		ttl := sub.GetDuration("ttl")
		if ttl == 0 {
			return nil, fmt.Errorf("redlock ttl is empty")
		}
		opts = append(opts, redlock.WithTtl(ttl))
	}

	if sub.IsSet("retries") {
		opts = append(opts, redlock.WithRetries(sub.GetInt("retries")))
	}

	if sub.IsSet("retry_delay") {
		retryDelay := sub.GetDuration("retry_delay")
		if retryDelay == 0 {
			return nil, fmt.Errorf("redlock retry_delay is empty")
		}
		opts = append(opts, redlock.WithRetryDelay(retryDelay))
	}

	return opts, nil
}

// newTokenOptions 按配置构造 Token 管理器的可选项。
func (s *simServer) newTokenOptions(path string) ([]token.Option, error) {
	sub := viper.Sub(path)
	if sub == nil {
		return nil, nil
	}

	opts := make([]token.Option, 0, 3)

	accessTtl := sub.GetDuration("token_ttl")
	if accessTtl == 0 {
		return nil, fmt.Errorf("%s token_ttl is empty", path)
	}
	opts = append(opts, token.WithAccessTtl(accessTtl))

	refreshTtl := sub.GetDuration("refresh_ttl")
	if refreshTtl == 0 {
		return nil, fmt.Errorf("%s refresh_ttl is empty", path)
	}
	opts = append(opts, token.WithRefreshTtl(refreshTtl))

	if sub.GetBool("unique") {
		opts = append(opts, token.WithUnique())
	}

	return opts, nil
}

// initDatabase 初始化数据库连接并注册到 DbStore。
func (s *simServer) initDatabase() error {
	for name := range viper.GetStringMap("database") {
		sub := viper.Sub("database." + name)
		if sub == nil {
			continue
		}

		dsn := sub.GetString("dsn")
		if dsn == "" {
			return fmt.Errorf("database %s dsn is empty", name)
		}

		sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
		db := bun.NewDB(sqldb, pgdialect.New())
		db.AddQueryHook(bunotel.NewQueryHook(bunotel.WithDBName("splay")))
		db.AddQueryHook(bunlog.NewQueryHook(
			bunlog.WithLogSlow(sub.GetDuration("slow")),
		))

		s.dbs.Set(name, db)
		if err := s.boot.AddShutdown(func(context.Context) error {
			return db.Close()
		}); err != nil {
			_ = db.Close()
			return err
		}

		log.Info().Str("database_name", name).Msg("database initialized")
	}
	return nil
}

// initServices 按配置注册需要加载的服务。
func (s *simServer) initServices() error {
	services, err := s.newConfiguredServices()
	if err != nil {
		return err
	}
	if len(services) == 0 {
		return nil
	}

	if err := s.boot.Register(services...); err != nil {
		return err
	}

	log.Info().Int("service_count", len(services)).Msg("services registered")
	return nil
}

// newConfiguredServices 根据配置构造待注册的服务列表。
func (s *simServer) newConfiguredServices() ([]boot.Service, error) {
	factories := map[string]func() boot.Service{
		"sim.user": user.NewService,
	}

	configured := viper.GetStringSlice("services")
	services := make([]boot.Service, 0, len(configured))
	for _, name := range configured {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		factory, ok := factories[name]
		if !ok {
			return nil, fmt.Errorf("service %s is not supported", name)
		}
		services = append(services, factory())
	}

	return services, nil
}

// shouldRunMigration 判断指定数据库是否启用迁移流程。
func (s *simServer) shouldRunMigration(name string) bool {
	sub := viper.Sub("database." + name)
	if sub == nil {
		return true
	}
	if sub.IsSet("migrate") && !sub.GetBool("migrate") {
		return false
	}
	return true
}

// initMigration 根据启动参数执行数据库初始化、迁移或回滚。
func (s *simServer) initMigration() error {
	op := migration.DetectOp(os.Args)
	if op == migration.OpNone {
		return nil
	}
	defer os.Exit(0)

	ctx := context.Background()
	runner := migration.NewRunner(
		s.dbs,
		migration.WithShouldRun(s.shouldRunMigration),
	)
	runner.MustRegister("default", defaultmgt.Migrations)

	switch op {
	case migration.OpInit:
		if err := runner.Init(ctx); err != nil {
			return err
		}
	case migration.OpUp:
		if err := runner.Up(ctx); err != nil {
			return err
		}
	case migration.OpDown:
		if err := runner.Down(ctx); err != nil {
			return err
		}
	}

	log.Info().Str("migration_op", string(op)).Msg("migration finished")
	return nil
}

// newOtelResource 构造当前进程的 OpenTelemetry 资源标签。
func (s *simServer) newOtelResource() *resource.Resource {
	info := boot.Read()
	serviceName := viper.GetString("name")
	if serviceName == "" {
		serviceName = "simd"
	}

	instanceId := serviceName
	if info.Hostname != "" {
		instanceId = info.Hostname + "." + serviceName
	}

	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(serviceName),
		semconv.ServiceVersionKey.String(info.Version),
		semconv.ServiceInstanceIDKey.String(instanceId),
	}
	if mode := viper.GetString("mode"); mode != "" {
		attrs = append(attrs, attribute.String("env", mode))
	}

	return resource.NewSchemaless(attrs...)
}
