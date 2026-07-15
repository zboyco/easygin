package easygin

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

var defaultRedactedQueryParameters = []string{
	"token",
	"access_token",
	"refresh_token",
	"id_token",
	"authorization",
	"api_key",
	"password",
	"client_secret",
	"secret",
	"signature",
	"sig",
	"session",
	"session_id",
}

const (
	defaultMaxBodyBytes      int64         = 10 * 1024 * 1024
	defaultReadHeaderTimeout time.Duration = 5 * time.Second
	defaultReadTimeout       time.Duration = 60 * time.Second
	defaultWriteTimeout      time.Duration = 60 * time.Second
	defaultIdleTimeout       time.Duration = 120 * time.Second
)

// serverOptions 保存服务器的可选安全、可观测性和生命周期配置。
type serverOptions struct {
	maxBodyBytes          int64
	exposeErrorDetails    bool
	redactQueryParameters map[string]struct{}
	tracerProvider        trace.TracerProvider
	readHeaderTimeout     time.Duration
	readTimeout           time.Duration
	writeTimeout          time.Duration
	idleTimeout           time.Duration
}

// ServerOption 配置 NewServerWithOptions 创建的服务器。
type ServerOption func(*serverOptions) error

// defaultServerOptions 返回 v0.2.0 的严格安全默认配置。
func defaultServerOptions() serverOptions {
	return serverOptions{
		maxBodyBytes:          defaultMaxBodyBytes,
		redactQueryParameters: normalizedQueryParameterSet(defaultRedactedQueryParameters),
		readHeaderTimeout:     defaultReadHeaderTimeout,
		readTimeout:           defaultReadTimeout,
		writeTimeout:          defaultWriteTimeout,
		idleTimeout:           defaultIdleTimeout,
	}
}

// normalizedQueryParameterSet 将参数名称规范化为独立的小写集合。
func normalizedQueryParameterSet(names []string) map[string]struct{} {
	result := make(map[string]struct{}, len(names))
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			result[name] = struct{}{}
		}
	}
	return result
}

// WithMaxBodyBytes 设置请求体最大字节数；传入 0 表示不限制。
func WithMaxBodyBytes(limit int64) ServerOption {
	return func(options *serverOptions) error {
		if limit < 0 {
			return fmt.Errorf("max body bytes must be non-negative, got %d", limit)
		}
		options.maxBodyBytes = limit
		return nil
	}
}

// WithExposeErrorDetails 设置是否向客户端返回普通错误和 5xx ErrorHttp 的详情。
// 4xx ErrorHttp 始终保留业务状态码、消息和描述。
func WithExposeErrorDetails(enabled bool) ServerOption {
	return func(options *serverOptions) error {
		options.exposeErrorDetails = enabled
		return nil
	}
}

// WithRedactQueryParameters 向默认敏感参数集合追加访问日志中需要移除的查询参数名称。
func WithRedactQueryParameters(names ...string) ServerOption {
	return func(options *serverOptions) error {
		if options.redactQueryParameters == nil {
			options.redactQueryParameters = normalizedQueryParameterSet(defaultRedactedQueryParameters)
		}
		for _, name := range names {
			name = strings.ToLower(strings.TrimSpace(name))
			if name != "" {
				options.redactQueryParameters[name] = struct{}{}
			}
		}
		return nil
	}
}

// WithTracerProvider 设置 Gin OpenTelemetry 中间件使用的 Provider。
func WithTracerProvider(provider trace.TracerProvider) ServerOption {
	return func(options *serverOptions) error {
		if isNilTracerProvider(provider) {
			return fmt.Errorf("tracer provider is nil")
		}
		options.tracerProvider = provider
		return nil
	}
}

// isNilTracerProvider 判断 TracerProvider 是否为 nil interface 或 typed-nil 指针。
func isNilTracerProvider(provider trace.TracerProvider) bool {
	if provider == nil {
		return true
	}

	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// WithHTTPTimeouts 设置底层 http.Server 的请求生命周期超时。
func WithHTTPTimeouts(readHeader, read, write, idle time.Duration) ServerOption {
	return func(options *serverOptions) error {
		for _, timeout := range []struct {
			name  string
			value time.Duration
		}{
			{name: "read header timeout", value: readHeader},
			{name: "read timeout", value: read},
			{name: "write timeout", value: write},
			{name: "idle timeout", value: idle},
		} {
			if timeout.value < 0 {
				return fmt.Errorf("%s must be non-negative, got %s", timeout.name, timeout.value)
			}
		}
		options.readHeaderTimeout = readHeader
		options.readTimeout = read
		options.writeTimeout = write
		options.idleTimeout = idle
		return nil
	}
}
