package redlb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	json "github.com/goccy/go-json"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var (
	// ErrServiceNameRequired 表示服务名为空。
	ErrServiceNameRequired = errors.New("redlb: service name is required")
	// ErrIpRequired 表示无法确定注册 IP。
	ErrIpRequired = errors.New("redlb: ip is required")
	// ErrGrpcAddrRequired 表示未配置 grpc.addr。
	ErrGrpcAddrRequired = errors.New("redlb: grpc addr is required")
)

// Endpoint 表示 Redis 中保存的服务实例信息。
type Endpoint struct {
	ServiceName string `json:"service_name"`
	InstanceId  string `json:"instance_id"`
	Ip          string `json:"ip"`
	Hostname    string `json:"hostname,omitempty"`
	GrpcPort    int    `json:"grpc_port"`
	HttpPort    int    `json:"http_port,omitempty"`
	UpdatedAt   int64  `json:"updated_at"`
}

// GrpcAddress 返回实例的 gRPC 地址，格式为 host:port。
func (e Endpoint) GrpcAddress() string {
	return net.JoinHostPort(e.Ip, strconv.Itoa(e.GrpcPort))
}

// Registry 提供服务注册与服务发现能力。
type Registry struct {
	rdb redis.UniversalClient
	opt options
}

// Lease 表示一个正在续期的注册实例。
type Lease struct {
	key              string
	indexKey         string
	rdb              redis.UniversalClient
	operationTimeout time.Duration
	stopCh           chan struct{}
	once             sync.Once
	doneCh           chan struct{}
}

// NewRegistry 创建 Registry。
func NewRegistry(rdb redis.UniversalClient, opts ...Option) *Registry {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}
	return &Registry{rdb: rdb, opt: o}
}

// Register 将实例注册到 Redis 并自动续期。
//
// 该方法会：
// 1. 使用显式传入的注册信息构造实例信息。
// 2. 把实例信息写入 Redis 并设置过期时间（Ttl）。
// 3. 按心跳间隔持续续期。
// 4. Stop 时主动删除 key。
func (r *Registry) Register(ctx context.Context, req Registration) (*Lease, Endpoint, error) {
	req.ServiceName = r.normalizeServiceName(req.ServiceName)
	if req.ServiceName == "" {
		return nil, Endpoint{}, ErrServiceNameRequired
	}

	ep, key, err := r.buildEndpoint(req)
	if err != nil {
		return nil, Endpoint{}, err
	}
	indexKey := r.indexKey(req.ServiceName)
	if err = r.upsert(ctx, key, indexKey, ep); err != nil {
		return nil, Endpoint{}, err
	}

	lease := &Lease{
		key:              key,
		indexKey:         indexKey,
		rdb:              r.rdb,
		operationTimeout: r.opt.operationTimeout,
		stopCh:           make(chan struct{}),
		doneCh:           make(chan struct{}),
	}
	go r.keepAlive(ctx, lease, ep)
	return lease, ep, nil
}

// Discover 查询指定服务的所有在线实例。
func (r *Registry) Discover(ctx context.Context, serviceName string) ([]Endpoint, error) {
	serviceName = r.normalizeServiceName(serviceName)
	if serviceName == "" {
		return nil, ErrServiceNameRequired
	}

	keys, err := r.rdb.SMembers(ctx, r.indexKey(serviceName)).Result()
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}

	values, err := r.rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	endpoints := make([]Endpoint, 0, len(values))
	staleKeys := make([]string, 0)
	for i, raw := range values {
		if raw == nil {
			staleKeys = append(staleKeys, keys[i])
			continue
		}
		str, ok := raw.(string)
		if !ok {
			staleKeys = append(staleKeys, keys[i])
			continue
		}
		var ep Endpoint
		if err = json.Unmarshal([]byte(str), &ep); err != nil {
			staleKeys = append(staleKeys, keys[i])
			continue
		}
		if ep.ServiceName != serviceName || ep.Ip == "" || ep.GrpcPort <= 0 {
			staleKeys = append(staleKeys, keys[i])
			continue
		}
		endpoints = append(endpoints, ep)
	}

	r.cleanupStaleKeys(ctx, serviceName, staleKeys)

	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].InstanceId < endpoints[j].InstanceId
	})
	return endpoints, nil
}

// Stop 停止续期并删除 Redis key。
func (l *Lease) Stop(ctx context.Context) error {
	l.once.Do(func() {
		close(l.stopCh)
		<-l.doneCh
	})
	if l.rdb == nil || l.key == "" {
		return nil
	}
	opCtx, cancel := withOperationTimeout(ctx, l.operationTimeout)
	defer cancel()

	_, err := l.rdb.TxPipelined(opCtx, func(pipe redis.Pipeliner) error {
		pipe.Del(opCtx, l.key)
		if l.indexKey != "" {
			pipe.SRem(opCtx, l.indexKey, l.key)
		}
		return nil
	})
	return err
}

// buildEndpoint 组装注册信息并返回对应的 Redis key。
func (r *Registry) buildEndpoint(req Registration) (Endpoint, string, error) {
	ip := strings.TrimSpace(req.Ip)
	if ip == "" {
		return Endpoint{}, "", ErrIpRequired
	}

	hostname := strings.TrimSpace(req.Hostname)
	if hostname == "" {
		hostname, _ = os.Hostname()
		hostname = strings.TrimSpace(hostname)
	}

	grpcAddr := strings.TrimSpace(req.GrpcAddr)
	if strings.TrimSpace(grpcAddr) == "" {
		return Endpoint{}, "", ErrGrpcAddrRequired
	}
	httpAddr := strings.TrimSpace(req.HttpAddr)

	grpcPort, err := splitHostPortPort(grpcAddr)
	if err != nil {
		return Endpoint{}, "", fmt.Errorf("redlb: parse grpc.addr failed: %w", err)
	}

	httpPort := 0
	if strings.TrimSpace(httpAddr) != "" {
		httpPort, err = splitHostPortPort(httpAddr)
		if err != nil {
			return Endpoint{}, "", fmt.Errorf("redlb: parse http.addr failed: %w", err)
		}
	}

	instanceId := strings.TrimSpace(req.InstanceId)
	if instanceId == "" {
		instanceId = fmt.Sprintf("%s-%s-%d-%d-%d", hostname, ip, grpcPort, httpPort, os.Getpid())
	}

	ep := Endpoint{
		ServiceName: req.ServiceName,
		InstanceId:  instanceId,
		Ip:          ip,
		Hostname:    hostname,
		GrpcPort:    grpcPort,
		HttpPort:    httpPort,
		UpdatedAt:   time.Now().Unix(),
	}
	return ep, r.serviceKey(req.ServiceName, instanceId), nil
}

// keepAlive 负责心跳续期，直到 ctx 取消或 lease.Stop 被调用。
func (r *Registry) keepAlive(ctx context.Context, lease *Lease, endpoint Endpoint) {
	defer close(lease.doneCh)

	interval := r.opt.heartbeatInterval
	if interval <= 0 {
		interval = 4 * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-lease.stopCh:
			return
		case <-ticker.C:
			endpoint.UpdatedAt = time.Now().Unix()
			heartbeatCtx, cancel := withOperationTimeout(context.Background(), r.opt.operationTimeout)
			err := r.upsert(heartbeatCtx, lease.key, lease.indexKey, endpoint)
			cancel()
			if err != nil {
				log.Ctx(ctx).
					Warn().
					Err(err).
					Str("service_name", endpoint.ServiceName).
					Str("instance_id", endpoint.InstanceId).
					Msg("redlb keepalive failed")
			}
		}
	}
}

// upsert 把实例信息写入 Redis 并设置过期时间。
func (r *Registry) upsert(ctx context.Context, key string, indexKey string, endpoint Endpoint) error {
	body, err := json.Marshal(endpoint)
	if err != nil {
		return err
	}
	_, err = r.rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, key, body, r.opt.ttl)
		if indexKey != "" {
			pipe.SAdd(ctx, indexKey, key)
			pipe.PExpire(ctx, indexKey, r.opt.ttl)
		}
		return nil
	})
	return err
}

// serviceKey 返回单个实例的 Redis key。
func (r *Registry) serviceKey(serviceName, instanceId string) string {
	return fmt.Sprintf("%s:%s:%s", r.opt.prefix, serviceName, instanceId)
}

// indexKey 返回服务实例索引的 Redis key。
func (r *Registry) indexKey(serviceName string) string {
	return fmt.Sprintf("%s:index:%s", r.opt.prefix, serviceName)
}

// splitHostPortPort 从监听地址中提取端口。
//
// 该函数基于 net.SplitHostPort，仅支持如下形式：
// - ":9988"
// - "127.0.0.1:9988"
// - "[::1]:9988"
func splitHostPortPort(addr string) (int, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0, errors.New("redlb: empty address")
	}

	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("redlb: split host port failed: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("redlb: invalid port %q", portStr)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("redlb: port out of range: %d", port)
	}
	return port, nil
}

// normalizeServiceName 归一化服务名。
func (r *Registry) normalizeServiceName(serviceName string) string {
	return strings.TrimSpace(serviceName)
}

// cleanupStaleKeys 尝试从索引集合中移除已经失效的实例 key。
func (r *Registry) cleanupStaleKeys(ctx context.Context, serviceName string, keys []string) {
	if len(keys) == 0 {
		return
	}

	if err := r.rdb.SRem(ctx, r.indexKey(serviceName), stringSliceToAny(keys)...).Err(); err != nil {
		log.Ctx(ctx).
			Warn().
			Err(err).
			Str("service_name", serviceName).
			Int("stale_key_count", len(keys)).
			Msg("redlb cleanup stale keys failed")
	}
}

// withOperationTimeout 为 Redis 后台操作补上统一超时控制。
func withOperationTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// stringSliceToAny 将字符串切片转换为 Redis 可接受的可变参数切片。
func stringSliceToAny(values []string) []any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return items
}
