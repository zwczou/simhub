package token

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

var (
	// ErrTokenNotFound 表示 Token 不存在或已过期
	ErrTokenNotFound = errors.New("token: not found")
	// ErrTokenValueNil 表示 TokenValue 为 nil
	ErrTokenValueNil = errors.New("token: token value is nil")
	// ErrRefreshTokenNotFound 表示 Refresh Token 不存在或已过期
	ErrRefreshTokenNotFound = errors.New("token: refresh token not found")
)

const luaCreateSession = `
local unique_key = KEYS[1]
local session_key = KEYS[2]
local access_key = KEYS[3]
local refresh_key = KEYS[4]

local session_prefix = ARGV[1]
local access_prefix = ARGV[2]
local refresh_prefix = ARGV[3]
local unique_enabled = ARGV[4]
local session_id = ARGV[5]
local access_ttl_ms = ARGV[6]
local refresh_ttl_ms = ARGV[7]

if unique_enabled == "1" then
	local old_session_id = redis.call('GET', unique_key)
	if old_session_id and old_session_id ~= session_id then
		local old_session_key = session_prefix .. old_session_id
		local old_access = redis.call('HGET', old_session_key, 'access_token')
		local old_refresh = redis.call('HGET', old_session_key, 'refresh_token')
		if old_access and old_access ~= '' then
			redis.call('DEL', access_prefix .. old_access)
		end
		if old_refresh and old_refresh ~= '' then
			redis.call('DEL', refresh_prefix .. old_refresh)
		end
		redis.call('DEL', old_session_key)
	end
end

redis.call('HSET', session_key,
	'session_id', ARGV[5],
	'user_id', ARGV[8],
	'user_type', ARGV[9],
	'access_token', ARGV[10],
	'refresh_token', ARGV[11],
	'platform', ARGV[12],
	'created_at', ARGV[13],
	'access_ttl', ARGV[14],
	'refresh_ttl', ARGV[15],
	'extras', ARGV[16]
)
redis.call('PEXPIRE', session_key, refresh_ttl_ms)
redis.call('SET', access_key, session_id, 'PX', access_ttl_ms)
redis.call('SET', refresh_key, session_id, 'PX', refresh_ttl_ms)

if unique_enabled == "1" then
	redis.call('SET', unique_key, session_id, 'PX', refresh_ttl_ms)
end

return 1
`

const luaRefreshSession = `
local session_key = KEYS[1]
local old_access_key = KEYS[2]
local old_refresh_key = KEYS[3]
local new_access_key = KEYS[4]
local new_refresh_key = KEYS[5]
local unique_key = KEYS[6]

local unique_enabled = ARGV[1]
local session_id = ARGV[2]
local expected_access = ARGV[3]
local expected_refresh = ARGV[4]
local access_ttl_ms = ARGV[5]
local refresh_ttl_ms = ARGV[6]

if redis.call('EXISTS', session_key) == 0 then
	return 0
end

local current_access = redis.call('HGET', session_key, 'access_token')
local current_refresh = redis.call('HGET', session_key, 'refresh_token')
if current_access ~= expected_access or current_refresh ~= expected_refresh then
	return 0
end

redis.call('DEL', old_access_key)
redis.call('DEL', old_refresh_key)
redis.call('HSET', session_key,
	'session_id', session_id,
	'user_id', ARGV[7],
	'user_type', ARGV[8],
	'access_token', ARGV[9],
	'refresh_token', ARGV[10],
	'platform', ARGV[11],
	'created_at', ARGV[12],
	'access_ttl', ARGV[13],
	'refresh_ttl', ARGV[14],
	'extras', ARGV[15]
)
redis.call('PEXPIRE', session_key, refresh_ttl_ms)
redis.call('SET', new_access_key, session_id, 'PX', access_ttl_ms)
redis.call('SET', new_refresh_key, session_id, 'PX', refresh_ttl_ms)

if unique_enabled == "1" then
	redis.call('SET', unique_key, session_id, 'PX', refresh_ttl_ms)
end

return 1
`

const luaRevokeSession = `
local session_key = KEYS[1]
local access_key = KEYS[2]
local refresh_key = KEYS[3]
local unique_key = KEYS[4]

local unique_enabled = ARGV[1]
local session_id = ARGV[2]
local expected_access = ARGV[3]
local expected_refresh = ARGV[4]

if redis.call('EXISTS', session_key) == 0 then
	return 0
end

local current_access = redis.call('HGET', session_key, 'access_token')
local current_refresh = redis.call('HGET', session_key, 'refresh_token')
if current_access ~= expected_access or current_refresh ~= expected_refresh then
	return 0
end

redis.call('DEL', access_key)
redis.call('DEL', refresh_key)
redis.call('DEL', session_key)

if unique_enabled == "1" then
	local current_session_id = redis.call('GET', unique_key)
	if current_session_id == session_id then
		redis.call('DEL', unique_key)
	end
end

return 1
`

var (
	// createSessionScript 负责原子创建新会话
	createSessionScript = redis.NewScript(luaCreateSession)
	// refreshSessionScript 负责原子刷新会话中的 Token 对
	refreshSessionScript = redis.NewScript(luaRefreshSession)
	// revokeSessionScript 负责原子注销会话
	revokeSessionScript = redis.NewScript(luaRevokeSession)
)

// TokenManager 是 Redis 驱动的 Token 管理器
type TokenManager struct {
	rdb redis.UniversalClient
	opt options
}

// NewTokenManager 创建一个 Token 管理器
//
//	tm := token.NewTokenManager(rdb, token.WithPrefix("user"), token.WithUnique())
func NewTokenManager(rdb redis.UniversalClient, opts ...Option) *TokenManager {
	o := defaultOptions
	for _, fn := range opts {
		fn(&o)
	}
	if o.genToken == nil {
		o.genToken = generateToken
	}
	if o.nowFn == nil {
		o.nowFn = time.Now
	}
	if o.accessTtl <= 0 {
		o.accessTtl = defaultOptions.accessTtl
	}
	if o.refreshTtl <= 0 {
		o.refreshTtl = defaultOptions.refreshTtl
	}
	return &TokenManager{rdb: rdb, opt: o}
}

// Create 从 TokenValue 实例创建一个新的 Token（仅使用其 UserId, UserType, Platform, Extras 字段）
// 最终逻辑由 CreateRaw 实现
func (tm *TokenManager) Create(ctx context.Context, tv *TokenValue) (*TokenValue, error) {
	if tv == nil {
		return nil, ErrTokenValueNil
	}
	return tm.CreateRaw(ctx, tv.UserId, tv.UserType, tv.Platform, tv.Extras)
}

// CreateRaw 直接使用原始参数创建一对 Access Token 和 Refresh Token 并写入 Redis
func (tm *TokenManager) CreateRaw(ctx context.Context, userId int64, userType, platform string, extras []byte) (*TokenValue, error) {
	tv := &TokenValue{
		UserId:       userId,
		UserType:     userType,
		AccessToken:  tm.opt.genToken(),
		RefreshToken: tm.opt.genToken(),
		Platform:     platform,
		CreatedAt:    tm.now().Unix(),
		AccessTtl:    int64(tm.opt.accessTtl.Seconds()),
		RefreshTtl:   int64(tm.opt.refreshTtl.Seconds()),
		Extras:       extras,
	}
	sv := newSessionValue(generateSessionId(), tv)

	if err := tm.createSession(ctx, sv); err != nil {
		return nil, err
	}

	log.Ctx(ctx).Info().Int64("user_id", userId).Str("platform", platform).Msg("token created")
	return tv, nil
}

// Verify 验证 Access Token 是否有效，有效则返回对应的 TokenValue
func (tm *TokenManager) Verify(ctx context.Context, accessToken string) (*TokenValue, error) {
	sessionId, err := tm.loadSessionId(ctx, tm.tokenKey(accessToken))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("token: verify lookup failed: %w", err)
	}

	sv, err := tm.loadSession(ctx, sessionId)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("token: verify read session failed: %w", err)
	}
	if sv.AccessToken != accessToken {
		return nil, ErrTokenNotFound
	}
	return sv.TokenValue(), nil
}

// Refresh 使用 Refresh Token 生成新的 Access Token 和 Refresh Token
// 旧的 Access Token 和 Refresh Token 会立即失效
func (tm *TokenManager) Refresh(ctx context.Context, refreshToken string) (*TokenValue, error) {
	sessionId, err := tm.loadSessionId(ctx, tm.refreshKey(refreshToken))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("token: refresh lookup failed: %w", err)
	}

	oldSession, err := tm.loadSession(ctx, sessionId)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil, ErrRefreshTokenNotFound
		}
		return nil, fmt.Errorf("token: refresh read session failed: %w", err)
	}
	if oldSession.RefreshToken != refreshToken {
		return nil, ErrRefreshTokenNotFound
	}

	newTV := &TokenValue{
		UserId:       oldSession.UserId,
		UserType:     oldSession.UserType,
		AccessToken:  tm.opt.genToken(),
		RefreshToken: tm.opt.genToken(),
		Platform:     oldSession.Platform,
		CreatedAt:    tm.now().Unix(),
		AccessTtl:    oldSession.AccessTtl,
		RefreshTtl:   oldSession.RefreshTtl,
		Extras:       oldSession.Extras,
	}
	newSession := newSessionValue(sessionId, newTV)

	ok, err := tm.refreshSession(ctx, oldSession, newSession)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrRefreshTokenNotFound
	}

	log.Ctx(ctx).Info().Int64("user_id", newTV.UserId).Str("platform", newTV.Platform).Msg("token refreshed")
	return newTV, nil
}

// Revoke 主动注销 Access Token，同时清理对应的 Refresh Token 和 unique key
func (tm *TokenManager) Revoke(ctx context.Context, accessToken string) error {
	sessionId, err := tm.loadSessionId(ctx, tm.tokenKey(accessToken))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil
		}
		return fmt.Errorf("token: revoke lookup failed: %w", err)
	}

	sv, err := tm.loadSession(ctx, sessionId)
	if err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil
		}
		return fmt.Errorf("token: revoke read session failed: %w", err)
	}
	if sv.AccessToken != accessToken {
		return nil
	}

	ok, err := tm.revokeSession(ctx, sv)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	log.Ctx(ctx).Info().Int64("user_id", sv.UserId).Str("platform", sv.Platform).Msg("token revoked")
	return nil
}

// createSession 使用 Lua 脚本原子写入会话及其索引
func (tm *TokenManager) createSession(ctx context.Context, sv *sessionValue) error {
	keys := []string{
		tm.uniqueKey(sv.UserId, sv.Platform),
		tm.sessionKey(sv.SessionId),
		tm.tokenKey(sv.AccessToken),
		tm.refreshKey(sv.RefreshToken),
	}

	if _, err := createSessionScript.Run(ctx, tm.rdb, keys, tm.createSessionArgs(sv)...).Result(); err != nil {
		return fmt.Errorf("token: create failed: %w", err)
	}
	return nil
}

// refreshSession 使用 Lua 脚本原子刷新会话中的 Token 对
func (tm *TokenManager) refreshSession(ctx context.Context, oldSession, newSession *sessionValue) (bool, error) {
	keys := []string{
		tm.sessionKey(oldSession.SessionId),
		tm.tokenKey(oldSession.AccessToken),
		tm.refreshKey(oldSession.RefreshToken),
		tm.tokenKey(newSession.AccessToken),
		tm.refreshKey(newSession.RefreshToken),
		tm.uniqueKey(newSession.UserId, newSession.Platform),
	}

	result, err := refreshSessionScript.Run(ctx, tm.rdb, keys, tm.refreshSessionArgs(oldSession, newSession)...).Int64()
	if err != nil {
		return false, fmt.Errorf("token: refresh failed: %w", err)
	}
	return result == 1, nil
}

// revokeSession 使用 Lua 脚本原子注销会话及其索引
func (tm *TokenManager) revokeSession(ctx context.Context, sv *sessionValue) (bool, error) {
	keys := []string{
		tm.sessionKey(sv.SessionId),
		tm.tokenKey(sv.AccessToken),
		tm.refreshKey(sv.RefreshToken),
		tm.uniqueKey(sv.UserId, sv.Platform),
	}

	result, err := revokeSessionScript.Run(ctx, tm.rdb, keys, tm.revokeSessionArgs(sv)...).Int64()
	if err != nil {
		return false, fmt.Errorf("token: revoke failed: %w", err)
	}
	return result == 1, nil
}

// createSessionArgs 构造创建会话脚本的参数
func (tm *TokenManager) createSessionArgs(sv *sessionValue) []any {
	return []any{
		tm.sessionPrefix(),
		tm.tokenPrefix(),
		tm.refreshPrefix(),
		boolFlag(tm.opt.unique),
		sv.SessionId,
		durationMillis(sv.AccessTtl),
		durationMillis(sv.RefreshTtl),
		strconv.FormatInt(sv.UserId, 10),
		sv.UserType,
		sv.AccessToken,
		sv.RefreshToken,
		sv.Platform,
		strconv.FormatInt(sv.CreatedAt, 10),
		strconv.FormatInt(sv.AccessTtl, 10),
		strconv.FormatInt(sv.RefreshTtl, 10),
		string(sv.Extras),
	}
}

// refreshSessionArgs 构造刷新会话脚本的参数
func (tm *TokenManager) refreshSessionArgs(oldSession, newSession *sessionValue) []any {
	return []any{
		boolFlag(tm.opt.unique),
		oldSession.SessionId,
		oldSession.AccessToken,
		oldSession.RefreshToken,
		durationMillis(newSession.AccessTtl),
		durationMillis(newSession.RefreshTtl),
		strconv.FormatInt(newSession.UserId, 10),
		newSession.UserType,
		newSession.AccessToken,
		newSession.RefreshToken,
		newSession.Platform,
		strconv.FormatInt(newSession.CreatedAt, 10),
		strconv.FormatInt(newSession.AccessTtl, 10),
		strconv.FormatInt(newSession.RefreshTtl, 10),
		string(newSession.Extras),
	}
}

// revokeSessionArgs 构造注销会话脚本的参数
func (tm *TokenManager) revokeSessionArgs(sv *sessionValue) []any {
	return []any{
		boolFlag(tm.opt.unique),
		sv.SessionId,
		sv.AccessToken,
		sv.RefreshToken,
	}
}

// loadSessionId 读取 Access 或 Refresh 索引对应的 SessionId
func (tm *TokenManager) loadSessionId(ctx context.Context, key string) (string, error) {
	return tm.rdb.Get(ctx, key).Result()
}

// loadSession 根据 SessionId 读取内部 Session 记录
func (tm *TokenManager) loadSession(ctx context.Context, sessionId string) (*sessionValue, error) {
	fields, err := tm.rdb.HGetAll(ctx, tm.sessionKey(sessionId)).Result()
	if err != nil {
		return nil, err
	}
	return parseSessionValue(fields)
}

// tokenKey 生成 access token 索引的 Redis key
func (tm *TokenManager) tokenKey(accessToken string) string {
	return tm.tokenPrefix() + accessToken
}

// tokenPrefix 返回 access token 索引的 Redis key 前缀
func (tm *TokenManager) tokenPrefix() string {
	return tm.opt.prefix + ":token:"
}

// refreshKey 生成 refresh token 索引的 Redis key
func (tm *TokenManager) refreshKey(refreshToken string) string {
	return tm.refreshPrefix() + refreshToken
}

// refreshPrefix 返回 refresh token 索引的 Redis key 前缀
func (tm *TokenManager) refreshPrefix() string {
	return tm.opt.prefix + ":refresh:"
}

// sessionKey 生成 Session 记录的 Redis key
func (tm *TokenManager) sessionKey(sessionId string) string {
	return tm.sessionPrefix() + sessionId
}

// sessionPrefix 返回 Session 记录的 Redis key 前缀
func (tm *TokenManager) sessionPrefix() string {
	return tm.opt.prefix + ":session:"
}

// uniqueKey 生成唯一性约束的 Redis key
func (tm *TokenManager) uniqueKey(userId int64, platform string) string {
	return fmt.Sprintf("%s:unique:%d:%s", tm.opt.prefix, userId, platform)
}

// now 返回当前时间，便于测试注入
func (tm *TokenManager) now() time.Time {
	return tm.opt.nowFn()
}

// boolFlag 将布尔值转换为 Lua 脚本可识别的标记
func boolFlag(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// durationMillis 将秒级 TTL 转换为毫秒字符串
func durationMillis(seconds int64) string {
	return strconv.FormatInt(int64(time.Duration(seconds)*time.Second/time.Millisecond), 10)
}

// generateToken 生成 32 字符的随机 hex 字符串
func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// generateSessionId 生成内部使用的 SessionId
func generateSessionId() string {
	return generateToken()
}
