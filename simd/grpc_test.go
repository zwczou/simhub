package simd

import (
	"context"
	"testing"

	"github.com/iot/simhub/apis/basepb"
	"github.com/iot/simhub/internal/errfmt"
	"github.com/iot/simhub/pkg/meta"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestParseGrpcMethod 验证 gRPC 方法路径解析结果。
func TestParseGrpcMethod(t *testing.T) {
	tests := []struct {
		name       string
		fullMethod string
		service    string
		method     string
		isMan      bool
		wantErr    bool
	}{
		{
			name:       "user method",
			fullMethod: "/userpb.UserService/Login",
			service:    "userpb.UserService",
			method:     "Login",
		},
		{
			name:       "man method",
			fullMethod: "/manpb.AuthService/ListUsers",
			service:    "manpb.AuthService",
			method:     "ListUsers",
			isMan:      true,
		},
		{
			name:       "invalid method",
			fullMethod: "broken",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGrpcMethod(tt.fullMethod)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseGrpcMethod() error = %v", err)
			}
			if got.service != tt.service || got.method != tt.method || got.isMan != tt.isMan {
				t.Fatalf("parseGrpcMethod() = %#v, want service=%s method=%s isMan=%v", got, tt.service, tt.method, tt.isMan)
			}
		})
	}
}

// TestNewGrpcRuntimeConfig 验证运行时配置会读取默认值和白名单。
func TestNewGrpcRuntimeConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("auth.disabled", true)
	viper.Set("auth.whitelists", []string{"/v1/auth/login", "/v1/auth/refresh"})

	cfg := newGrpcRuntimeConfig()
	if !cfg.authDisabled {
		t.Fatal("expected authDisabled to be true")
	}
	if cfg.logLength != 2048 {
		t.Fatalf("expected default log length 2048, got %d", cfg.logLength)
	}
	if !cfg.isAuthWhitelisted("/v1/auth/login") {
		t.Fatal("expected whitelist path to match")
	}
	if cfg.isAuthWhitelisted("/v1/users") {
		t.Fatal("expected non-whitelist path to miss")
	}
}

// TestAuthenticateGrpcRequestWhitelist 验证白名单请求不会依赖 token 管理器。
func TestAuthenticateGrpcRequestWhitelist(t *testing.T) {
	s := &simServer{}
	cfg := grpcRuntimeConfig{
		authWhitelists: map[string]struct{}{
			"/v1/auth/login": {},
		},
	}
	metadata := meta.New()
	metadata.Set(meta.MetaRequestPath, "/v1/auth/login")
	ctx := metadata.Context(context.Background())

	nextCtx, err := s.authenticateGrpcRequest(ctx, metadata, grpcMethodInfo{}, cfg, log.Logger)
	if err != nil {
		t.Fatalf("authenticateGrpcRequest() error = %v", err)
	}
	if meta.FromContext(nextCtx).GetString(meta.MetaRequestPath) != "/v1/auth/login" {
		t.Fatal("expected request path to stay in context")
	}
}

// TestAuthenticateGrpcRequestMissingToken 验证非白名单请求缺少 token 时返回未授权。
func TestAuthenticateGrpcRequestMissingToken(t *testing.T) {
	s := &simServer{}
	cfg := grpcRuntimeConfig{}
	metadata := meta.New()
	metadata.Set(meta.MetaRequestPath, "/v1/users")
	ctx := metadata.Context(context.Background())

	_, err := s.authenticateGrpcRequest(ctx, metadata, grpcMethodInfo{}, cfg, log.Logger)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected grpc status error, got %v", err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Fatalf("expected unauthenticated, got %s", st.Code())
	}
	if baseErr := errfmt.Parse(err); baseErr == nil || baseErr.Code != uint32(basepb.Code_CODE_UNAUTHORIZED) {
		t.Fatalf("expected unauthorized base error, got %#v", baseErr)
	}
}

// TestMapGrpcCodeToBaseCode 验证网关错误码映射规则。
func TestMapGrpcCodeToBaseCode(t *testing.T) {
	tests := []struct {
		code codes.Code
		want basepb.Code
	}{
		{code: codes.InvalidArgument, want: basepb.Code_CODE_ARGUMENT},
		{code: codes.NotFound, want: basepb.Code_CODE_NOT_FOUND},
		{code: codes.Unimplemented, want: basepb.Code_CODE_NOT_FOUND},
		{code: codes.Unauthenticated, want: basepb.Code_CODE_UNAUTHORIZED},
		{code: codes.PermissionDenied, want: basepb.Code_CODE_FORBIDDEN},
		{code: codes.ResourceExhausted, want: basepb.Code_CODE_RATE_LIMIT},
		{code: codes.Unavailable, want: basepb.Code_CODE_MAINTENANCE},
		{code: codes.Internal, want: basepb.Code_CODE_INTERNAL_SERVER},
	}

	for _, tt := range tests {
		if got := mapGrpcCodeToBaseCode(tt.code); got != tt.want {
			t.Fatalf("mapGrpcCodeToBaseCode(%s) = %v, want %v", tt.code, got, tt.want)
		}
	}
}
