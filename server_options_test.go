package easygin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// testErrorResponse 表示测试中解码后的统一错误响应。
type testErrorResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Desc string `json:"desc"`
}

// testClosedListener 提供固定地址并立即返回 Accept 错误，避免生命周期测试占用真实端口。
type testClosedListener struct {
	address net.Addr
}

// repeatedByteReader 持续生成固定字节，用于测试默认 Body 上限而不分配完整请求体。
type repeatedByteReader struct {
	value byte
}

// Read 使用固定字节填满目标缓冲区。
func (r *repeatedByteReader) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = r.value
	}
	return len(buffer), nil
}

// Accept 模拟已经关闭的 Listener。
func (l *testClosedListener) Accept() (net.Conn, error) {
	return nil, errors.New("listener is closed")
}

// Close 对测试 Listener 执行无操作关闭。
func (l *testClosedListener) Close() error {
	return nil
}

// Addr 返回测试配置的固定监听地址。
func (l *testClosedListener) Addr() net.Addr {
	return l.address
}

// decodeTestErrorResponse 解码测试响应，JSON 无效时立即终止当前测试。
func decodeTestErrorResponse(t *testing.T, body *bytes.Buffer) testErrorResponse {
	t.Helper()
	var response testErrorResponse
	if err := json.Unmarshal(body.Bytes(), &response); err != nil {
		t.Fatalf("decode error response %q: %v", body.String(), err)
	}
	return response
}

// TestNewServerWithOptions 验证 v0.2.0 严格默认值和显式覆盖行为。
func TestNewServerWithOptions(t *testing.T) {
	server, err := NewServerWithOptions(
		"test",
		":0",
		false,
		WithMaxBodyBytes(128),
		WithExposeErrorDetails(true),
		WithRedactQueryParameters(" custom_secret "),
		WithTracerProvider(noop.NewTracerProvider()),
		WithHTTPTimeouts(time.Second, 2*time.Second, 3*time.Second, 4*time.Second),
	)
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}
	if server.options.maxBodyBytes != 128 || !server.options.exposeErrorDetails {
		t.Fatalf("unexpected server options: %+v", server.options)
	}
	for _, name := range append(append([]string(nil), defaultRedactedQueryParameters...), "custom_secret") {
		if _, ok := server.options.redactQueryParameters[name]; !ok {
			t.Fatalf("sensitive query parameter %q is not configured: %#v", name, server.options.redactQueryParameters)
		}
	}
	if server.options.readHeaderTimeout != time.Second || server.options.readTimeout != 2*time.Second || server.options.writeTimeout != 3*time.Second || server.options.idleTimeout != 4*time.Second {
		t.Fatalf("unexpected timeout options: %+v", server.options)
	}

	strict := NewServer("test", ":0", false)
	if strict.options.maxBodyBytes != defaultMaxBodyBytes || strict.options.exposeErrorDetails {
		t.Fatalf("NewServer strict defaults = %+v", strict.options)
	}
	if strict.options.readHeaderTimeout != defaultReadHeaderTimeout || strict.options.readTimeout != defaultReadTimeout || strict.options.writeTimeout != defaultWriteTimeout || strict.options.idleTimeout != defaultIdleTimeout {
		t.Fatalf("NewServer timeout defaults = %+v", strict.options)
	}
	for _, name := range defaultRedactedQueryParameters {
		if _, ok := strict.options.redactQueryParameters[name]; !ok {
			t.Fatalf("default sensitive query parameter %q is missing: %#v", name, strict.options.redactQueryParameters)
		}
	}

	unsafeServer, err := NewServerWithOptions("test", ":0", false,
		WithMaxBodyBytes(0),
		WithExposeErrorDetails(true),
		WithHTTPTimeouts(0, 0, 0, 0),
	)
	if err != nil {
		t.Fatalf("create explicitly unsafe server: %v", err)
	}
	if unsafeServer.options.maxBodyBytes != 0 || !unsafeServer.options.exposeErrorDetails || unsafeServer.options.readHeaderTimeout != 0 || unsafeServer.options.readTimeout != 0 || unsafeServer.options.writeTimeout != 0 || unsafeServer.options.idleTimeout != 0 {
		t.Fatalf("explicit unsafe overrides = %+v", unsafeServer.options)
	}

	strict.options.redactQueryParameters["server_a_only"] = struct{}{}
	other := NewServer("test", ":0", false)
	if _, ok := other.options.redactQueryParameters["server_a_only"]; ok {
		t.Fatalf("redaction maps are shared across servers: %#v", other.options.redactQueryParameters)
	}
}

// TestServerOptionValidation 验证非法选项不会创建服务器。
func TestServerOptionValidation(t *testing.T) {
	tests := []ServerOption{
		WithMaxBodyBytes(-1),
		WithHTTPTimeouts(-time.Second, 0, 0, 0),
		WithTracerProvider(nil),
		nil,
	}
	for i, option := range tests {
		t.Run(fmt.Sprintf("option-%d", i), func(t *testing.T) {
			if server, err := NewServerWithOptions("test", ":0", false, option); err == nil || server != nil {
				t.Fatalf("expected option error, server=%v err=%v", server, err)
			}
		})
	}

	var typedNilProvider *noop.TracerProvider
	if server, err := NewServerWithOptions("test", ":0", false, WithTracerProvider(typedNilProvider)); err == nil || server != nil {
		t.Fatalf("expected typed-nil provider error, server=%v err=%v", server, err)
	}
}

// TestMaxBodyBytesReturns413 验证请求体超限映射为 413。
func TestMaxBodyBytesReturns413(t *testing.T) {
	server, err := NewServerWithOptions("test", ":0", false, WithMaxBodyBytes(5))
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}
	group := NewRouterGroup("/")
	group.RegisterAPI(&testBodyLimitAPI{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"too long","count":1}`))
	server.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestDefaultMaxBodyBytesReturns413 验证 NewServer 默认启用 10 MiB Body 限制。
func TestDefaultMaxBodyBytesReturns413(t *testing.T) {
	server := NewServer("test", ":0", false)
	group := NewRouterGroup("/")
	group.RegisterAPI(&testBodyLimitAPI{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	body := io.LimitReader(&repeatedByteReader{value: ' '}, defaultMaxBodyBytes+1)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", body)
	server.engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", recorder.Code, recorder.Body.String())
	}
}

// testBodyLimitAPI 定义用于验证 Body 上限的最小 JSON API。
type testBodyLimitAPI struct {
	MethodPost
	Body TestBodyValuePayload `in:"body"`
}

func (testBodyLimitAPI) Path() string {
	return "/"
}

func (testBodyLimitAPI) Output(context.Context) (any, error) {
	return nil, nil
}

// testMultipartLimitAPI 定义用于验证 Multipart Body 上限的最小 API。
type testMultipartLimitAPI struct {
	MethodPost
	Body TestMultipartCleanupBody `in:"body" mime:"multipart"`
}

func (testMultipartLimitAPI) Path() string {
	return "/multipart"
}

func (testMultipartLimitAPI) Output(context.Context) (any, error) {
	return nil, nil
}

// TestMaxBodyBytesMultipartReturns413 验证 Multipart 超限同样映射为 413。
func TestMaxBodyBytesMultipartReturns413(t *testing.T) {
	server, err := NewServerWithOptions("test", ":0", false, WithMaxBodyBytes(32))
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}
	group := NewRouterGroup("/")
	group.RegisterAPI(&testMultipartLimitAPI{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormField("value")
	if err != nil {
		t.Fatalf("CreateFormField: %v", err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 128)); err != nil {
		t.Fatalf("write multipart field: %v", err)
	}
	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/multipart", body)
	req.Header.Set("Content-Type", contentType)
	server.engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", recorder.Code, recorder.Body.String())
	}
}

// TestErrorDetailsPolicy 验证普通错误和 5xx ErrorHttp 可脱敏，而 4xx ErrorHttp 保留业务响应。
func TestErrorDetailsPolicy(t *testing.T) {
	var typedNilError *Error
	var typedNil error = typedNilError
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name          string
		err           error
		exposeDetails bool
		wantStatus    int
		wantMsg       string
		wantDesc      string
		forbidden     []string
	}{
		{
			name:       "generic error is redacted",
			err:        errors.New("database password=secret"),
			wantStatus: http.StatusInternalServerError,
			wantMsg:    http.StatusText(http.StatusInternalServerError),
			wantDesc:   http.StatusText(http.StatusInternalServerError),
			forbidden:  []string{"database password=secret"},
		},
		{
			name:       "wrapped client error keeps business details",
			err:        fmt.Errorf("wrapped: %w", NewError(http.StatusTeapot, "teapot", "business detail")),
			wantStatus: http.StatusTeapot,
			wantMsg:    "teapot",
			wantDesc:   "business detail",
		},
		{
			name:       "server ErrorHttp is redacted",
			err:        NewError(http.StatusServiceUnavailable, "backend failed", "database password=secret"),
			wantStatus: http.StatusServiceUnavailable,
			wantMsg:    http.StatusText(http.StatusServiceUnavailable),
			wantDesc:   http.StatusText(http.StatusServiceUnavailable),
			forbidden:  []string{"backend failed", "database password=secret"},
		},
		{
			name:          "server ErrorHttp remains compatible when exposed",
			err:           NewError(http.StatusInternalServerError, "backend failed", "database password=secret"),
			exposeDetails: true,
			wantStatus:    http.StatusInternalServerError,
			wantMsg:       "backend failed",
			wantDesc:      "database password=secret",
		},
		{
			name:       "status below error range is rejected",
			err:        NewError(399, "invalid low", "secret low"),
			wantStatus: http.StatusInternalServerError,
			wantMsg:    http.StatusText(http.StatusInternalServerError),
			wantDesc:   http.StatusText(http.StatusInternalServerError),
			forbidden:  []string{"invalid low", "secret low"},
		},
		{
			name:       "status above error range is rejected",
			err:        fmt.Errorf("wrapped: %w", NewError(600, "invalid high", "secret high")),
			wantStatus: http.StatusInternalServerError,
			wantMsg:    http.StatusText(http.StatusInternalServerError),
			wantDesc:   http.StatusText(http.StatusInternalServerError),
			forbidden:  []string{"invalid high", "secret high"},
		},
		{
			name:       "typed nil ErrorHttp is rejected",
			err:        typedNil,
			wantStatus: http.StatusInternalServerError,
			wantMsg:    http.StatusText(http.StatusInternalServerError),
			wantDesc:   http.StatusText(http.StatusInternalServerError),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			handleErrorWithPolicy(ctx, test.err, test.exposeDetails)
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			response := decodeTestErrorResponse(t, recorder.Body)
			if response.Code != test.wantStatus || response.Msg != test.wantMsg || response.Desc != test.wantDesc {
				t.Fatalf("response = %+v, want code=%d msg=%q desc=%q", response, test.wantStatus, test.wantMsg, test.wantDesc)
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(recorder.Body.String(), forbidden) {
					t.Fatalf("response leaked %q: %s", forbidden, recorder.Body.String())
				}
			}
		})
	}

	if !IsErrorHttp(fmt.Errorf("wrapped: %w", NewError(http.StatusBadRequest, "bad", "request"))) {
		t.Fatal("IsErrorHttp should recognize wrapped ErrorHttp")
	}
	if IsErrorHttp(typedNil) {
		t.Fatal("IsErrorHttp should reject typed-nil ErrorHttp")
	}
	for _, test := range []struct {
		status int
		want   bool
	}{
		{status: 399, want: false},
		{status: http.StatusBadRequest, want: true},
		{status: maxErrorHTTPStatus, want: true},
		{status: 600, want: false},
	} {
		if got := IsErrorHttp(NewError(test.status, "test", "test")); got != test.want {
			t.Fatalf("IsErrorHttp status %d = %t, want %t", test.status, got, test.want)
		}
	}
}

// TestRecoveryErrorDetailsPolicy 验证 Recovery 在关闭详情时不返回 panic 文本。
func TestRecoveryErrorDetailsPolicy(t *testing.T) {
	server, err := NewServerWithOptions("test", ":0", false, WithExposeErrorDetails(false))
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}

	group := NewRouterGroup("/")
	group.RegisterAPI(&testPanicAPI{})
	group.RegisterAPI(&testPanicErrorAPI{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	response := decodeTestErrorResponse(t, recorder.Body)
	if response.Code != http.StatusInternalServerError || response.Msg != http.StatusText(http.StatusInternalServerError) || response.Desc != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("panic response = %+v, want generic 500", response)
	}

	recorder = httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic-error", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("panic ErrorHttp status = %d, want 500", recorder.Code)
	}
	response = decodeTestErrorResponse(t, recorder.Body)
	if response.Code != http.StatusInternalServerError || response.Msg != http.StatusText(http.StatusInternalServerError) || response.Desc != http.StatusText(http.StatusInternalServerError) {
		t.Fatalf("panic ErrorHttp response = %+v, want generic 500", response)
	}
}

// TestRecoveryDetailsOptIn 验证显式开启详情时沿用旧版 Recovery 的状态码和字段语义。
func TestRecoveryDetailsOptIn(t *testing.T) {
	server, err := NewServerWithOptions("test", ":0", false, WithExposeErrorDetails(true))
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}
	group := NewRouterGroup("/")
	group.RegisterAPI(&testPanicAPI{})
	group.RegisterAPI(&testPanicErrorAPI{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	for _, path := range []string{"/panic", "/panic-error"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500", recorder.Code)
			}
			response := decodeTestErrorResponse(t, recorder.Body)
			if response.Code != http.StatusInternalServerError || response.Msg != http.StatusText(http.StatusInternalServerError) || response.Desc != "panic secret" {
				t.Fatalf("response = %+v, want legacy Recovery payload", response)
			}
		})
	}
}

// testPanicAPI 定义用于验证 Recovery 错误详情策略的 API。
type testPanicAPI struct {
	MethodGet
}

func (testPanicAPI) Path() string {
	return "/panic"
}

func (testPanicAPI) Output(context.Context) (any, error) {
	panic("panic secret")
}

// testPanicErrorAPI 定义用于验证 Recovery 不暴露 ErrorHttp panic 详情的 API。
type testPanicErrorAPI struct {
	MethodGet
}

func (testPanicErrorAPI) Path() string {
	return "/panic-error"
}

func (testPanicErrorAPI) Output(context.Context) (any, error) {
	panic(NewError(http.StatusTeapot, "panic secret", "panic detail"))
}

// TestRequestLogSecurityControls 验证默认查询参数脱敏。
func TestRequestLogSecurityControls(t *testing.T) {
	u := mustParseTestURL(t, "/users?token=secret&name=alice")
	redacted := redactRequestURI(u, map[string]struct{}{"token": {}})
	if redacted != "/users?name=alice" {
		t.Fatalf("redacted URI = %q, want /users?name=alice", redacted)
	}
	if u.RawQuery != "token=secret&name=alice" {
		t.Fatalf("original URL was modified: %q", u.RawQuery)
	}
	if got := redactRequestURI(&url.URL{Path: "/health"}, map[string]struct{}{"token": {}}); got != "/health" {
		t.Fatalf("URI without query = %q, want /health", got)
	}
	noSensitive := mustParseTestURL(t, "/users?name=alice")
	if got := redactRequestURI(noSensitive, map[string]struct{}{"token": {}}); got != "/users?name=alice" {
		t.Fatalf("URI without sensitive query = %q, want /users?name=alice", got)
	}
	encodedSensitive := mustParseTestURL(t, "/users?TOKEN=secret&%61ccess_token=hidden&name=alice")
	if got := redactRequestURI(encodedSensitive, map[string]struct{}{"token": {}, "access_token": {}}); got != "/users?name=alice" {
		t.Fatalf("URI with encoded sensitive query = %q, want /users?name=alice", got)
	}
	malformedSensitive := mustParseTestURL(t, "/users?token=secret;scope=all&name=alice")
	if got := redactRequestURI(malformedSensitive, map[string]struct{}{"token": {}}); got != "/users?name=alice" {
		t.Fatalf("URI with malformed sensitive query = %q, want /users?name=alice", got)
	}
	if malformedSensitive.RawQuery != "token=secret;scope=all&name=alice" {
		t.Fatalf("malformed original URL was modified: %q", malformedSensitive.RawQuery)
	}

	defaultOptions := defaultServerOptions()
	if err := WithRedactQueryParameters()(&defaultOptions); err != nil {
		t.Fatalf("WithRedactQueryParameters returned error: %v", err)
	}
	for _, name := range defaultRedactedQueryParameters {
		if _, ok := defaultOptions.redactQueryParameters[name]; !ok {
			t.Fatalf("default sensitive query parameter %q is not configured: %#v", name, defaultOptions.redactQueryParameters)
		}
	}
}

// TestClientLogHeaderCannotSuppressAccessLog 验证客户端请求头不能关闭访问日志，且默认脱敏 Query。
func TestClientLogHeaderCannotSuppressAccessLog(t *testing.T) {
	server := NewServer("test", ":0", false)
	group := NewRouterGroup("/")
	group.RegisterAPI(&testAnyRoute{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/any?token=secret;scope=all&name=alice", nil)
	request.Header.Set("x-log-level", "disabled")
	output := captureTestStderr(t, func() {
		server.engine.ServeHTTP(recorder, request)
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	fields := decodeTestLogLine(t, output)
	if fields["level"] != "info" || fields["tag"] != "access" || fields["request_uri"] != "/any?name=alice" {
		t.Fatalf("unexpected access log fields: %#v", fields)
	}
	if strings.Contains(output, "secret") {
		t.Fatalf("access log leaked sensitive query: %s", output)
	}
}

// TestServerServeShutdownAndTracerProvider 验证外部 Listener、优雅关闭和 Provider 注入。
func TestServerServeShutdownAndTracerProvider(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter))
	defer func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("provider Shutdown: %v", err)
		}
	}()
	server, err := NewServerWithOptions("test", "", false,
		WithTracerProvider(provider),
		WithHTTPTimeouts(time.Second, 2*time.Second, 3*time.Second, 4*time.Second),
	)
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}
	group := NewRouterGroup("/")
	group.RegisterAPI(&testAnyRoute{})
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	if server.httpServer.ReadHeaderTimeout != time.Second || server.httpServer.WriteTimeout != 3*time.Second {
		t.Fatalf("http.Server timeouts not applied: %+v", server.httpServer)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	client := &http.Client{Timeout: 2 * time.Second}
	var response *http.Response
	for i := 0; i < 20; i++ {
		response, err = client.Get("http://" + listener.Addr().String() + "/any")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /any: %v", err)
	}
	body, readErr := io.ReadAll(response.Body)
	if closeErr := response.Body.Close(); closeErr != nil {
		t.Fatalf("close response body: %v", closeErr)
	}
	if readErr != nil || string(body) != "route-ok" {
		t.Fatalf("response body = %q, read error = %v", body, readErr)
	}
	if err := server.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		t.Fatalf("Serve error = %v, want http.ErrServerClosed", err)
	}
	if err := provider.ForceFlush(context.Background()); err != nil {
		t.Fatalf("ForceFlush: %v", err)
	}
	if len(exporter.GetSpans()) == 0 {
		t.Fatal("injected tracer provider exported no spans")
	}
}

// TestServerLifecycleValidation 验证 Serve、Shutdown 和重复 Setup 的错误边界。
func TestServerLifecycleValidation(t *testing.T) {
	server := NewServer("test", ":0", false)
	if err := server.Serve(nil); err == nil {
		t.Fatal("Serve before Setup should fail")
	}
	if err := server.Shutdown(context.Background()); err == nil {
		t.Fatal("Shutdown before Setup should fail")
	}
	group := NewRouterGroup("/")
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	if err := server.Setup(group); err == nil {
		t.Fatal("second Setup should fail")
	}
}

// TestServerStartupLogOccursInServe 验证 Setup 不宣称已监听，Serve 使用外部 Listener 的真实地址。
func TestServerStartupLogOccursInServe(t *testing.T) {
	server := NewServer("test", ":configured", false)
	group := NewRouterGroup("/")
	var setupErr error
	setupOutput := captureTestStdout(t, func() {
		setupErr = server.Setup(group)
	})
	if setupErr != nil {
		t.Fatalf("Setup returned error: %v", setupErr)
	}
	if strings.Contains(setupOutput, "Listening and serving") {
		t.Fatalf("Setup emitted a listening message before Serve: %q", setupOutput)
	}

	listener := &testClosedListener{
		address: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 43210},
	}
	listenerAddress := listener.Addr().String()

	var serveErr error
	serveOutput := captureTestStdout(t, func() {
		serveErr = server.Serve(listener)
	})
	if serveErr == nil {
		t.Fatal("Serve with a closed listener should fail")
	}
	if !strings.Contains(serveOutput, listenerAddress) || strings.Contains(serveOutput, ":configured") {
		t.Fatalf("Serve output = %q, want external listener address %q", serveOutput, listenerAddress)
	}
}

// captureTestStdout 捕获函数执行期间写入标准输出的内容，并在返回前恢复全局输出句柄。
func captureTestStdout(t *testing.T, fn func()) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	previous := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = previous
		_ = writer.Close()
		_ = reader.Close()
	}()

	fn()
	os.Stdout = previous
	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	return string(output)
}

// TestServerSetupConcurrent 验证并发 Setup 只允许一个调用完成初始化。
func TestServerSetupConcurrent(t *testing.T) {
	server := NewServer("test", ":0", false)
	group := NewRouterGroup("/")
	group.RegisterAPI(&testAnyRoute{})

	const attempts = 8
	start := make(chan struct{})
	errs := make(chan error, attempts)
	var waitGroup sync.WaitGroup
	waitGroup.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func() {
			defer waitGroup.Done()
			<-start
			errs <- server.Setup(group)
		}()
	}
	close(start)
	waitGroup.Wait()
	close(errs)

	var success int
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 {
		t.Fatalf("successful Setup calls = %d, want 1", success)
	}
}

// TestServerSetupRetryAfterError 验证路由注册失败后可以安全重试，不会复用半成品引擎。
func TestServerSetupRetryAfterError(t *testing.T) {
	server := NewServer("test", ":0", false)
	invalid := NewRouterGroup("/")
	invalid.RegisterAPI(nil)
	if err := server.Setup(invalid); err == nil {
		t.Fatal("Setup with nil API should fail")
	}

	valid := NewRouterGroup("/")
	valid.RegisterAPI(&testAnyRoute{})
	if err := server.Setup(valid); err != nil {
		t.Fatalf("Setup retry returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	server.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/any", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "route-ok" {
		t.Fatalf("retry response = %d %q, want 200 route-ok", recorder.Code, recorder.Body.String())
	}
}

// TestServerLifecyclePreservesGinMode 验证构造和 Setup 均不会修改调用方选择的 Gin 模式。
func TestServerLifecyclePreservesGinMode(t *testing.T) {
	previousMode := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previousMode) })

	for _, debug := range []bool{false, true} {
		name := "release"
		if debug {
			name = "debug"
		}
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			server := NewServer("test", ":0", debug)
			if got := gin.Mode(); got != gin.TestMode {
				t.Fatalf("gin mode after %s NewServer = %q, want %q", name, got, gin.TestMode)
			}
			group := NewRouterGroup("/")
			group.RegisterAPI(&testAnyRoute{})
			if err := server.Setup(group); err != nil {
				t.Fatalf("Setup returned error: %v", err)
			}
			if got := gin.Mode(); got != gin.TestMode {
				t.Fatalf("gin mode after %s Setup = %q, want %q", name, got, gin.TestMode)
			}
		})
	}
}

// TestServerSetupRestoresStateAfterGinPanic 验证 Gin 初始化 panic 后会恢复 Setup 状态并允许重试。
func TestServerSetupRestoresStateAfterGinPanic(t *testing.T) {
	previousMode := gin.Mode()
	previousDebugPrintFunc := gin.DebugPrintFunc
	t.Cleanup(func() {
		gin.SetMode(previousMode)
		gin.DebugPrintFunc = previousDebugPrintFunc
	})

	server := NewServer("test", ":0", true)
	gin.SetMode(gin.DebugMode)
	gin.DebugPrintFunc = func(string, ...interface{}) {
		panic("debug print failure")
	}
	group := NewRouterGroup("/")
	group.RegisterAPI(&testAnyRoute{})

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = server.Setup(group)
	}()
	if recovered == nil {
		t.Fatal("Setup should propagate Gin debug print panic")
	}
	if got := gin.Mode(); got != gin.DebugMode {
		t.Fatalf("gin mode after panic = %q, want %q", got, gin.DebugMode)
	}

	gin.DebugPrintFunc = nil
	if err := server.Setup(group); err != nil {
		t.Fatalf("Setup retry after Gin panic returned error: %v", err)
	}
}

// TestServerSetupWhileAnotherServerRecovers 覆盖运行期 Gin 模式读取与另一 Server Setup 的并发路径。
func TestServerSetupWhileAnotherServerRecovers(t *testing.T) {
	previousMode := gin.Mode()
	previousErrorWriter := gin.DefaultErrorWriter
	t.Cleanup(func() {
		gin.SetMode(previousMode)
		gin.DefaultErrorWriter = previousErrorWriter
	})
	gin.SetMode(gin.TestMode)
	gin.DefaultErrorWriter = io.Discard

	runningServer, err := NewServerWithOptions("running", ":0", false, WithExposeErrorDetails(false))
	if err != nil {
		t.Fatalf("NewServerWithOptions returned error: %v", err)
	}
	runningGroup := NewRouterGroup("/")
	runningGroup.RegisterAPI(&testPanicAPI{})
	if err := runningServer.Setup(runningGroup); err != nil {
		t.Fatalf("running server Setup returned error: %v", err)
	}

	setupServer := NewServer("setup", ":0", true)
	setupGroup := NewRouterGroup("/")
	setupGroup.RegisterAPI(&testAnyRoute{})

	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	waitGroup.Add(1)
	go func() {
		defer waitGroup.Done()
		<-start
		for i := 0; i < 64; i++ {
			recorder := httptest.NewRecorder()
			runningServer.engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
		}
	}()
	close(start)
	if err := setupServer.Setup(setupGroup); err != nil {
		t.Fatalf("concurrent Setup returned error: %v", err)
	}
	waitGroup.Wait()
}

var benchmarkRedactedURI string

// BenchmarkRedactRequestURI 对比查询脱敏在无查询、无敏感参数和命中敏感参数时的分配成本。
func BenchmarkRedactRequestURI(b *testing.B) {
	tests := []struct {
		name string
		u    *url.URL
	}{
		{name: "no-query", u: &url.URL{Path: "/health"}},
		{name: "no-sensitive", u: mustParseBenchmarkURL("/users?name=alice&page=1")},
		{name: "sensitive", u: mustParseBenchmarkURL("/users?token=secret&name=alice")},
		{name: "malformed-sensitive", u: mustParseBenchmarkURL("/users?token=secret;scope=all&name=alice")},
	}
	sensitiveNames := map[string]struct{}{"token": {}}
	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				benchmarkRedactedURI = redactRequestURI(test.u, sensitiveNames)
			}
		})
	}
}

// mustParseBenchmarkURL 解析基准测试 URL，失败时终止基准测试初始化。
func mustParseBenchmarkURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

// mustParseTestURL 解析测试 URL，失败时终止当前测试。
func mustParseTestURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	return u
}
