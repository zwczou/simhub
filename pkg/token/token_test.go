package token

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis 创建 miniredis 实例和对应的 Redis 客户端
func newTestRedis(t *testing.T) (*miniredis.Miniredis, redis.UniversalClient) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return mr, rdb
}

// mustSessionId 读取指定索引对应的 SessionId
func mustSessionId(t *testing.T, rdb redis.UniversalClient, key string) string {
	t.Helper()
	sessionId, err := rdb.Get(context.Background(), key).Result()
	if err != nil {
		t.Fatalf("get session id failed: %v", err)
	}
	return sessionId
}

// mustSessionValue 读取指定 SessionId 的内部会话记录
func mustSessionValue(t *testing.T, tm *TokenManager, sessionId string) *sessionValue {
	t.Helper()
	sv, err := tm.loadSession(context.Background(), sessionId)
	if err != nil {
		t.Fatalf("load session failed: %v", err)
	}
	return sv
}

// requireTtlEqual 校验指定 key 的 TTL 是否符合预期
func requireTtlEqual(t *testing.T, mr *miniredis.Miniredis, key string, expected time.Duration) {
	t.Helper()
	ttl := mr.TTL(key)
	if ttl != expected {
		t.Fatalf("ttl mismatch for %s: expected=%s got=%s", key, expected, ttl)
	}
}

// TestCreateRaw 测试基本的 Token 创建、索引写入和验证
func TestCreateRaw(t *testing.T) {
	mr, rdb := newTestRedis(t)
	fixedNow := time.Unix(1700000000, 0)
	tm := NewTokenManager(rdb, withNowFunc(func() time.Time { return fixedNow }))

	tv, err := tm.CreateRaw(context.Background(), 100, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if tv.UserId != 100 {
		t.Fatalf("expected user_id=100, got=%d", tv.UserId)
	}
	if tv.UserType != "user" {
		t.Fatalf("expected user_type=user, got=%s", tv.UserType)
	}
	if tv.Platform != "ios" {
		t.Fatalf("expected platform=ios, got=%s", tv.Platform)
	}
	if tv.CreatedAt != fixedNow.Unix() {
		t.Fatalf("expected created_at=%d, got=%d", fixedNow.Unix(), tv.CreatedAt)
	}

	accessSessionId := mustSessionId(t, rdb, tm.tokenKey(tv.AccessToken))
	refreshSessionId := mustSessionId(t, rdb, tm.refreshKey(tv.RefreshToken))
	if accessSessionId != refreshSessionId {
		t.Fatalf("session id mismatch: access=%s refresh=%s", accessSessionId, refreshSessionId)
	}

	sv := mustSessionValue(t, tm, accessSessionId)
	if sv.AccessToken != tv.AccessToken || sv.RefreshToken != tv.RefreshToken {
		t.Fatal("session token pair mismatch")
	}

	requireTtlEqual(t, mr, tm.tokenKey(tv.AccessToken), 2*time.Hour)
	requireTtlEqual(t, mr, tm.refreshKey(tv.RefreshToken), 7*24*time.Hour)
	requireTtlEqual(t, mr, tm.sessionKey(accessSessionId), 7*24*time.Hour)

	got, err := tm.Verify(context.Background(), tv.AccessToken)
	if err != nil {
		t.Fatalf("verify failed: %v", err)
	}
	if got.UserId != 100 {
		t.Fatalf("verify: expected user_id=100, got=%d", got.UserId)
	}
}

// TestCreateFromValue 测试通过 TokenValue 实例创建 Token
func TestCreateFromValue(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	input := &TokenValue{
		UserId:   1001,
		UserType: "guest",
		Platform: "web",
		Extras:   []byte(`{"foo":"bar"}`),
	}

	tv, err := tm.Create(context.Background(), input)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if tv.UserId != input.UserId || tv.UserType != input.UserType || tv.Platform != input.Platform {
		t.Fatal("basic fields mismatch")
	}
	if tv.GetExtra("foo").String() != "bar" {
		t.Fatal("extras mismatch")
	}
}

// TestRefresh 测试刷新后旧 Token 失效、新 Token 可用且 SessionId 不变
func TestRefresh(t *testing.T) {
	_, rdb := newTestRedis(t)
	now := time.Unix(1700000000, 0)
	tm := NewTokenManager(rdb, withNowFunc(func() time.Time { return now }))

	tv, err := tm.CreateRaw(context.Background(), 200, "user", "android", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	oldSessionId := mustSessionId(t, rdb, tm.refreshKey(tv.RefreshToken))
	oldAccess := tv.AccessToken
	oldRefresh := tv.RefreshToken

	now = now.Add(5 * time.Minute)
	newTV, err := tm.Refresh(context.Background(), oldRefresh)
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if newTV.AccessToken == oldAccess {
		t.Fatal("new access token should differ from old")
	}
	if newTV.RefreshToken == oldRefresh {
		t.Fatal("new refresh token should differ from old")
	}
	if newTV.CreatedAt != now.Unix() {
		t.Fatalf("expected refreshed created_at=%d, got=%d", now.Unix(), newTV.CreatedAt)
	}

	newSessionId := mustSessionId(t, rdb, tm.refreshKey(newTV.RefreshToken))
	if newSessionId != oldSessionId {
		t.Fatalf("session id should stay the same: old=%s new=%s", oldSessionId, newSessionId)
	}
	if _, err := tm.Verify(context.Background(), oldAccess); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound for old access, got=%v", err)
	}
	if _, err := tm.Refresh(context.Background(), oldRefresh); err != ErrRefreshTokenNotFound {
		t.Fatalf("expected ErrRefreshTokenNotFound for old refresh, got=%v", err)
	}
}

// TestRefreshAfterAccessExpiry 测试 Access Token 过期后 Refresh Token 仍可刷新
func TestRefreshAfterAccessExpiry(t *testing.T) {
	mr, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithAccessTtl(10*time.Second), WithRefreshTtl(30*time.Second))

	tv, err := tm.CreateRaw(context.Background(), 210, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	mr.FastForward(11 * time.Second)

	newTV, err := tm.Refresh(context.Background(), tv.RefreshToken)
	if err != nil {
		t.Fatalf("refresh after access expiry failed: %v", err)
	}
	if _, err := tm.Verify(context.Background(), newTV.AccessToken); err != nil {
		t.Fatalf("verify refreshed access failed: %v", err)
	}
}

// TestRevoke 测试注销后 Token 不可用
func TestRevoke(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	tv, err := tm.CreateRaw(context.Background(), 300, "admin", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	if err := tm.Revoke(context.Background(), tv.AccessToken); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if _, err := tm.Verify(context.Background(), tv.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound after revoke, got=%v", err)
	}
	if _, err := tm.Refresh(context.Background(), tv.RefreshToken); err != ErrRefreshTokenNotFound {
		t.Fatalf("expected ErrRefreshTokenNotFound after revoke, got=%v", err)
	}
}

// TestRevokeAlreadyExpired 测试注销一个不存在的 token 不报错
func TestRevokeAlreadyExpired(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	if err := tm.Revoke(context.Background(), "nonexistent-token"); err != nil {
		t.Fatalf("revoke of nonexistent token should not error, got=%v", err)
	}
}

// TestUniqueCreateRaw 测试开启 unique 后，重复创建会清理旧 Session
func TestUniqueCreateRaw(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithUnique())

	tv1, err := tm.CreateRaw(context.Background(), 400, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create tv1 failed: %v", err)
	}
	oldSessionId := mustSessionId(t, rdb, tm.tokenKey(tv1.AccessToken))

	tv2, err := tm.CreateRaw(context.Background(), 400, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create tv2 failed: %v", err)
	}
	if _, err := tm.Verify(context.Background(), tv1.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected old access revoked, got=%v", err)
	}
	if _, err := tm.Refresh(context.Background(), tv1.RefreshToken); err != ErrRefreshTokenNotFound {
		t.Fatalf("expected old refresh revoked, got=%v", err)
	}

	fields, err := rdb.HGetAll(context.Background(), tm.sessionKey(oldSessionId)).Result()
	if err != nil {
		t.Fatalf("read old session failed: %v", err)
	}
	if len(fields) != 0 {
		t.Fatalf("expected old session removed, got=%v", fields)
	}

	newSessionId := mustSessionId(t, rdb, tm.uniqueKey(400, "ios"))
	got := mustSessionValue(t, tm, newSessionId)
	if got.AccessToken != tv2.AccessToken {
		t.Fatalf("expected unique session to point to latest access token, got=%s", got.AccessToken)
	}
}

// TestUniqueConcurrentCreate 测试并发创建时 unique 模式只保留一个有效 Session
func TestUniqueConcurrentCreate(t *testing.T) {
	_, rdb := newTestRedis(t)
	var counter uint64
	gen := func() string {
		id := atomic.AddUint64(&counter, 1)
		return fmt.Sprintf("token-%d", id)
	}
	tm := NewTokenManager(rdb, WithUnique(), WithTokenGenerator(gen))

	const workers = 8
	results := make([]*TokenValue, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = tm.CreateRaw(context.Background(), 401, "user", "ios", nil)
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
	}

	validCount := 0
	var validToken string
	for _, tv := range results {
		if _, err := tm.Verify(context.Background(), tv.AccessToken); err == nil {
			validCount++
			validToken = tv.AccessToken
		}
	}
	if validCount != 1 {
		t.Fatalf("expected exactly one valid token, got=%d", validCount)
	}

	sessionId := mustSessionId(t, rdb, tm.uniqueKey(401, "ios"))
	sv := mustSessionValue(t, tm, sessionId)
	if sv.AccessToken != validToken {
		t.Fatalf("expected unique session access token=%s, got=%s", validToken, sv.AccessToken)
	}
}

// TestStaleRevokeAfterRefreshKeepsSession 测试旧撤销不会误删刷新后的 Session
func TestStaleRevokeAfterRefreshKeepsSession(t *testing.T) {
	_, rdb := newTestRedis(t)
	now := time.Unix(1700000000, 0)
	tm := NewTokenManager(rdb, WithUnique(), withNowFunc(func() time.Time { return now }))

	tv, err := tm.CreateRaw(context.Background(), 410, "user", "ios", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	sessionId := mustSessionId(t, rdb, tm.tokenKey(tv.AccessToken))
	oldSession := mustSessionValue(t, tm, sessionId)

	now = now.Add(1 * time.Minute)
	newTV := &TokenValue{
		UserId:       oldSession.UserId,
		UserType:     oldSession.UserType,
		AccessToken:  "new-access",
		RefreshToken: "new-refresh",
		Platform:     oldSession.Platform,
		CreatedAt:    now.Unix(),
		AccessTtl:    oldSession.AccessTtl,
		RefreshTtl:   oldSession.RefreshTtl,
		Extras:       oldSession.Extras,
	}
	newSession := newSessionValue(sessionId, newTV)

	ok, err := tm.refreshSession(context.Background(), oldSession, newSession)
	if err != nil {
		t.Fatalf("refresh session failed: %v", err)
	}
	if !ok {
		t.Fatal("expected refresh session success")
	}

	ok, err = tm.revokeSession(context.Background(), oldSession)
	if err != nil {
		t.Fatalf("revoke session failed: %v", err)
	}
	if ok {
		t.Fatal("expected stale revoke to be ignored")
	}

	if _, err := tm.Verify(context.Background(), newTV.AccessToken); err != nil {
		t.Fatalf("expected refreshed access to stay valid, got=%v", err)
	}
	if sessionId != mustSessionId(t, rdb, tm.uniqueKey(410, "ios")) {
		t.Fatal("expected unique key to keep current session")
	}
}

// TestUniqueDisabled 测试未开启 unique 时同 user+platform 可并存多个 token
func TestUniqueDisabled(t *testing.T) {
	_, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb)

	tv1, err := tm.CreateRaw(context.Background(), 500, "user", "web", nil)
	if err != nil {
		t.Fatalf("create tv1 failed: %v", err)
	}
	tv2, err := tm.CreateRaw(context.Background(), 500, "user", "web", nil)
	if err != nil {
		t.Fatalf("create tv2 failed: %v", err)
	}
	if _, err := tm.Verify(context.Background(), tv1.AccessToken); err != nil {
		t.Fatalf("tv1 should still be valid, got=%v", err)
	}
	if _, err := tm.Verify(context.Background(), tv2.AccessToken); err != nil {
		t.Fatalf("tv2 should still be valid, got=%v", err)
	}
}

// TestPrefixIsolation 测试不同前缀的 TokenManager 数据互相隔离
func TestPrefixIsolation(t *testing.T) {
	_, rdb := newTestRedis(t)
	userTM := NewTokenManager(rdb, WithPrefix("user"))
	adminTM := NewTokenManager(rdb, WithPrefix("admin"))

	tv, err := userTM.CreateRaw(context.Background(), 600, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := adminTM.Verify(context.Background(), tv.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected token not found across prefix, got=%v", err)
	}
	if _, err := userTM.Verify(context.Background(), tv.AccessToken); err != nil {
		t.Fatalf("verify with correct prefix failed: %v", err)
	}
}

// TestExtras 测试 SetExtra / GetExtra 读写正确性
func TestExtras(t *testing.T) {
	tv := &TokenValue{Extras: []byte(`{}`)}
	tv.SetExtra("role", "admin")
	tv.SetExtra("level", 5)

	if tv.GetExtra("role").String() != "admin" {
		t.Fatalf("expected role=admin, got=%s", tv.GetExtra("role").String())
	}
	if tv.GetExtra("level").Int() != 5 {
		t.Fatalf("expected level=5, got=%d", tv.GetExtra("level").Int())
	}
	if tv.GetExtra("nonexistent").String() != "" {
		t.Fatalf("expected empty string for nonexistent key, got=%s", tv.GetExtra("nonexistent").String())
	}
}

// TestAccessTokenExpiry 测试 Access Token 过期后 Verify 返回错误
func TestAccessTokenExpiry(t *testing.T) {
	mr, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithAccessTtl(10*time.Second))

	tv, err := tm.CreateRaw(context.Background(), 800, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	mr.FastForward(11 * time.Second)

	if _, err := tm.Verify(context.Background(), tv.AccessToken); err != ErrTokenNotFound {
		t.Fatalf("expected ErrTokenNotFound after expiry, got=%v", err)
	}
}

// TestRefreshTokenExpiry 测试 Refresh Token 过期后无法刷新
func TestRefreshTokenExpiry(t *testing.T) {
	mr, rdb := newTestRedis(t)
	tm := NewTokenManager(rdb, WithAccessTtl(10*time.Second), WithRefreshTtl(30*time.Second))

	tv, err := tm.CreateRaw(context.Background(), 900, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	mr.FastForward(31 * time.Second)

	if _, err := tm.Refresh(context.Background(), tv.RefreshToken); err != ErrRefreshTokenNotFound {
		t.Fatalf("expected ErrRefreshTokenNotFound after expiry, got=%v", err)
	}
}

// TestCustomTokenGenerator 测试自定义 Token 生成函数生效
func TestCustomTokenGenerator(t *testing.T) {
	_, rdb := newTestRedis(t)
	counter := 0
	gen := func() string {
		counter++
		if counter%2 == 1 {
			return fmt.Sprintf("custom-access-%d", counter)
		}
		return fmt.Sprintf("custom-refresh-%d", counter)
	}

	tm := NewTokenManager(rdb, WithTokenGenerator(gen))

	tv, err := tm.CreateRaw(context.Background(), 1000, "user", "web", nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if tv.AccessToken != "custom-access-1" {
		t.Fatalf("expected access_token=custom-access-1, got=%s", tv.AccessToken)
	}
	if tv.RefreshToken != "custom-refresh-2" {
		t.Fatalf("expected refresh_token=custom-refresh-2, got=%s", tv.RefreshToken)
	}
}

// TestBinaryMarshalUnmarshal 测试 TokenValue 的 BinaryMarshaler / BinaryUnmarshaler 实现
func TestBinaryMarshalUnmarshal(t *testing.T) {
	original := TokenValue{
		UserId:       42,
		UserType:     "admin",
		AccessToken:  "abc123",
		RefreshToken: "def456",
		Platform:     "web",
		CreatedAt:    1000,
		AccessTtl:    7200,
		RefreshTtl:   604800,
		Extras:       []byte(`{"role":"super"}`),
	}

	data, err := original.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded TokenValue
	if err := decoded.UnmarshalBinary(data); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded.UserId != original.UserId {
		t.Fatalf("user_id mismatch: %d vs %d", decoded.UserId, original.UserId)
	}
	if decoded.AccessToken != original.AccessToken {
		t.Fatalf("access_token mismatch: %s vs %s", decoded.AccessToken, original.AccessToken)
	}
	if decoded.GetExtra("role").String() != "super" {
		t.Fatalf("extras mismatch: got=%s", decoded.GetExtra("role").String())
	}
}
