package easygin

import (
	"context"
	"encoding"
	"mime/multipart"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestProcessRouterGroupSkipsNoGenParameter(t *testing.T) {
	root := NewRouterGroup("/")
	root.middlewares = []RouterHandler{
		&testMiddleware{name: "root-mw"},
		&testMiddlewareNoGen{},
	}
	root.apis = []RouterAPI{
		&testSimpleAPI{name: "root-api"},
		&testSkipAPI{},
	}

	child := NewRouterGroup("/child")
	child.middlewares = []RouterHandler{&testMiddleware{name: "child-mw"}}
	child.apis = []RouterAPI{&testSimpleAPI{name: "child-api"}}
	root.children = []*RouterGroup{child}

	handlers, err := processRouterGroup(root)
	if err != nil {
		t.Fatalf("processRouterGroup returned error: %v", err)
	}

	if len(handlers) != 4 {
		t.Fatalf("expected 4 handlers, got %d", len(handlers))
	}

	assertMiddleware(t, handlers[0], "root-mw")
	assertAPI(t, handlers[1], "root-api")
	assertMiddleware(t, handlers[2], "child-mw")
	assertAPI(t, handlers[3], "child-api")
}

func TestGeneratePathBindingRequired(t *testing.T) {
	type pathStruct struct {
		ID int `in:"path" name:"item_id"`
	}
	field := reflect.TypeOf(pathStruct{}).Field(0)

	var builder strings.Builder
	if err := generatePathBinding(&builder, "r.ID", "item_id", field, reflect.TypeOf(pathStruct{}).PkgPath(), nil); err != nil {
		t.Fatalf("generatePathBinding returned error: %v", err)
	}
	output := builder.String()

	if !strings.Contains(output, `return errors.New("missing required parameter 'item_id' in path")`) {
		t.Fatalf("missing required parameter guard in generated code:\n%s", output)
	}
	if !strings.Contains(output, `strconv.ParseInt(pathVal, 10, 64)`) {
		t.Fatalf("missing integer conversion in generated code:\n%s", output)
	}
}

func TestGenerateQueryBindingOptionalDefault(t *testing.T) {
	type queryStruct struct {
		Name string `in:"query" name:"name,omitempty" default:"anonymous"`
	}
	field := reflect.TypeOf(queryStruct{}).Field(0)

	var builder strings.Builder
	if err := generateQueryBinding(&builder, "r.Name", "name", field, reflect.TypeOf(queryStruct{}).PkgPath(), nil); err != nil {
		t.Fatalf("generateQueryBinding returned error: %v", err)
	}
	output := builder.String()

	if !strings.Contains(output, `queryVal = "anonymous"`) {
		t.Fatalf("expected default value assignment, got:\n%s", output)
	}
	if strings.Contains(output, `missing required parameter 'name' in query`) {
		t.Fatalf("omitempty field should not generate required error, got:\n%s", output)
	}
}

// TestGenerateQueryBindingPreservesExplicitZeroValues 验证静态 query 绑定不应把 0/false 改写为空字符串。
func TestGenerateQueryBindingPreservesExplicitZeroValues(t *testing.T) {
	type queryParams struct {
		Count   int  `in:"query" name:"count"`
		Enabled bool `in:"query" name:"enabled"`
	}

	tests := []struct {
		name          string
		fieldIndex    int
		forbiddenCode string
	}{
		{name: "int zero", fieldIndex: 0, forbiddenCode: `queryVal == "0"`},
		{name: "bool false", fieldIndex: 1, forbiddenCode: `queryVal == "false"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := reflect.TypeOf(queryParams{}).Field(tt.fieldIndex)
			var builder strings.Builder
			if err := generateQueryBinding(&builder, "r."+field.Name, strings.Split(field.Tag.Get("name"), ",")[0], field, reflect.TypeOf(queryParams{}).PkgPath(), nil); err != nil {
				t.Fatalf("generateQueryBinding returned error: %v", err)
			}
			if output := builder.String(); strings.Contains(output, tt.forbiddenCode) {
				t.Fatalf("generated binding treats explicit zero value as missing:\n%s", output)
			}
		})
	}
}

func TestGenerateBindingUsesFieldBitSize(t *testing.T) {
	type bitSizeParams struct {
		SmallInt    int8    `in:"query" name:"small_int"`
		SmallUint   uint8   `in:"query" name:"small_uint"`
		SmallFloat  float32 `in:"query" name:"small_float"`
		SmallIntIDs []int8  `in:"query" name:"small_int_id"`
	}

	structType := reflect.TypeOf(bitSizeParams{})
	tests := []struct {
		name       string
		fieldIndex int
		want       string
	}{
		{name: "int8", fieldIndex: 0, want: "strconv.ParseInt(queryVal, 10, 8)"},
		{name: "uint8", fieldIndex: 1, want: "strconv.ParseUint(queryVal, 10, 8)"},
		{name: "float32", fieldIndex: 2, want: "strconv.ParseFloat(queryVal, 32)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := structType.Field(tt.fieldIndex)
			var builder strings.Builder
			if err := generateQueryBinding(&builder, "r."+field.Name, strings.Split(field.Tag.Get("name"), ",")[0], field, structType.PkgPath(), nil); err != nil {
				t.Fatalf("generateQueryBinding returned error: %v", err)
			}
			if output := builder.String(); !strings.Contains(output, tt.want) {
				t.Fatalf("generated binding should use field bit size %q, got:\n%s", tt.want, output)
			}
		})
	}

	t.Run("slice int8", func(t *testing.T) {
		field := structType.Field(3)
		var builder strings.Builder
		ok := generateQuerySliceBinding(&builder, "r."+field.Name, "small_int_id", field.Type, false, false, structType.PkgPath(), nil)
		if !ok {
			t.Fatal("expected int8 query slice to be supported")
		}
		if output := builder.String(); !strings.Contains(output, "strconv.ParseInt(val, 10, 8)") {
			t.Fatalf("generated slice binding should use element bit size, got:\n%s", output)
		}
	})
}

func TestGenerateBindingInvalidDefaultReturnsError(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		generate func(*strings.Builder, reflect.StructField) error
	}{
		{
			name: "path",
			tag:  `in:"path" name:"id,omitempty" default:"bad"`,
			generate: func(builder *strings.Builder, field reflect.StructField) error {
				return generatePathBinding(builder, "r.ID", "id", field, reflect.TypeOf(struct{}{}).PkgPath(), nil)
			},
		},
		{
			name: "query",
			tag:  `in:"query" name:"id,omitempty" default:"bad"`,
			generate: func(builder *strings.Builder, field reflect.StructField) error {
				return generateQueryBinding(builder, "r.ID", "id", field, reflect.TypeOf(struct{}{}).PkgPath(), nil)
			},
		},
		{
			name: "header",
			tag:  `in:"header" name:"id,omitempty" default:"bad"`,
			generate: func(builder *strings.Builder, field reflect.StructField) error {
				return generateHeaderBinding(builder, "r.ID", "id", field, reflect.TypeOf(struct{}{}).PkgPath(), nil)
			},
		},
		{
			name: "form",
			tag:  `name:"id,omitempty" default:"bad"`,
			generate: func(builder *strings.Builder, field reflect.StructField) error {
				return generateFormBinding(builder, "r.ID", "id", field, reflect.TypeOf(struct{}{}).PkgPath(), nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field := fieldWithTag(t, tt.tag)
			var builder strings.Builder
			err := tt.generate(&builder, field)
			if err == nil {
				t.Fatal("expected invalid default to return error")
			}
			if !strings.Contains(err.Error(), "default value") {
				t.Fatalf("expected default value error, got: %v", err)
			}
		})
	}
}

func TestGenerateQuerySliceBindingIntPtr(t *testing.T) {
	type sliceStruct struct {
		IDs []*int `in:"query" name:"ids"`
	}
	field := reflect.TypeOf(sliceStruct{}).Field(0)

	var builder strings.Builder
	ok := generateQuerySliceBinding(&builder, "r.IDs", "ids", field.Type, false, false, reflect.TypeOf(sliceStruct{}).PkgPath(), nil)
	if !ok {
		t.Fatal("expected slice type to be supported")
	}

	output := builder.String()
	if !strings.Contains(output, "strconv.ParseInt") {
		t.Fatalf("missing integer parsing in generated slice binding:\n%s", output)
	}
	if !strings.Contains(output, "convertedVals = append(convertedVals, &valCopy)") {
		t.Fatalf("missing pointer element handling in generated slice binding:\n%s", output)
	}
}

func TestGenerateQuerySliceBindingTextUnmarshaler(t *testing.T) {
	type sliceStruct struct {
		Values []testTextValue `in:"query" name:"values"`
	}
	field := reflect.TypeOf(sliceStruct{}).Field(0)

	var builder strings.Builder
	ok := generateQuerySliceBinding(&builder, "r.Values", "values", field.Type, false, false, reflect.TypeOf(sliceStruct{}).PkgPath(), nil)
	if !ok {
		t.Fatal("expected TextUnmarshaler slice type to be supported")
	}

	output := builder.String()
	if !strings.Contains(output, "parsedVal := new(testTextValue)") {
		t.Fatalf("missing TextUnmarshaler element allocation in generated slice binding:\n%s", output)
	}
	if !strings.Contains(output, "if err := parsedVal.UnmarshalText([]byte(val)); err != nil {") {
		t.Fatalf("missing TextUnmarshaler parsing in generated slice binding:\n%s", output)
	}
	if !strings.Contains(output, "convertedVals = append(convertedVals, *parsedVal)") {
		t.Fatalf("missing value element append in generated slice binding:\n%s", output)
	}
}

func TestGenerateQuerySliceBindingTextUnmarshalerPtrElem(t *testing.T) {
	type sliceStruct struct {
		Values []*testTextValue `in:"query" name:"values"`
	}
	field := reflect.TypeOf(sliceStruct{}).Field(0)

	var builder strings.Builder
	ok := generateQuerySliceBinding(&builder, "r.Values", "values", field.Type, false, false, reflect.TypeOf(sliceStruct{}).PkgPath(), nil)
	if !ok {
		t.Fatal("expected TextUnmarshaler pointer element slice type to be supported")
	}

	output := builder.String()
	if !strings.Contains(output, "convertedVals := make([]*testTextValue, 0, len(queryVals))") {
		t.Fatalf("missing pointer element slice allocation in generated slice binding:\n%s", output)
	}
	if !strings.Contains(output, "convertedVals = append(convertedVals, parsedVal)") {
		t.Fatalf("missing pointer element append in generated slice binding:\n%s", output)
	}
	if strings.Contains(output, "append(convertedVals, *parsedVal)") {
		t.Fatalf("pointer element slice should append parsed pointers, got:\n%s", output)
	}
}

func TestGenerateBodyBindingJSONPointer(t *testing.T) {
	type bodyStruct struct {
		Payload *struct {
			Field string `json:"field"`
		} `in:"body"`
	}
	field := reflect.TypeOf(bodyStruct{}).Field(0)

	var builder strings.Builder
	if err := generateBodyBinding(&builder, "r.Payload", field, reflect.TypeOf(bodyStruct{}).PkgPath(), nil); err != nil {
		t.Fatalf("generateBodyBinding returned error: %v", err)
	}
	output := builder.String()

	if !strings.Contains(output, "if r.Payload == nil") {
		t.Fatalf("expected pointer instantiation guard, got:\n%s", output)
	}
	if !strings.Contains(output, "decoder := json.NewDecoder") {
		t.Fatalf("expected JSON decoder block, got:\n%s", output)
	}
	if !strings.Contains(output, "decoder.Decode(&struct{}{})") || !strings.Contains(output, "multiple JSON values are not allowed") {
		t.Fatalf("expected multiple JSON value guard, got:\n%s", output)
	}
	if !strings.Contains(output, "easygin.ValidateJsonRequiredFields") {
		t.Fatalf("expected validation call, got:\n%s", output)
	}
}

func TestGenerateBodyBindingMultipartForm(t *testing.T) {
	type multipartForm struct {
		Form struct {
			Upload *multipart.FileHeader `name:"upload"`
			Tags   []string              `name:"tags,omitempty"`
		} `in:"body" mime:"multipart"`
	}
	field := reflect.TypeOf(multipartForm{}).Field(0)

	var builder strings.Builder
	if err := generateBodyBinding(&builder, "r.Form", field, reflect.TypeOf(multipartForm{}).PkgPath(), nil); err != nil {
		t.Fatalf("generateBodyBinding returned error: %v", err)
	}
	output := builder.String()

	if !strings.Contains(output, "ParseMultipartForm") {
		t.Fatalf("expected multipart parsing block, got:\n%s", output)
	}
	if strings.Contains(output, "1 << 30") {
		t.Fatalf("generated multipart binding hard-codes memory limit instead of using configuration:\n%s", output)
	}
	if !strings.Contains(output, "GetMultipartMemoryLimit()") {
		t.Fatalf("generated multipart binding should call GetMultipartMemoryLimit():\n%s", output)
	}
	if !strings.Contains(output, `return errors.New("missing required file 'upload'")`) {
		t.Fatalf("expected required file error, got:\n%s", output)
	}
	if !strings.Contains(output, `c.PostFormArray("tags")`) {
		t.Fatalf("expected slice form binding, got:\n%s", output)
	}
}

func TestGenerateFileContentDedupAndImports(t *testing.T) {
	content, err := generateFileContent(
		"github.com/zboyco/easygin/custompkg",
		[]RouterHandler{
			&complexBindingAPI{},
			&complexBindingAPI{},
			&headerOnlyAPI{},
		},
	)
	if err != nil {
		t.Fatalf("generateFileContent returned error: %v", err)
	}

	if count := strings.Count(content, "func (r *complexBindingAPI) EasyGinBindParameters("); count != 1 {
		t.Fatalf("expected 1 generated method for complexBindingAPI, got %d", count)
	}

	for _, pkg := range []string{
		"\"bytes\"",
		"\"encoding/json\"",
		"\"errors\"",
		"\"fmt\"",
		"\"reflect\"",
		"\"strconv\"",
		"\"time\"",
		"\"github.com/gin-gonic/gin\"",
		"\"github.com/zboyco/easygin\"",
	} {
		if !strings.Contains(content, pkg) {
			t.Fatalf("expected import %s in generated content:\n%s", pkg, content)
		}
	}

	if !strings.Contains(content, "package custompkg") {
		t.Fatalf("expected package declaration for custompkg, got:\n%s", content)
	}
}

func TestGenerateFileContentUnsupportedQuerySliceFails(t *testing.T) {
	_, err := generateFileContent(
		"github.com/zboyco/easygin",
		[]RouterHandler{&unsupportedQuerySliceAPI{}},
	)
	if err == nil {
		t.Fatal("expected unsupported query slice type to fail generation")
	}
	if !strings.Contains(err.Error(), "unsupported query slice parameter type") {
		t.Fatalf("expected unsupported query slice error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Unsupported") {
		t.Fatalf("expected API type context in error, got: %v", err)
	}
}

// --- Helpers ----------------------------------------------------------------

type testMiddleware struct {
	name string
}

func (m *testMiddleware) Output(context.Context) (any, error) {
	return m.name, nil
}

type testMiddlewareNoGen struct{}

func (testMiddlewareNoGen) Output(context.Context) (any, error) {
	return nil, nil
}

func (testMiddlewareNoGen) IgnoreGenParameter() {}

type testTextValue string

var _ encoding.TextUnmarshaler = (*testTextValue)(nil)

func (v *testTextValue) UnmarshalText(data []byte) error {
	*v = testTextValue(data)
	return nil
}

type testSimpleAPI struct {
	name string
}

func (a *testSimpleAPI) Method() string { return "GET" }

func (a *testSimpleAPI) Path() string { return "/" + a.name }

func (a *testSimpleAPI) Output(context.Context) (any, error) { return a.name, nil }

type testSkipAPI struct {
	testSimpleAPI
}

func (testSkipAPI) IgnoreGenParameter() {}

func assertMiddleware(t *testing.T, handler RouterHandler, name string) {
	t.Helper()
	mw, ok := handler.(*testMiddleware)
	if !ok {
		t.Fatalf("expected middleware, got %T", handler)
	}
	if mw.name != name {
		t.Fatalf("expected middleware %s, got %s", name, mw.name)
	}
}

func assertAPI(t *testing.T, handler RouterHandler, name string) {
	t.Helper()
	api, ok := handler.(*testSimpleAPI)
	if !ok {
		t.Fatalf("expected API, got %T", handler)
	}
	if api.name != name {
		t.Fatalf("expected API %s, got %s", name, api.name)
	}
}

func fieldWithTag(t *testing.T, tag string) reflect.StructField {
	t.Helper()

	structType := reflect.StructOf([]reflect.StructField{{
		Name: "ID",
		Type: reflect.TypeOf(0),
		Tag:  reflect.StructTag(tag),
	}})
	return structType.Field(0)
}

type complexBindingAPI struct {
	PathID     int       `in:"path" name:"id"`
	QueryVals  []int     `in:"query" name:"values"`
	Timestamp  time.Time `in:"query" name:"ts,omitempty"`
	HeaderAuth string    `in:"header" name:"Authorization"`
	Body       *struct {
		Name string `json:"name"`
		Tags []int  `json:"tags"`
	} `in:"body"`
}

func (a *complexBindingAPI) Method() string { return "GET" }

func (a *complexBindingAPI) Path() string { return "/complex" }

func (a *complexBindingAPI) Output(context.Context) (any, error) { return nil, nil }

type headerOnlyAPI struct {
	Header bool `in:"header" name:"x-flag"`
}

func (a *headerOnlyAPI) Method() string { return "GET" }

func (a *headerOnlyAPI) Path() string { return "/header" }

func (a *headerOnlyAPI) Output(context.Context) (any, error) { return nil, nil }

type unsupportedQuerySliceAPI struct {
	Unsupported []struct{} `in:"query" name:"unsupported"`
}

func (a *unsupportedQuerySliceAPI) Method() string { return "GET" }

func (a *unsupportedQuerySliceAPI) Path() string { return "/unsupported" }

func (a *unsupportedQuerySliceAPI) Output(context.Context) (any, error) { return nil, nil }
