package msgcode

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis 创建 miniredis 实例和对应的 Redis 客户端。
func newTestRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	return mr, rdb
}

// TestSendAndVerify 测试发送、读取、校验、手动删除验证码的完整流程。
func TestSendAndVerify(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	m := NewManager(rdb)

	res, err := m.Send(ctx, "foo@example.com", 1, 123456)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if res.Value == nil || res.Value.Code != 123456 {
		t.Fatalf("unexpected value: %#v", res.Value)
	}
	if res.HourlyCount != 1 || res.DailyCount != 1 {
		t.Fatalf("unexpected count: hourly=%d daily=%d", res.HourlyCount, res.DailyCount)
	}

	got, err := m.Get(ctx, "foo@example.com", 1)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Code != 123456 || got.Type != 1 {
		t.Fatalf("unexpected code value: %#v", got)
	}

	if _, err := m.Verify(ctx, "foo@example.com", 1, 123456); err != nil {
		t.Fatalf("verify failed: %v", err)
	}

	// Verify 默认不删除，仍可读取。
	if _, err := m.Get(ctx, "foo@example.com", 1); err != nil {
		t.Fatalf("code should still exist after verify, got=%v", err)
	}

	if err := m.Delete(ctx, "foo@example.com", 1); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if _, err := m.Get(ctx, "foo@example.com", 1); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("expected ErrCodeNotFound after delete, got=%v", err)
	}
}

// TestVerifyMismatch 测试验证码不匹配时返回 ErrCodeMismatch。
func TestVerifyMismatch(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	m := NewManager(rdb)

	if _, err := m.Send(ctx, "13812345678", 2, 888888); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if _, err := m.Verify(ctx, "13812345678", 2, 111111); !errors.Is(err, ErrCodeMismatch) {
		t.Fatalf("expected ErrCodeMismatch, got=%v", err)
	}
}

// TestCodeValueTypeMismatch 测试类型不匹配时返回 ErrCodeTypeMismatch。
func TestCodeValueTypeMismatch(t *testing.T) {
	cv := CodeValue{Type: 1, Code: 666666, CreatedAt: time.Now().Unix(), CodeTtl: 300}
	if err := cv.Valid(3, 666666); !errors.Is(err, ErrCodeTypeMismatch) {
		t.Fatalf("expected ErrCodeTypeMismatch, got=%v", err)
	}
}

// TestVerifyExpired 测试验证码过期时返回 ErrCodeExpired。
func TestVerifyExpired(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()

	now := time.Date(2026, 3, 14, 10, 0, 0, 0, time.Local)
	m := NewManager(rdb, WithCodeTtl(5*time.Minute), withNowFunc(func() time.Time { return now }))

	if _, err := m.Send(ctx, "foo@example.com", 1, 123123); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	now = now.Add(6 * time.Minute)
	if _, err := m.Verify(ctx, "foo@example.com", 1, 123123); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("expected ErrCodeExpired, got=%v", err)
	}
}

// TestStatAndCount 测试发送统计和读取统计。
func TestStatAndCount(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	m := NewManager(rdb)

	for i := 0; i < 3; i++ {
		if _, err := m.Send(ctx, "foo@example.com", 1, 100000+i); err != nil {
			t.Fatalf("send failed: %v", err)
		}
	}

	stat, err := m.Stat(ctx, "foo@example.com", 1)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if stat.HourlyCount != 3 || stat.DailyCount != 3 {
		t.Fatalf("unexpected stat: %#v", stat)
	}

	now := time.Now()
	hourly, err := m.CountHour(ctx, "foo@example.com", 1, now)
	if err != nil {
		t.Fatalf("count hour failed: %v", err)
	}
	daily, err := m.CountDay(ctx, "foo@example.com", 1, now)
	if err != nil {
		t.Fatalf("count day failed: %v", err)
	}
	if hourly != 3 || daily != 3 {
		t.Fatalf("unexpected count: hourly=%d daily=%d", hourly, daily)
	}
}

// TestStatIsolation 测试不同 key、不同 type 的统计互不干扰。
func TestStatIsolation(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	m := NewManager(rdb)

	if _, err := m.Send(ctx, "foo@example.com", 1, 111111); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if _, err := m.Send(ctx, "foo@example.com", 2, 222222); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if _, err := m.Send(ctx, "13812345678", 1, 333333); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	stat11, _ := m.Stat(ctx, "foo@example.com", 1)
	stat12, _ := m.Stat(ctx, "foo@example.com", 2)
	stat21, _ := m.Stat(ctx, "13812345678", 1)

	if stat11.HourlyCount != 1 || stat11.DailyCount != 1 {
		t.Fatalf("unexpected stat11: %#v", stat11)
	}
	if stat12.HourlyCount != 1 || stat12.DailyCount != 1 {
		t.Fatalf("unexpected stat12: %#v", stat12)
	}
	if stat21.HourlyCount != 1 || stat21.DailyCount != 1 {
		t.Fatalf("unexpected stat21: %#v", stat21)
	}
}

// TestHourDayWindowSwitch 测试跨小时、跨天后统计窗口切换正确。
func TestHourDayWindowSwitch(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	loc := time.FixedZone("UTC+8", 8*3600)

	now := time.Date(2026, 3, 14, 23, 59, 0, 0, loc)
	m := NewManager(rdb, WithLocation(loc), withNowFunc(func() time.Time { return now }))

	if _, err := m.Send(ctx, "foo@example.com", 1, 111111); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	oldHour, err := m.CountHour(ctx, "foo@example.com", 1, now)
	if err != nil || oldHour != 1 {
		t.Fatalf("unexpected old hour count: %d, err=%v", oldHour, err)
	}
	oldDay, err := m.CountDay(ctx, "foo@example.com", 1, now)
	if err != nil || oldDay != 1 {
		t.Fatalf("unexpected old day count: %d, err=%v", oldDay, err)
	}

	now = now.Add(2 * time.Minute)
	if _, err := m.Send(ctx, "foo@example.com", 1, 222222); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	newHour, err := m.CountHour(ctx, "foo@example.com", 1, now)
	if err != nil || newHour != 1 {
		t.Fatalf("unexpected new hour count: %d, err=%v", newHour, err)
	}
	newDay, err := m.CountDay(ctx, "foo@example.com", 1, now)
	if err != nil || newDay != 1 {
		t.Fatalf("unexpected new day count: %d, err=%v", newDay, err)
	}
}

// TestPrefixIsolation 测试不同 prefix 数据互相隔离。
func TestPrefixIsolation(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()

	m1 := NewManager(rdb, WithPrefix("user-msg"))
	m2 := NewManager(rdb, WithPrefix("admin-msg"))

	if _, err := m1.Send(ctx, "foo@example.com", 1, 123456); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if _, err := m2.Get(ctx, "foo@example.com", 1); !errors.Is(err, ErrCodeNotFound) {
		t.Fatalf("expected ErrCodeNotFound across prefix, got=%v", err)
	}
}

// TestSendValueNil 测试 SendValue 输入 nil 的报错。
func TestSendValueNil(t *testing.T) {
	_, rdb := newTestRedis(t)
	ctx := context.Background()
	m := NewManager(rdb)

	if _, err := m.SendValue(ctx, nil); !errors.Is(err, ErrCodeValueNil) {
		t.Fatalf("expected ErrCodeValueNil, got=%v", err)
	}
}

// TestCodeValueValidAt 测试 CodeValue.ValidAt 的正确性。
func TestCodeValueValidAt(t *testing.T) {
	created := time.Date(2026, 3, 14, 10, 0, 0, 0, time.Local)
	cv := CodeValue{Type: 1, Code: 999999, CreatedAt: created.Unix(), CodeTtl: 60}

	if err := cv.ValidAt(1, 111111, created); !errors.Is(err, ErrCodeMismatch) {
		t.Fatalf("expected ErrCodeMismatch, got=%v", err)
	}
	if err := cv.ValidAt(1, 999999, created.Add(61*time.Second)); !errors.Is(err, ErrCodeExpired) {
		t.Fatalf("expected ErrCodeExpired, got=%v", err)
	}
	if err := cv.ValidAt(1, 999999, created.Add(30*time.Second)); err != nil {
		t.Fatalf("expected valid code, got=%v", err)
	}
}

// TestBinaryMarshalUnmarshal 测试 CodeValue 的二进制编解码实现。
func TestBinaryMarshalUnmarshal(t *testing.T) {
	input := CodeValue{
		Type:      3,
		Code:      654321,
		Key:       "foo@example.com",
		CreatedAt: 1700000000,
		CodeTtl:   300,
	}

	data, err := input.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var output CodeValue
	if err := output.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if output.Type != input.Type || output.Code != input.Code || output.Key != input.Key {
		t.Fatalf("decoded mismatch: input=%#v output=%#v", input, output)
	}
}

// ExampleManager_Send 展示发送验证码并读取发送统计的用法。
func ExampleManager_Send() {
	ctx := context.Background()
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	m := NewManager(rdb)
	res, _ := m.Send(ctx, "foo@example.com", 1, 123456)

	_, _ = m.Stat(ctx, "foo@example.com", 1)
	_, _ = res.Value, res.HourlyCount
}

// ExampleManager_Verify 展示校验验证码的用法。
func ExampleManager_Verify() {
	ctx := context.Background()
	mr, _ := miniredis.Run()
	defer mr.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rdb.Close()

	m := NewManager(rdb)
	_, _ = m.Send(ctx, "13812345678", 2, 666666)

	// 只校验，不删除。
	_, _ = m.Verify(ctx, "13812345678", 2, 666666)
	// 业务成功后手动删除。
	_ = m.Delete(ctx, "13812345678", 2)
}
