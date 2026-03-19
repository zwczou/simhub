package token

import (
	"github.com/redis/go-redis/v9"
)

// ManToken 管理后台专用Token
type ManToken struct {
	*TokenManager
}

// UserToken 前端用户专用Token
type UserToken struct {
	*TokenManager
}

// NewManToken 创建管理后台Token
func NewManToken(rdb redis.UniversalClient, opts ...Option) *ManToken {
	opts = append([]Option{WithPrefix("man")}, opts...)
	return &ManToken{TokenManager: NewTokenManager(rdb, opts...)}
}

// NewUserToken 创建前端用户Token
func NewUserToken(rdb redis.UniversalClient, opts ...Option) *UserToken {
	opts = append([]Option{WithPrefix("user")}, opts...)
	return &UserToken{TokenManager: NewTokenManager(rdb, opts...)}
}
