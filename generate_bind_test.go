package easygin

import (
	"context"
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
	generatePathBinding(&builder, "r.ID", "item_id", field)
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
	generateQueryBinding(&builder, "r.Name", "name", field)
	output := builder.String()

	if !strings.Contains(output, `queryVal = "anonymous"`) {
		t.Fatalf("expected default value assignment, got:\n%s", output)
	}
	if strings.Contains(output, `missing required parameter 'name' in query`) {
		t.Fatalf("omitempty field should not generate required error, got:\n%s", output)
	}
}

func TestGenerateQuerySliceBindingIntPtr(t *testing.T) {
	type sliceStruct struct {
		IDs []*int `in:"query" name:"ids"`
	}
	field := reflect.TypeOf(sliceStruct{}).Field(0)

	var builder strings.Builder
	ok := generateQuerySliceBinding(&builder, "r.IDs", "ids", field.Type, false, false)
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

func TestGenerateBodyBindingJSONPointer(t *testing.T) {
	type bodyStruct struct {
		Payload *struct {
			Field string `json:"field"`
		} `in:"body"`
	}
	field := reflect.TypeOf(bodyStruct{}).Field(0)

	var builder strings.Builder
	generateBodyBinding(&builder, "r.Payload", field, reflect.TypeOf(bodyStruct{}).PkgPath())
	output := builder.String()

	if !strings.Contains(output, "if r.Payload == nil") {
		t.Fatalf("expected pointer instantiation guard, got:\n%s", output)
	}
	if !strings.Contains(output, "decoder := json.NewDecoder") {
		t.Fatalf("expected JSON decoder block, got:\n%s", output)
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
	generateBodyBinding(&builder, "r.Form", field, reflect.TypeOf(multipartForm{}).PkgPath())
	output := builder.String()

	if !strings.Contains(output, "ParseMultipartForm") {
		t.Fatalf("expected multipart parsing block, got:\n%s", output)
	}
	if !strings.Contains(output, `return errors.New("missing required file 'upload'")`) {
		t.Fatalf("expected required file error, got:\n%s", output)
	}
	if !strings.Contains(output, `c.PostFormArray("tags")`) {
		t.Fatalf("expected slice form binding, got:\n%s", output)
	}
}

func TestGenerateFileContentDedupAndImports(t *testing.T) {
	content := generateFileContent(
		"github.com/zboyco/easygin/custompkg",
		[]RouterHandler{
			&complexBindingAPI{},
			&complexBindingAPI{},
			&headerOnlyAPI{},
		},
	)

	if count := strings.Count(content, "func (r *complexBindingAPI) EasyGinBindParameters("); count != 1 {
		t.Fatalf("expected 1 generated method for complexBindingAPI, got %d", count)
	}

	for _, pkg := range []string{
		"\"encoding/json\"",
		"\"errors\"",
		"\"fmt\"",
		"\"reflect\"",
		"\"strconv\"",
		"\"strings\"",
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
