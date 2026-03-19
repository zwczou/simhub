package simd

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iot/simhub/pkg/boot"
	"github.com/rs/zerolog/log"
)

// simServer 是 simd 的核心服务器结构，负责管理服务生命周期、处理请求和协调各个组件
type simServer struct {
	mu       sync.RWMutex
	wg       sync.WaitGroup
	opts     *option
	rdbs     *boot.RedisStore
	dbs      *boot.DbStore
	boot     *boot.Boot
	startAt  time.Time
	isClosed atomic.Bool
	exitChan chan struct{}
}

// New 创建并返回一个新的 simServer 实例
func New(opts *option) *simServer {
	return &simServer{
		opts:     opts,
		boot:     boot.NewBoot(),
		dbs:      boot.NewDbStore(),
		rdbs:     boot.NewRedisStore(),
		exitChan: make(chan struct{}),
	}
}

func (s *simServer) Main() {
	err := s.init()
	if err != nil {
		log.Fatal().Err(err).Msg("init error")
	}

	s.boot.Provide(s.boot, s.dbs, s.rdbs)
	s.boot.Load(context.Background())
}

func (s *simServer) Exit() {
	start := time.Now()

	// 关闭服务，通知其他监听函数
	close(s.exitChan)

	// 关闭API服务，此时API返回维护中错误
	s.isClosed.Store(true)

	s.boot.Unload(context.Background())
	spent := time.Since(start).Seconds() * 1e3
	log.Info().Float64("spent", spent).Msg("server exited")

	// 在等待一会儿，确保所有的请求都处理完毕
	time.Sleep(time.Millisecond * 200)

	// 关闭redis连接
	for _, rdb := range s.rdbs.Items() {
		rdb.Close()
	}
}
