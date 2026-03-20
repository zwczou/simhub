package simd

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iot/simhub/pkg/boot"
	"github.com/iot/simhub/pkg/ratelimit"
	"github.com/iot/simhub/pkg/red/redlock"
	"github.com/iot/simhub/pkg/token"
	"github.com/rs/zerolog/log"
)

// simServer 是 simd 的核心服务器结构，负责管理服务生命周期、处理请求和协调各个组件
type simServer struct {
	mu        sync.RWMutex
	wg        sync.WaitGroup
	opts      *option
	boot      *boot.Boot
	rdbs      *boot.RedisStore
	dbs       *boot.DbStore
	limiter   *ratelimit.RedisRateLimiter
	rdlock    *redlock.RedLock
	userToken *token.UserToken
	manToken  *token.ManToken
	startAt   time.Time
	isClosed  atomic.Bool
	exitChan  chan struct{}
}

// New 创建并返回一个新的 simServer 实例
func New(opts *option) *simServer {
	return &simServer{
		opts:     opts,
		boot:     boot.GetBoot(),
		dbs:      boot.NewDbStore(),
		rdbs:     boot.NewRedisStore(),
		exitChan: make(chan struct{}),
	}
}

// Main 启动 simd 服务并加载已注册的生命周期组件。
func (s *simServer) Main() {
	// 初始化
	err := s.init()
	if err != nil {
		log.Fatal().Err(err).Msg("init error")
	}

	// 注册依赖变量
	s.boot.Provide(s.boot, s.dbs, s.rdbs, s.limiter, s.userToken, s.manToken, s.rdlock)

	// 加载注册的服务
	s.boot.Load(context.Background())
}

// Exit 停止 simd 服务并释放进程内持有的基础设施资源。
func (s *simServer) Exit() {
	start := time.Now()

	// 关闭服务，通知其他监听函数
	close(s.exitChan)

	// 关闭API服务，此时API返回维护中错误
	s.isClosed.Store(true)

	// 卸载注册的服务
	if err := s.boot.Unload(context.Background()); err != nil {
		log.Error().Err(err).Msg("unload failed")
	}

	// 关闭Publish/Subscribe
	s.boot.Close()

	// 在等待一会儿，确保所有的请求都处理完毕
	time.Sleep(time.Millisecond * 200)

	// 在服务完全下线后，再做最终资源清理。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.boot.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("shutdown failed")
	}

	log.Info().Dur("spent", time.Since(start)).Msg("server exited")
}
