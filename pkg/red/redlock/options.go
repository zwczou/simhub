package redlock

import "time"

// options 是锁的配置项
type options struct {
	prefix     string
	ttl        time.Duration
	retries    int
	retryDelay time.Duration
	value      string // 自定义锁值（测试用）
}

var defaultOptions = options{
	prefix:     "redlock",
	ttl:        10 * time.Second,
	retries:    3,
	retryDelay: 200 * time.Millisecond,
}

// Option 是配置函数
type Option func(*options)

// WithPrefix 设置 Redis key 前缀，默认 "redlock"
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithTtl 设置锁过期时间，默认 10 秒
func WithTtl(d time.Duration) Option {
	return func(o *options) { o.ttl = d }
}

// WithRetries 设置 Lock() 最大重试次数，默认 3
func WithRetries(n int) Option {
	return func(o *options) { o.retries = n }
}

// WithRetryDelay 设置重试间隔，默认 200ms
func WithRetryDelay(d time.Duration) Option {
	return func(o *options) { o.retryDelay = d }
}

// WithValue 自定义锁值（主要用于测试）
func WithValue(v string) Option {
	return func(o *options) { o.value = v }
}
