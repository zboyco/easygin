package easygin

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestGenerateOpenAPISupportsPatchMethod 验证 MethodPatch 能生成 OpenAPI patch operation。
func TestGenerateOpenAPISupportsPatchMethod(t *testing.T) {
	processedTypes = make(map[string]bool)
	paths := openapi3.Paths{}
	doc := &openapi3.T{
		Paths: &paths,
		Components: &openapi3.Components{
			Schemas: make(map[string]*openapi3.SchemaRef),
		},
	}

	group := NewRouterGroup("/")
	group.RegisterAPI(&testPatchOpenAPI{})

	if err := generateGroupPaths(doc, group, ""); err != nil {
		t.Fatalf("generateGroupPaths returned error: %v", err)
	}

	pathItem := doc.Paths.Value("/patch")
	if pathItem == nil || pathItem.Patch == nil {
		t.Fatalf("expected PATCH operation to be generated, got path item: %+v", pathItem)
	}
}

// testPatchOpenAPI 是用于验证 OpenAPI PATCH 方法生成的最小 API。
type testPatchOpenAPI struct {
	MethodPatch `summary:"patch test"`
}

// Path 返回测试 API 的路由路径。
func (*testPatchOpenAPI) Path() string {
	return "/patch"
}

// Output 返回测试 API 的响应。
func (*testPatchOpenAPI) Output(context.Context) (any, error) {
	return nil, nil
}
