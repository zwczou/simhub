package redlb

import "time"

// options 是 redlb 的全局配置。
type options struct {
	prefix            string
	ttl               time.Duration
	heartbeatInterval time.Duration
	operationTimeout  time.Duration
}

var defaultOptions = options{
	prefix:            "redlb:service",
	ttl:               12 * time.Second,
	heartbeatInterval: 4 * time.Second,
	operationTimeout:  time.Second,
}

// Option 用于配置 Registry。
type Option func(*options)

// WithPrefix 设置 Redis key 前缀，默认值为 "redlb:service"。
func WithPrefix(prefix string) Option {
	return func(o *options) {
		if prefix != "" {
			o.prefix = prefix
		}
	}
}

// WithTtl 设置注册信息在 Redis 中的过期时间。
func WithTtl(ttl time.Duration) Option {
	return func(o *options) {
		if ttl > 0 {
			o.ttl = ttl
		}
	}
}

// WithHeartbeatInterval 设置心跳续期间隔。
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(o *options) {
		if interval > 0 {
			o.heartbeatInterval = interval
		}
	}
}

// WithOperationTimeout 设置后台 Redis 操作的超时时间。
func WithOperationTimeout(timeout time.Duration) Option {
	return func(o *options) {
		if timeout > 0 {
			o.operationTimeout = timeout
		}
	}
}

// Registration 表示一次显式的服务注册请求。
type Registration struct {
	ServiceName string
	Ip          string
	GrpcAddr    string
	Hostname    string
	HttpAddr    string
	InstanceId  string
}
