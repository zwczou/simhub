package simd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	json "github.com/goccy/go-json"
	"github.com/spf13/viper"
)

// TestNewHttpRuntimeConfig 验证 proxy.enabled 默认开启并支持配置覆盖。
func TestNewHttpRuntimeConfig(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	cfg := newHttpRuntimeConfig()
	if !cfg.proxyEnabled {
		t.Fatal("expected proxy to be enabled by default")
	}

	viper.Set("proxy.enabled", false)
	cfg = newHttpRuntimeConfig()
	if cfg.proxyEnabled {
		t.Fatal("expected proxy to be disabled by config")
	}
}

// TestGatewayHandlerProxyEnabled 验证开启包装时会输出统一响应结构。
func TestGatewayHandlerProxyEnabled(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("proxy.enabled", true)

	handler := gatewayHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"simhub"}`))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/demo", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"code":0`) || !strings.Contains(body, `"message":"success"`) || !strings.Contains(body, `"data":{"name":"simhub"}`) {
		t.Fatalf("unexpected wrapped body: %s", body)
	}
}

// TestGatewayHandlerProxyDisabled 验证关闭包装时直接透传原始响应。
func TestGatewayHandlerProxyDisabled(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("proxy.enabled", false)

	handler := gatewayHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/demo", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	if body := rec.Body.String(); body != `{"ok":true}` {
		t.Fatalf("unexpected passthrough body: %s", body)
	}
}

// TestResponseWrapperFinalizeError 验证错误响应会保留 code/message。
func TestResponseWrapperFinalizeError(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapper := newResponseWrapper(rec)
	wrapper.WriteHeader(http.StatusBadRequest)
	_, _ = wrapper.Write([]byte(`{"code":11011,"message":"bad request"}`))
	wrapper.finalize()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if got := int(body["code"].(float64)); got != 11011 {
		t.Fatalf("expected code 11011, got %d", got)
	}
	if got := body["message"].(string); got != "bad request" {
		t.Fatalf("expected message bad request, got %s", got)
	}
}
