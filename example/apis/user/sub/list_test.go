package sub

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestListSubGeneratedBindingInitializesEmbeddedPointer 验证生成绑定会初始化匿名嵌入指针。
func TestListSubGeneratedBindingInitializesEmbeddedPointer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/list?size=25", nil)

	handler := &ListSub{}
	if err := handler.EasyGinBindParameters(ctx); err != nil {
		t.Fatalf("EasyGinBindParameters returned error: %v", err)
	}
	if handler.ListSubQuery == nil {
		t.Fatal("embedded ListSubQuery was not initialized")
	}
	if handler.Size != 25 || handler.Offset != 0 {
		t.Fatalf("bound query = size:%d offset:%d, want size:25 offset:0", handler.Size, handler.Offset)
	}
}
