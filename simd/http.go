package simd

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-json"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/iot/simhub/apis/basepb"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// httpRuntimeConfig 保存 HTTP 处理路径需要的配置快照。
type httpRuntimeConfig struct {
	proxyEnabled bool
	staticPrefix string
	staticDir    string
}

// responseWrapper 缓存 gateway 的原始响应，便于统一包装输出。
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

// newResponseWrapper 创建响应包装器。
func newResponseWrapper(w http.ResponseWriter) *responseWrapper {
	return &responseWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           bytes.NewBuffer(nil),
	}
}

// WriteHeader 记录原始状态码，最终输出由 finalize 统一处理。
func (w *responseWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// Write 缓存原始响应体。
func (w *responseWrapper) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

// finalize 将原始响应转换为统一的 code/message/data 格式。
func (w *responseWrapper) finalize() {
	w.Header().Del("Vary")
	w.Header().Del("grpc-metadata-content-type")
	w.Header().Del("Content-Length")
	w.Header().Set("Content-Type", "application/json")
	w.ResponseWriter.WriteHeader(http.StatusOK)

	response := make(map[string]any, 3)
	if w.statusCode >= http.StatusBadRequest {
		var errBody struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(w.body.Bytes(), &errBody); err == nil {
			response["code"] = errBody.Code
			response["message"] = errBody.Message
		} else {
			log.Error().Bytes("body", w.body.Bytes()).Msg("parse error response body failed")
			response["code"] = w.statusCode
			response["message"] = w.body.String()
		}
	} else {
		response["code"] = 0
		response["message"] = "success"
		if w.body.Len() > 0 {
			response["data"] = json.RawMessage(w.body.Bytes())
		}
	}

	if err := json.NewEncoder(w.ResponseWriter).Encode(response); err != nil {
		log.Error().Err(err).Msg("write wrapped response failed")
	}
}

// newHttpRuntimeConfig 读取 HTTP 运行时配置。
func newHttpRuntimeConfig() httpRuntimeConfig {
	staticDir := cmp.Or(viper.GetString("static.dir"), "./static")
	proxyEnabled := true
	if viper.IsSet("proxy.enabled") {
		proxyEnabled = viper.GetBool("proxy.enabled")
	}

	return httpRuntimeConfig{
		proxyEnabled: proxyEnabled,
		staticPrefix: viper.GetString("static.prefix"),
		staticDir:    staticDir,
	}
}

// gatewayHandler 对 gateway 请求进行 trace 透传和可选响应包装。
func gatewayHandler(h http.Handler) http.Handler {
	cfg := newHttpRuntimeConfig()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		req := r.WithContext(ctx)
		if !cfg.proxyEnabled {
			h.ServeHTTP(w, req)
			return
		}

		wrapper := newResponseWrapper(w)
		h.ServeHTTP(wrapper, req)
		wrapper.finalize()
	})
}

// initHttpHandler 初始化 HTTP 处理器，聚合静态文件和 API。
func initHttpHandler(gwmux *runtime.ServeMux) http.Handler {
	cfg := newHttpRuntimeConfig()
	rootMux := http.NewServeMux()

	if cfg.staticPrefix != "" {
		log.Info().Str("prefix", cfg.staticPrefix).Str("dir", cfg.staticDir).Msg("register static file server")
		rootMux.Handle(cfg.staticPrefix, http.StripPrefix(cfg.staticPrefix, http.FileServer(http.Dir(cfg.staticDir))))
	}

	rootMux.Handle("/", gatewayHandler(gwmux))
	return rootMux
}

// writeHttpError 写入统一错误响应。
func writeHttpError(w http.ResponseWriter, code basepb.Code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"code":    int32(code),
		"message": msg,
	}); err != nil {
		log.Error().Err(err).Msg("write http error failed")
	}
}

// updateFileHandle 处理上传文件请求。
func (s *simServer) updateFileHandle(w http.ResponseWriter, r *http.Request, params map[string]string) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Error().Err(err).Msg("parse form error")
		writeHttpError(w, basepb.Code_CODE_ARGUMENT, "parse form error: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Error().Err(err).Msg("get file error")
		writeHttpError(w, basepb.Code_CODE_ARGUMENT, "get file error: "+err.Error())
		return
	}
	defer file.Close()

	now := time.Now()
	rootDir := cmp.Or(viper.GetString("static.dir"), "./static")
	dirname := filepath.Join(rootDir, "upload", now.Format("2006-01-02"))
	if err := os.MkdirAll(dirname, 0755); err != nil {
		log.Error().Err(err).Str("path", dirname).Msg("create upload dir error")
		writeHttpError(w, basepb.Code_CODE_INTERNAL_SERVER, "create upload dir error: "+err.Error())
		return
	}

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%d%d%s", time.Now().UnixNano()/1e6, 100+rand.Intn(899), ext)
	path := filepath.Join(dirname, filename)

	newFile, err := os.Create(path)
	if err != nil {
		log.Error().Err(err).Str("path", path).Msg("create file error")
		writeHttpError(w, basepb.Code_CODE_INTERNAL_SERVER, "create file error: "+err.Error())
		return
	}
	defer newFile.Close()

	if _, err = io.Copy(newFile, file); err != nil {
		log.Error().Err(err).Str("path", path).Msg("write file error")
		writeHttpError(w, basepb.Code_CODE_INTERNAL_SERVER, "write file error: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"path": filepath.Join("upload", now.Format("2006-01-02"), filename),
	}); err != nil {
		log.Error().Err(err).Msg("write upload response failed")
	}
}
