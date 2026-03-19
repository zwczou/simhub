package simd

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/iot/simhub/pkg/boot"
	"github.com/iot/simhub/pkg/logger/bunlog"
	"github.com/iot/simhub/pkg/logger/otellog"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
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
	return s.initDatabase()
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
