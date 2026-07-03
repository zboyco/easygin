package easygin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace/noop"
)

// TestRouteFromContextReturnsNilForUnexpectedValue 确认上下文值异常时不会触发 panic。
func TestRouteFromContextReturnsNilForUnexpectedValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey(2), "not a route")
	if route := RouteFromContext(ctx); route != nil {
		t.Fatalf("expected nil route for unexpected context value, got %T", route)
	}
}

// TestAnyRouteIsAvailableFromContext 覆盖 ANY 路由在真实 HTTP 方法下的上下文注入。
func TestAnyRouteIsAvailableFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prevWriter := gin.DefaultWriter
	gin.DefaultWriter = io.Discard
	t.Cleanup(func() {
		gin.DefaultWriter = prevWriter
	})

	group := NewRouterGroup("/")
	route := &testAnyRoute{}
	group.RegisterAPI(route)

	s := NewServer("test", ":0", false)
	s.handlerMap = make(map[string]RouterAPI)
	s.engine.Use(func(c *gin.Context) {
		mainHandler := routeFromHandlerMap(s.handlerMap, c.Request.Method, c.FullPath())
		if mainHandler != nil {
			c.Request = c.Request.WithContext(ContextWithRoute(c.Request.Context(), mainHandler))
		}
	})
	handleGroup(s.handlerMap, &s.engine.RouterGroup, group)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/any", nil)
	s.engine.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if body := recorder.Body.String(); body != "route-ok" {
		t.Fatalf("body = %q, want %q", body, "route-ok")
	}
}

// TestSpanLoggerStartPreservesServiceName 确认子 logger 继续使用父 logger 的 serviceName。
func TestSpanLoggerStartPreservesServiceName(t *testing.T) {
	_, span := noop.NewTracerProvider().Tracer("root").Start(context.Background(), "root")
	parent := SpanLogger("svc", span).(*spanLogger)

	_, child := parent.Start(context.Background(), "child")
	childLogger, ok := child.(*spanLogger)
	if !ok {
		t.Fatalf("expected *spanLogger, got %T", child)
	}
	if childLogger.serviceName != "svc" {
		t.Fatalf("child serviceName = %q, want %q", childLogger.serviceName, "svc")
	}
}

type testAnyRoute struct {
	MethodAny
}

func (*testAnyRoute) Path() string {
	return "/any"
}

func (r *testAnyRoute) Output(ctx context.Context) (any, error) {
	if RouteFromContext(ctx) == nil {
		return "route-missing", nil
	}
	return "route-ok", nil
}
