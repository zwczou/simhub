package user

import (
	"context"
	"sync"

	"github.com/iot/simhub/apis/userpb"
	"github.com/iot/simhub/pkg/boot"
)

// userServer 是用户服务的生命周期承载对象。
type userServer struct {
	userpb.UnimplementedUserServerServer
	mu       sync.RWMutex
	wg       sync.WaitGroup
	name     string
	exitChan chan struct{}
}

// NewService 创建一个新的用户服务实例。
func NewService() boot.Service {
	return newUserServer()
}

// newUserServer 创建用户服务实例。
func newUserServer() *userServer {
	return &userServer{
		name:     "sim.user",
		exitChan: make(chan struct{}),
	}
}

// Name 返回服务名称。
func (s *userServer) Name() string {
	return s.name
}

// Load 加载用户服务。
func (s *userServer) Load(ctx context.Context) error {
	return nil
}

// Unload 卸载用户服务。
func (s *userServer) Unload(ctx context.Context) error {
	s.mu.Lock()
	close(s.exitChan)
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}
