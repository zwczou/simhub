package simd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/iot/simhub/apis/basepb"
	"github.com/iot/simhub/internal/errfmt"
	"github.com/iot/simhub/internal/utils"
	"github.com/iot/simhub/pkg/meta"
	"github.com/iot/simhub/pkg/token"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cast"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type validator interface {
	Validate() error
}

// grpcMethodInfo 保存当前 gRPC 方法的解析结果。
type grpcMethodInfo struct {
	service string
	method  string
	isMan   bool
}

// grpcRuntimeConfig 保存请求路径上会用到的配置快照。
type grpcRuntimeConfig struct {
	logLength      int
	authDisabled   bool
	authWhitelists map[string]struct{}
}

var (
	jsonpbMarshaler = protojson.MarshalOptions{
		UseProtoNames:   true,
		EmitUnpopulated: true,
		UseEnumNumbers:  false,
	}

	jsonpbUnmarshaler = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
)

// compressData 压缩数据，避免日志输出过长。
func compressData(data any, length int) string {
	body := ""
	if data != nil {
		body = cast.ToString(data)
	}
	return utils.MaskString(body, length)
}

// unaryServerInterceptor 统一处理一元请求的日志、限流、校验与鉴权逻辑。
func (s *simServer) unaryServerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (res any, err error) {
	start := time.Now()
	cfg := newGrpcRuntimeConfig()

	methodInfo, err := parseGrpcMethod(info.FullMethod)
	if err != nil {
		log.Error().Err(err).Str("full_method", info.FullMethod).Msg("invalid grpc method")
		return nil, errfmt.Internal(basepb.Code_CODE_INTERNAL_SERVER)
	}

	metadata := meta.FromContext(ctx)
	logger := newGrpcLogger(req, metadata, methodInfo, cfg.logLength)
	ctx, logger = withTraceLogger(ctx, logger)

	defer s.recoverGrpcPanic(ctx, logger, &err)

	if err = s.checkGrpcMaintenance(); err != nil {
		return nil, err
	}
	if err = s.applyGrpcRateLimit(ctx, logger); err != nil {
		return nil, err
	}
	if err = validateGrpcRequest(req, logger); err != nil {
		return nil, err
	}

	ctx, err = s.authenticateGrpcRequest(ctx, metadata, methodInfo, cfg, logger)
	if err != nil {
		return nil, err
	}

	res, err = handler(ctx, req)
	s.logGrpcResult(ctx, logger, res, err, cfg.logLength, time.Since(start))
	return res, err
}

// newGrpcRuntimeConfig 读取当前请求需要的配置快照。
func newGrpcRuntimeConfig() grpcRuntimeConfig {
	whitelists := make(map[string]struct{})
	for _, path := range viper.GetStringSlice("auth.whitelists") {
		if path == "" {
			continue
		}
		whitelists[path] = struct{}{}
	}

	logLength := viper.GetInt("log.length")
	if logLength <= 0 {
		logLength = 2048
	}

	return grpcRuntimeConfig{
		logLength:      logLength,
		authDisabled:   viper.GetBool("auth.disabled"),
		authWhitelists: whitelists,
	}
}

// parseGrpcMethod 解析 gRPC 全路径方法名。
func parseGrpcMethod(fullMethod string) (grpcMethodInfo, error) {
	parts := strings.Split(strings.TrimPrefix(fullMethod, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return grpcMethodInfo{}, fmt.Errorf("invalid grpc method: %s", fullMethod)
	}

	return grpcMethodInfo{
		service: parts[0],
		method:  parts[1],
		isMan:   strings.HasPrefix(fullMethod, "/manpb."),
	}, nil
}

// newGrpcLogger 构造当前请求的基础日志字段。
func newGrpcLogger(req any, metadata *meta.Meta, methodInfo grpcMethodInfo, length int) zerolog.Logger {
	return log.With().
		Str("grpc_service", methodInfo.service).
		Str("grpc_method", methodInfo.method).
		Str("grpc_request", compressData(req, length)).
		Str("user_ip", metadata.GetString(meta.MetaUserIp)).
		Str("user_platform", metadata.GetString(meta.MetaPlatform)).
		Str("device_id", metadata.GetString(meta.MetaDeviceId)).
		Logger()
}

// withTraceLogger 将 trace 信息注入日志上下文。
func withTraceLogger(ctx context.Context, logger zerolog.Logger) (context.Context, zerolog.Logger) {
	span := trace.SpanFromContext(ctx)
	spanCtx := span.SpanContext()
	if !spanCtx.IsValid() {
		return ctx, logger
	}

	traceLogger := logger.With().
		Str("trace_id", spanCtx.TraceID().String()).
		Str("span_id", spanCtx.SpanID().String()).
		Logger()

	return traceLogger.WithContext(ctx), traceLogger
}

// recoverGrpcPanic 兜底捕获请求处理中的 panic。
func (s *simServer) recoverGrpcPanic(ctx context.Context, logger zerolog.Logger, errPtr *error) {
	if e := recover(); e != nil {
		stack := debug.Stack()
		logger.Error().Ctx(ctx).Bytes("stack", stack).Interface("panic", e).Msg("panic error")
		*errPtr = errfmt.Internal(basepb.Code_CODE_INTERNAL_SERVER)
	}
}

// checkGrpcMaintenance 判断服务是否处于维护状态。
func (s *simServer) checkGrpcMaintenance() error {
	if s.isClosed.Load() {
		return errfmt.Errorf(codes.Unavailable, basepb.Code_CODE_MAINTENANCE)
	}
	return nil
}

// applyGrpcRateLimit 执行请求级限流校验。
func (s *simServer) applyGrpcRateLimit(ctx context.Context, logger zerolog.Logger) error {
	if s.limiter == nil {
		return nil
	}

	quota, err := s.limiter.Allow(ctx)
	if err != nil {
		logger.Warn().Err(err).Msg("rate limit check failed")
	}
	if quota == nil {
		return errfmt.Internal(basepb.Code_CODE_INTERNAL_SERVER)
	}
	if !quota.Allowed {
		logger.Warn().
			Str("rule_name", quota.RuleName).
			Int64("quota_total", quota.Total).
			Int64("quota_remaining", quota.Remaining).
			Msg("rate limit exceeded")
		return errfmt.Errorf(codes.ResourceExhausted, basepb.Code_CODE_RATE_LIMIT)
	}

	return nil
}

// validateGrpcRequest 调用请求对象上的 Validate 方法。
func validateGrpcRequest(req any, logger zerolog.Logger) error {
	v, ok := req.(validator)
	if !ok {
		return nil
	}
	if err := v.Validate(); err != nil {
		logger.Warn().Err(err).Msg("validate request failed")
		return errfmt.BadRequest(basepb.Code_CODE_ARGUMENT)
	}
	return nil
}

// authenticateGrpcRequest 执行请求鉴权并回填用户元数据。
func (s *simServer) authenticateGrpcRequest(ctx context.Context, metadata *meta.Meta, methodInfo grpcMethodInfo, cfg grpcRuntimeConfig, logger zerolog.Logger) (context.Context, error) {
	ctx = metadata.Context(ctx)
	if cfg.authDisabled {
		return ctx, nil
	}

	requestPath := metadata.GetString(meta.MetaRequestPath)
	if cfg.isAuthWhitelisted(requestPath) {
		return ctx, nil
	}

	tokenId := metadata.GetString(meta.MetaToken)
	if tokenId == "" {
		logger.Warn().Str("request_path", requestPath).Msg("missing token")
		return ctx, errfmt.Unauthorized(basepb.Code_CODE_UNAUTHORIZED)
	}

	platform := metadata.GetString(meta.MetaPlatform)
	val, err := s.verifyGrpcToken(ctx, methodInfo.isMan, tokenId)
	if err != nil {
		if errors.Is(err, token.ErrTokenNotFound) {
			logger.Warn().Err(err).Str("request_path", requestPath).Str("token_id", tokenId).Msg("invalid token")
			return ctx, errfmt.Unauthorized(basepb.Code_CODE_UNAUTHORIZED)
		}
		logger.Error().Err(err).Str("request_path", requestPath).Msg("token verify failed")
		return ctx, errfmt.Internal(basepb.Code_CODE_INTERNAL_SERVER)
	}

	if !strings.EqualFold(val.Platform, platform) {
		logger.Warn().
			Str("request_path", requestPath).
			Str("token_platform", val.Platform).
			Str("request_platform", platform).
			Msg("token platform mismatch")
		return ctx, errfmt.Unauthorized(basepb.Code_CODE_UNAUTHORIZED)
	}

	metadata.Set(meta.MetaUserId, val.UserId)
	metadata.Set(meta.MetaUserType, val.UserType)
	return metadata.Context(ctx), nil
}

// verifyGrpcToken 根据请求类型选择对应的 Token 管理器。
func (s *simServer) verifyGrpcToken(ctx context.Context, isMan bool, tokenId string) (*token.TokenValue, error) {
	if isMan {
		if s.manToken == nil {
			return nil, fmt.Errorf("man token manager is nil")
		}
		return s.manToken.Verify(ctx, tokenId)
	}

	if s.userToken == nil {
		return nil, fmt.Errorf("user token manager is nil")
	}
	return s.userToken.Verify(ctx, tokenId)
}

// logGrpcResult 统一记录请求完成后的日志与 trace 错误属性。
func (s *simServer) logGrpcResult(ctx context.Context, logger zerolog.Logger, res any, err error, length int, spent time.Duration) {
	logger = logger.With().
		Str("grpc_response", compressData(res, length)).
		Float64("spent", spent.Seconds()*1e3).
		Logger()

	if err == nil {
		logger.Info().Msg("request success")
		return
	}

	span := trace.SpanFromContext(ctx)
	if baseErr := errfmt.Parse(err); baseErr != nil {
		span.SetAttributes(attribute.Int("error.code", int(baseErr.Code)))
		if baseErr.Message != "" {
			span.SetAttributes(attribute.String("error.message", baseErr.Message))
		}
		logger = logger.With().
			Int("error_code", int(baseErr.Code)).
			Str("error_message", baseErr.Message).
			Logger()
	} else {
		logger = logger.With().Str("error_message", err.Error()).Logger()
	}

	if st, ok := status.FromError(err); ok {
		span.SetAttributes(attribute.String("grpc.code", st.Code().String()))
	}

	logger.Error().Err(err).Msg("request failed")
}

// isAuthWhitelisted 判断当前请求路径是否命中鉴权白名单。
func (c grpcRuntimeConfig) isAuthWhitelisted(path string) bool {
	if path == "" {
		return false
	}
	_, ok := c.authWhitelists[path]
	return ok
}

// gatewayErrorHandler 统一处理 grpc-gateway 返回给 HTTP 客户端的错误。
func gatewayErrorHandler(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	s, ok := status.FromError(err)
	if !ok {
		s = status.New(codes.Unknown, err.Error())
	}

	w.Header().Del("Trailer")
	w.Header().Set("Content-Type", marshaler.ContentType(nil))
	w.WriteHeader(runtime.HTTPStatusFromCode(s.Code()))

	details := s.Details()
	if len(details) > 0 {
		buf, merr := marshaler.Marshal(details[0])
		if merr == nil {
			_, _ = w.Write(buf)
		}
		return
	}

	log.Warn().
		Ctx(ctx).
		Str("path", r.URL.Path).
		Str("method", r.Method).
		Str("error", s.Message()).
		Str("code", s.Code().String()).
		Msg("gateway error")

	buf, merr := marshaler.Marshal(errfmt.NewError(mapGrpcCodeToBaseCode(s.Code()), s.Message()))
	if merr == nil {
		_, _ = w.Write(buf)
	}
}

// mapGrpcCodeToBaseCode 将 gRPC 状态码映射为统一业务错误码。
func mapGrpcCodeToBaseCode(code codes.Code) basepb.Code {
	switch code {
	case codes.InvalidArgument:
		return basepb.Code_CODE_ARGUMENT
	case codes.NotFound, codes.Unimplemented:
		return basepb.Code_CODE_NOT_FOUND
	case codes.Unauthenticated:
		return basepb.Code_CODE_UNAUTHORIZED
	case codes.PermissionDenied:
		return basepb.Code_CODE_FORBIDDEN
	case codes.ResourceExhausted:
		return basepb.Code_CODE_RATE_LIMIT
	case codes.Unavailable:
		return basepb.Code_CODE_MAINTENANCE
	default:
		return basepb.Code_CODE_INTERNAL_SERVER
	}
}
