package msgcode

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// SendStat 表示某个 key 在当前小时和当天的发送统计。
type SendStat struct {
	HourlyCount int64 `json:"hourly_count"`
	DailyCount  int64 `json:"daily_count"`
}

// SendResult 是 Send 的返回结果。
type SendResult struct {
	Value *CodeValue `json:"value"`
	SendStat
}

// Manager 是 Redis 驱动的验证码管理器。
type Manager struct {
	rdb redis.UniversalClient
	opt options
}

// NewManager 创建一个验证码管理器。
func NewManager(rdb redis.UniversalClient, opts ...Option) *Manager {
	o := defaultOptions
	for _, fn := range opts {
		fn(&o)
	}
	return &Manager{rdb: rdb, opt: o}
}

// Send 发送验证码并落库，同时累计当前小时和当天发送次数。
func (m *Manager) Send(ctx context.Context, key string, codeType int16, code int) (*SendResult, error) {
	now := m.now()
	cv := &CodeValue{
		Type:      codeType,
		Code:      code,
		Key:       key,
		CreatedAt: now.Unix(),
		CodeTtl:   int64(m.opt.codeTtl.Seconds()),
	}
	return m.SendValue(ctx, cv)
}

// SendValue 使用给定的 CodeValue 发送验证码并落库，同时累计当前小时和当天发送次数。
func (m *Manager) SendValue(ctx context.Context, cv *CodeValue) (*SendResult, error) {
	if cv == nil {
		return nil, ErrCodeValueNil
	}

	now := m.now().In(m.opt.loc)
	if cv.CreatedAt == 0 {
		cv.CreatedAt = now.Unix()
	}
	if cv.CodeTtl <= 0 {
		cv.CodeTtl = int64(m.opt.codeTtl.Seconds())
	}

	codeKey := m.codeKey(cv.Type, cv.Key)
	hourKey := m.hourKey(cv.Type, cv.Key, now)
	dayKey := m.dayKey(cv.Type, cv.Key, now)

	pipe := m.rdb.Pipeline()
	pipe.Set(ctx, codeKey, cv, m.opt.codeTtl)
	hourCmd := pipe.Incr(ctx, hourKey)
	dayCmd := pipe.Incr(ctx, dayKey)
	pipe.Expire(ctx, hourKey, 2*time.Hour)
	pipe.Expire(ctx, dayKey, 48*time.Hour)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("msgcode: send failed: %w", err)
	}

	log.Ctx(ctx).Info().Str("key", cv.Key).Int16("type", cv.Type).Msg("send message")

	return &SendResult{
		Value: cv,
		SendStat: SendStat{
			HourlyCount: hourCmd.Val(),
			DailyCount:  dayCmd.Val(),
		},
	}, nil
}

// Get 获取某个 key + type 当前有效的验证码。
func (m *Manager) Get(ctx context.Context, key string, codeType int16) (*CodeValue, error) {
	var cv CodeValue
	if err := m.rdb.Get(ctx, m.codeKey(codeType, key)).Scan(&cv); err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCodeNotFound
		}
		return nil, fmt.Errorf("msgcode: get failed: %w", err)
	}
	return &cv, nil
}

// Verify 校验验证码，仅校验不删除。
// 建议在外部业务流程成功后再调用 Delete 删除验证码。
func (m *Manager) Verify(ctx context.Context, key string, codeType int16, code int) (*CodeValue, error) {
	cv, err := m.Get(ctx, key, codeType)
	if err != nil {
		return nil, err
	}
	if err := cv.ValidAt(codeType, code, m.now()); err != nil {
		return nil, err
	}

	log.Ctx(ctx).Info().Str("key", key).Int16("type", codeType).Msg("verify message")
	return cv, nil
}

// Delete 删除某个 key + type 的验证码。
func (m *Manager) Delete(ctx context.Context, key string, codeType int16) error {
	if err := m.rdb.Del(ctx, m.codeKey(codeType, key)).Err(); err != nil {
		return fmt.Errorf("msgcode: delete code failed: %w", err)
	}
	return nil
}

// Stat 返回某个 key + type 在当前小时和当天发送次数。
func (m *Manager) Stat(ctx context.Context, key string, codeType int16) (*SendStat, error) {
	now := m.now().In(m.opt.loc)
	hourly, err := m.CountHour(ctx, key, codeType, now)
	if err != nil {
		return nil, err
	}
	daily, err := m.CountDay(ctx, key, codeType, now)
	if err != nil {
		return nil, err
	}
	return &SendStat{HourlyCount: hourly, DailyCount: daily}, nil
}

// CountHour 返回某个 key + type 在指定时刻所在小时的发送次数。
func (m *Manager) CountHour(ctx context.Context, key string, codeType int16, at time.Time) (int64, error) {
	count, err := m.readCounter(ctx, m.hourKey(codeType, key, at.In(m.opt.loc)))
	if err != nil {
		return 0, fmt.Errorf("msgcode: count hour failed: %w", err)
	}
	return count, nil
}

// CountDay 返回某个 key + type 在指定时刻所在自然日的发送次数。
func (m *Manager) CountDay(ctx context.Context, key string, codeType int16, at time.Time) (int64, error) {
	count, err := m.readCounter(ctx, m.dayKey(codeType, key, at.In(m.opt.loc)))
	if err != nil {
		return 0, fmt.Errorf("msgcode: count day failed: %w", err)
	}
	return count, nil
}

// readCounter 读取计数器，不存在时返回 0。
func (m *Manager) readCounter(ctx context.Context, k string) (int64, error) {
	v, err := m.rdb.Get(ctx, k).Int64()
	if err == nil {
		return v, nil
	}
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return 0, err
}

// codeKey 生成验证码存储 key。
func (m *Manager) codeKey(codeType int16, key string) string {
	return fmt.Sprintf("%s:code:%d:%s", m.opt.prefix, codeType, key)
}

// hourKey 生成小时统计 key。
func (m *Manager) hourKey(codeType int16, key string, at time.Time) string {
	return fmt.Sprintf("%s:stat:hour:%s:%d:%s", m.opt.prefix, at.Format("2006010215"), codeType, key)
}

// dayKey 生成天统计 key。
func (m *Manager) dayKey(codeType int16, key string, at time.Time) string {
	return fmt.Sprintf("%s:stat:day:%s:%d:%s", m.opt.prefix, at.Format("20060102"), codeType, key)
}

// now 返回当前时间，便于测试注入。
func (m *Manager) now() time.Time {
	return m.opt.nowFn()
}
