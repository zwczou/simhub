package redlb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/attributes"
	"google.golang.org/grpc/resolver"
)

const (
	// Scheme 是 redlb 的 gRPC resolver scheme。
	Scheme = "redlb"
)

// RegisterGrpcResolver 注册全局 gRPC resolver，可配合 grpc.Dial("redlb:///service.name") 使用。
func RegisterGrpcResolver(registry *Registry) {
	resolver.Register(NewGrpcResolverBuilder(registry))
}

// NewGrpcResolverBuilder 创建 redlb 的 gRPC resolver builder。
func NewGrpcResolverBuilder(registry *Registry) resolver.Builder {
	return &grpcBuilder{registry: registry}
}

// grpcBuilder 是 gRPC resolver builder 实现。
type grpcBuilder struct {
	registry *Registry
}

// Build 创建 resolver 实例并立即进行一次服务发现。
func (b *grpcBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	serviceName := parseServiceName(target)
	if serviceName == "" {
		return nil, fmt.Errorf("redlb: empty service name in target %q", target.URL.String())
	}

	r := &grpcResolver{
		registry:    b.registry,
		serviceName: serviceName,
		cc:          cc,
		closeCh:     make(chan struct{}),
	}
	r.refresh()
	go r.watch()
	return r, nil
}

// Scheme 返回 resolver scheme。
func (b *grpcBuilder) Scheme() string {
	return Scheme
}

// grpcResolver 负责把 Discover 结果更新到 gRPC ClientConn。
type grpcResolver struct {
	registry    *Registry
	serviceName string
	cc          resolver.ClientConn
	closeCh     chan struct{}
	closeOnce   sync.Once
}

// ResolveNow 触发一次立即刷新。
func (r *grpcResolver) ResolveNow(_ resolver.ResolveNowOptions) {
	r.refresh()
}

// Close 停止后台刷新协程。
func (r *grpcResolver) Close() {
	r.closeOnce.Do(func() {
		close(r.closeCh)
	})
}

// watch 以固定周期刷新服务列表。
func (r *grpcResolver) watch() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.closeCh:
			return
		case <-ticker.C:
			r.refresh()
		}
	}
}

// refresh 拉取最新实例并更新到 gRPC 连接状态。
func (r *grpcResolver) refresh() {
	ctx, cancel := withOperationTimeout(context.Background(), r.registry.opt.operationTimeout)
	defer cancel()

	endpoints, err := r.registry.Discover(ctx, r.serviceName)
	if err != nil {
		r.cc.ReportError(err)
		return
	}

	addresses := make([]resolver.Address, 0, len(endpoints))
	for _, ep := range endpoints {
		attrs := attributes.New("instance_id", ep.InstanceId).
			WithValue("hostname", ep.Hostname).
			WithValue("http_port", strconv.Itoa(ep.HttpPort))
		addresses = append(addresses, resolver.Address{
			Addr:       ep.GrpcAddress(),
			Attributes: attrs,
		})
	}

	if err = r.cc.UpdateState(resolver.State{Addresses: addresses}); err != nil {
		r.cc.ReportError(err)
	}
}

// parseServiceName 解析 resolver target 中的服务名。
//
// 同时支持：
// - redlb:///service.name
// - redlb://service.name
func parseServiceName(target resolver.Target) string {
	if target.URL.Host != "" {
		return strings.TrimSpace(target.URL.Host)
	}
	return strings.Trim(strings.TrimSpace(target.URL.Path), "/")
}
