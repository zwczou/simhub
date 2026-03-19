package token

import (
	"fmt"
	"strconv"

	json "github.com/goccy/go-json"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TokenValue 保存 Token 的完整信息
// 实现 encoding.BinaryMarshaler / encoding.BinaryUnmarshaler，
// 可直接用于 rdb.Set(ctx, key, &tv, ttl) 和 rdb.Get(...).Scan(&tv)
type TokenValue struct {
	UserId       int64  `json:"user_id"`
	UserType     string `json:"user_type"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Platform     string `json:"platform"`
	CreatedAt    int64  `json:"created_at"`
	AccessTtl    int64  `json:"access_ttl"`
	RefreshTtl   int64  `json:"refresh_ttl"`
	Extras       []byte `json:"extras"`
}

// MarshalBinary 实现 encoding.BinaryMarshaler，内部使用 go-json 序列化
func (tv TokenValue) MarshalBinary() ([]byte, error) {
	return json.Marshal(tv)
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler，内部使用 go-json 反序列化
func (tv *TokenValue) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, tv)
}

// SetExtra 向 Extras 中写入指定 key 的值
//
//	tv.SetExtra("role", "admin")
func (tv *TokenValue) SetExtra(key string, value any) {
	tv.Extras, _ = sjson.SetBytes(tv.Extras, key, value)
}

// GetExtra 从 Extras 中读取指定 key 的值
//
//	role := tv.GetExtra("role").String()
func (tv *TokenValue) GetExtra(key string) gjson.Result {
	return gjson.GetBytes(tv.Extras, key)
}

// sessionValue 表示内部 Redis Session 记录
type sessionValue struct {
	SessionId    string
	UserId       int64
	UserType     string
	AccessToken  string
	RefreshToken string
	Platform     string
	CreatedAt    int64
	AccessTtl    int64
	RefreshTtl   int64
	Extras       []byte
}

// newSessionValue 根据 TokenValue 构造内部 Session 记录
func newSessionValue(sessionId string, tv *TokenValue) *sessionValue {
	return &sessionValue{
		SessionId:    sessionId,
		UserId:       tv.UserId,
		UserType:     tv.UserType,
		AccessToken:  tv.AccessToken,
		RefreshToken: tv.RefreshToken,
		Platform:     tv.Platform,
		CreatedAt:    tv.CreatedAt,
		AccessTtl:    tv.AccessTtl,
		RefreshTtl:   tv.RefreshTtl,
		Extras:       tv.Extras,
	}
}

// TokenValue 返回对外暴露的 TokenValue 视图
func (sv *sessionValue) TokenValue() *TokenValue {
	return &TokenValue{
		UserId:       sv.UserId,
		UserType:     sv.UserType,
		AccessToken:  sv.AccessToken,
		RefreshToken: sv.RefreshToken,
		Platform:     sv.Platform,
		CreatedAt:    sv.CreatedAt,
		AccessTtl:    sv.AccessTtl,
		RefreshTtl:   sv.RefreshTtl,
		Extras:       sv.Extras,
	}
}

// hashFields 返回写入 Redis Hash 所需的字段集合
func (sv *sessionValue) hashFields() map[string]any {
	return map[string]any{
		"session_id":    sv.SessionId,
		"user_id":       sv.UserId,
		"user_type":     sv.UserType,
		"access_token":  sv.AccessToken,
		"refresh_token": sv.RefreshToken,
		"platform":      sv.Platform,
		"created_at":    sv.CreatedAt,
		"access_ttl":    sv.AccessTtl,
		"refresh_ttl":   sv.RefreshTtl,
		"extras":        string(sv.Extras),
	}
}

// parseSessionValue 将 Redis Hash 字段解析为内部 Session 记录
func parseSessionValue(fields map[string]string) (*sessionValue, error) {
	if len(fields) == 0 {
		return nil, ErrTokenNotFound
	}

	userId, err := parseInt64Field(fields, "user_id")
	if err != nil {
		return nil, err
	}
	createdAt, err := parseInt64Field(fields, "created_at")
	if err != nil {
		return nil, err
	}
	accessTtl, err := parseInt64Field(fields, "access_ttl")
	if err != nil {
		return nil, err
	}
	refreshTtl, err := parseInt64Field(fields, "refresh_ttl")
	if err != nil {
		return nil, err
	}

	return &sessionValue{
		SessionId:    fields["session_id"],
		UserId:       userId,
		UserType:     fields["user_type"],
		AccessToken:  fields["access_token"],
		RefreshToken: fields["refresh_token"],
		Platform:     fields["platform"],
		CreatedAt:    createdAt,
		AccessTtl:    accessTtl,
		RefreshTtl:   refreshTtl,
		Extras:       []byte(fields["extras"]),
	}, nil
}

// parseInt64Field 解析指定字段的 int64 值
func parseInt64Field(fields map[string]string, key string) (int64, error) {
	raw, ok := fields[key]
	if !ok || raw == "" {
		return 0, fmt.Errorf("token: session field %s is empty", key)
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("token: session field %s is invalid: %w", key, err)
	}
	return value, nil
}
