package easygin

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// TestGenerateOpenAPISupportsPatchMethod 验证 MethodPatch 能生成 OpenAPI patch operation。
func TestGenerateOpenAPISupportsPatchMethod(t *testing.T) {
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

// TestGenerateOpenAPIRequiredContract 验证 Body 和路径参数与运行时必填语义一致。
func TestGenerateOpenAPIRequiredContract(t *testing.T) {
	doc := newTestOpenAPIDocument()
	group := NewRouterGroup("/")
	group.RegisterAPI(&testRequiredOpenAPI{})

	if err := generateGroupPaths(doc, group, ""); err != nil {
		t.Fatalf("generateGroupPaths returned error: %v", err)
	}
	operation := doc.Paths.Value("/items/{id}").Post
	if operation.RequestBody == nil || !operation.RequestBody.Value.Required {
		t.Fatal("request body must be required")
	}
	if len(operation.Parameters) != 1 || !operation.Parameters[0].Value.Required {
		t.Fatalf("path parameter must be required: %#v", operation.Parameters)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("generated document validation failed: %v", err)
	}
}

// TestGenerateSchemaExpandsAnonymousPointer 验证匿名嵌入指针的 JSON 字段不会从 Schema 丢失。
func TestGenerateSchemaExpandsAnonymousPointer(t *testing.T) {
	doc := newTestOpenAPIDocument()
	schema := generateSchemaValue(doc, reflect.TypeOf(testOpenAPIBody{}), false)

	for _, property := range []string{"name", "count"} {
		if schema.Properties[property] == nil {
			t.Fatalf("schema property %q is missing: %#v", property, schema.Properties)
		}
	}
}

// TestGenerateOpenAPIAnyOperationIDsAreUnique 验证 ANY 展开的每个方法都有唯一 operationId。
func TestGenerateOpenAPIAnyOperationIDsAreUnique(t *testing.T) {
	doc := newTestOpenAPIDocument()
	group := NewRouterGroup("/")
	group.RegisterAPI(testAnyOpenAPI{})

	if err := generateGroupPaths(doc, group, ""); err != nil {
		t.Fatalf("generateGroupPaths returned error: %v", err)
	}
	pathItem := doc.Paths.Value("/any")
	operations := []*openapi3.Operation{
		pathItem.Get,
		pathItem.Post,
		pathItem.Put,
		pathItem.Delete,
		pathItem.Patch,
		pathItem.Head,
		pathItem.Options,
		pathItem.Connect,
		pathItem.Trace,
	}
	seen := make(map[string]bool, len(operations))
	for _, operation := range operations {
		if operation == nil || operation.OperationID == "" {
			t.Fatalf("ANY operation is missing operationId: %#v", operation)
		}
		if seen[operation.OperationID] {
			t.Fatalf("duplicate operationId %q", operation.OperationID)
		}
		seen[operation.OperationID] = true
	}
}

// TestGenerateOpenAPIPreservesExistingFileOnError 验证生成失败不会覆盖已有文档。
func TestGenerateOpenAPIPreservesExistingFileOnError(t *testing.T) {
	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	temporaryDir := t.TempDir()
	if err := os.Chdir(temporaryDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previousDir); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	filename := filepath.Join(temporaryDir, "openapi.json")
	const previousContent = "previous document"
	if err := os.WriteFile(filename, []byte(previousContent), 0o644); err != nil {
		t.Fatalf("write previous document: %v", err)
	}
	group := NewRouterGroup("/")
	group.RegisterAPI(&testInvalidMethodOpenAPI{})
	if err := GenerateOpenAPI(group); err == nil {
		t.Fatal("expected invalid HTTP method error")
	}

	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read previous document: %v", err)
	}
	if string(content) != previousContent {
		t.Fatalf("existing document was replaced: %q", content)
	}
}

// newTestOpenAPIDocument 创建具备校验所需基础信息的空文档。
func newTestOpenAPIDocument() *openapi3.T {
	paths := openapi3.Paths{}
	return &openapi3.T{
		OpenAPI: "3.0.3",
		Info:    &openapi3.Info{Title: "test", Version: "1.0.0"},
		Paths:   &paths,
		Components: &openapi3.Components{
			Schemas: make(map[string]*openapi3.SchemaRef),
		},
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

// testRequiredOpenAPI 验证 Body、路径参数必填及重复参数去重。
type testRequiredOpenAPI struct {
	MethodPost
	ID      string          `in:"path" name:"id,omitempty"`
	IDAgain string          `in:"path" name:"id,omitempty"`
	Body    testOpenAPIBody `in:"body"`
}

func (*testRequiredOpenAPI) Path() string {
	return "/items/:id"
}

func (*testRequiredOpenAPI) Output(context.Context) (any, error) {
	return nil, nil
}

// testOpenAPIEmbeddedBody 定义需要由父 Schema 展开的字段。
type testOpenAPIEmbeddedBody struct {
	Name string `json:"name"`
}

// testOpenAPIBody 验证匿名嵌入指针 Schema。
type testOpenAPIBody struct {
	*testOpenAPIEmbeddedBody
	Count int `json:"count"`
}

// testAnyOpenAPI 验证值类型 ANY 处理器及 operationId 展开。
type testAnyOpenAPI struct {
	MethodAny
}

func (testAnyOpenAPI) Path() string {
	return "/any"
}

func (testAnyOpenAPI) Output(context.Context) (any, error) {
	return nil, nil
}

// testInvalidMethodOpenAPI 验证非法 HTTP 方法不会产生部分文档。
type testInvalidMethodOpenAPI struct{}

func (*testInvalidMethodOpenAPI) Method() string {
	return "INVALID"
}

func (*testInvalidMethodOpenAPI) Path() string {
	return "/invalid"
}

func (*testInvalidMethodOpenAPI) Output(context.Context) (any, error) {
	return nil, nil
}
