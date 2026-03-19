package bunlog

import (
	"context"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"github.com/uptrace/bun"
)

// QueryHookOptions 定义了 sql 执行钩子的配置
type QueryHookOptions struct {
	LogSlow time.Duration
}

// QueryHookOption 定义函数选项模式的闭包类型
type QueryHookOption func(*QueryHookOptions)

// WithLogSlow 设置记录慢查询的时间阈值
func WithLogSlow(d time.Duration) QueryHookOption {
	return func(o *QueryHookOptions) {
		o.LogSlow = d
	}
}

// QueryHook wraps query hook
type QueryHook struct {
	opts QueryHookOptions
}

// NewQueryHook 创建一个新的 QueryHook 实例，支持函数选项模式进行灵活配置
func NewQueryHook(opts ...QueryHookOption) *QueryHook {
	o := QueryHookOptions{}
	for _, opt := range opts {
		opt(&o)
	}
	return &QueryHook{opts: o}
}

// BeforeQuery does nothing tbh
func (h *QueryHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	return ctx
}

// AfterQuery convert a bun QueryEvent into a logrus message
func (h *QueryHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	maxQueryLength := viper.GetInt("log.sql.max_query_length")
	if !viper.GetBool("log.sql.traced") {
		return
	}

	now := time.Now()
	dur := now.Sub(event.StartTime)

	logger := log.Ctx(ctx).With().Str("op", eventOperation(event)).Dur("duration", dur).Logger()
	if maxQueryLength == 0 || len(event.Query) <= maxQueryLength {
		logger = logger.With().Str("sql", event.Query).Logger()
	}
	if event.Err != nil {
		logger.Error().Err(event.Err).Msg("query failed")
		return
	}
	if h.opts.LogSlow > 0 && dur > h.opts.LogSlow {
		logger.Warn().Msg("slow sql")
		return
	}
	logger.Debug().Msg("sql")
}

// taken from bun
func eventOperation(event *bun.QueryEvent) string {
	switch event.IQuery.(type) {
	case *bun.SelectQuery:
		return "SELECT"
	case *bun.InsertQuery:
		return "INSERT"
	case *bun.UpdateQuery:
		return "UPDATE"
	case *bun.DeleteQuery:
		return "DELETE"
	case *bun.CreateTableQuery:
		return "CREATE TABLE"
	case *bun.DropTableQuery:
		return "DROP TABLE"
	}
	return queryOperation(event.Query)
}

// taken from bun
func queryOperation(name string) string {
	if idx := strings.Index(name, " "); idx > 0 {
		name = name[:idx]
	}
	if len(name) > 16 {
		name = name[:16]
	}
	return string(name)
}
