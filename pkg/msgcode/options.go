package msgcode

import "time"

// options 是 Manager 的配置项。
type options struct {
	prefix  string
	codeTtl time.Duration
	loc     *time.Location
	nowFn   func() time.Time
}

var defaultOptions = options{
	prefix:  "msgcode",
	codeTtl: 5 * time.Minute,
	loc:     time.Local,
	nowFn:   time.Now,
}

// Option 是 Manager 的配置函数。
type Option func(*options)

// WithPrefix 设置 Redis key 前缀，默认 "msgcode"。
func WithPrefix(prefix string) Option {
	return func(o *options) { o.prefix = prefix }
}

// WithCodeTtl 设置验证码有效期，默认 5 分钟。
func WithCodeTtl(d time.Duration) Option {
	return func(o *options) { o.codeTtl = d }
}

// WithLocation 设置统计窗口使用的时区，默认 time.Local。
func WithLocation(loc *time.Location) Option {
	return func(o *options) {
		if loc != nil {
			o.loc = loc
		}
	}
}

// withNowFunc 设置当前时间函数，仅用于测试。
func withNowFunc(fn func() time.Time) Option {
	return func(o *options) {
		if fn != nil {
			o.nowFn = fn
		}
	}
}
