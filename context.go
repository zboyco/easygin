package easygin

import (
	"context"

	"github.com/gin-gonic/gin"
)

type contextKey int

// ContextWithHandlerName 将处理器名称存储到上下文中
func ContextWithHandlerName(ctx context.Context, handlerName string) context.Context {
	return context.WithValue(ctx, contextKey(0), handlerName)
}

// HandlerNameFromContext 从上下文中获取处理器名称
func HandlerNameFromContext(ctx context.Context) string {
	if handlerName, ok := ctx.Value(contextKey(0)).(string); ok {
		return handlerName
	}
	return ""
}

// ContextWithGinContext 将 gin.Context 存储到上下文中
func ContextWithGinContext(ctx context.Context, c *gin.Context) context.Context {
	return context.WithValue(ctx, contextKey(1), c)
}

// GinContextFromContext 从上下文中获取 gin.Context
func GinContextFromContext(ctx context.Context) *gin.Context {
	raw := ctx.Value(contextKey(1))
	if raw == nil {
		return nil
	}
	return raw.(*gin.Context)
}

// ContextWithRoute 将 Route 存储到上下文中
func ContextWithRoute(ctx context.Context, api RouterAPI) context.Context {
	return context.WithValue(ctx, contextKey(2), api)
}

// RouteFromContext 从上下文中获取 RouterAPI
func RouteFromContext(ctx context.Context) RouterAPI {
	raw := ctx.Value(contextKey(2))
	if raw == nil {
		return nil
	}
	return raw.(RouterAPI)
}
