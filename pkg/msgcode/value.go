package msgcode

import (
	"errors"
	"time"

	json "github.com/goccy/go-json"
)

var (
	// ErrCodeNotFound 表示验证码不存在或已过期。
	ErrCodeNotFound = errors.New("msgcode: code not found")
	// ErrCodeValueNil 表示 CodeValue 为 nil。
	ErrCodeValueNil = errors.New("msgcode: code value is nil")
	// ErrCodeMismatch 表示验证码不匹配。
	ErrCodeMismatch = errors.New("msgcode: code mismatch")
	// ErrCodeTypeMismatch 表示验证码类型不匹配。
	ErrCodeTypeMismatch = errors.New("msgcode: code type mismatch")
	// ErrCodeExpired 表示验证码已过期。
	ErrCodeExpired = errors.New("msgcode: code expired")
)

// CodeValue 保存验证码信息。
// 实现 encoding.BinaryMarshaler / encoding.BinaryUnmarshaler，
// 可直接用于 rdb.Set(ctx, key, &cv, ttl) 和 rdb.Get(...).Scan(&cv)。
type CodeValue struct {
	Type      int16  `json:"type"`
	Code      int    `json:"code"`
	Key       string `json:"key"`
	CreatedAt int64  `json:"created_at"`
	CodeTtl   int64  `json:"code_ttl"`
}

// MarshalBinary 实现 encoding.BinaryMarshaler，内部使用 go-json 序列化。
func (cv CodeValue) MarshalBinary() ([]byte, error) {
	return json.Marshal(cv)
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler，内部使用 go-json 反序列化。
func (cv *CodeValue) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, cv)
}

// Valid 校验验证码类型与输入验证码是否匹配，且仍在有效期内。
func (cv CodeValue) Valid(codeType int16, input int) error {
	return cv.ValidAt(codeType, input, time.Now())
}

// ValidAt 在指定时间点校验验证码类型与输入验证码是否匹配且未过期。
func (cv CodeValue) ValidAt(codeType int16, input int, at time.Time) error {
	if cv.Type != codeType {
		return ErrCodeTypeMismatch
	}
	if cv.Code != input {
		return ErrCodeMismatch
	}
	if cv.CodeTtl > 0 && at.Unix() > cv.CreatedAt+cv.CodeTtl {
		return ErrCodeExpired
	}
	return nil
}
