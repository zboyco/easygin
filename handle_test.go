package easygin

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDecodeJSONRejectsMultipleJSONValues(t *testing.T) {
	var payload TestBodyPayload
	err := decodeJSON(strings.NewReader(`{"value":"one"}{"value":"two"}`), &payload)
	if err == nil {
		t.Fatal("expected multiple JSON values to be rejected")
	}
	if !strings.Contains(err.Error(), "multiple JSON values are not allowed") {
		t.Fatalf("expected multiple JSON values error, got: %v", err)
	}
}

// TestDecodeJSONRequiredAllowsExplicitZeroValues 验证 JSON 中显式提供的零值不应被当作缺失字段。
func TestDecodeJSONRequiredAllowsExplicitZeroValues(t *testing.T) {
	prev := HandleBodyJsonOmitEmptyAndDefault()
	SetHandleBodyJsonOmitEmptyAndDefault(true)
	t.Cleanup(func() {
		SetHandleBodyJsonOmitEmptyAndDefault(prev)
	})

	type payload struct {
		Count   int     `json:"count"`
		Enabled bool    `json:"enabled"`
		Ratio   float64 `json:"ratio"`
	}

	var got payload
	err := decodeJSON(strings.NewReader(`{"count":0,"enabled":false,"ratio":0}`), &got)
	if err != nil {
		t.Fatalf("explicit zero values should be accepted as present fields, got error: %v", err)
	}
}

// TestValidateJsonRequiredFieldsAnonymousStructCacheIsolation 验证匿名结构体不会复用错误的验证字段缓存。
func TestValidateJsonRequiredFieldsAnonymousStructCacheIsolation(t *testing.T) {
	validateFieldsCache = sync.Map{}
	fieldCache = sync.Map{}
	t.Cleanup(func() {
		validateFieldsCache = sync.Map{}
		fieldCache = sync.Map{}
	})

	var first struct {
		Name string `json:"name"`
	}
	if err := ValidateJsonRequiredFieldsWithRaw(reflect.ValueOf(&first), []byte(`{"name":"alice"}`)); err != nil {
		t.Fatalf("first anonymous struct validation failed: %v", err)
	}

	var second struct {
		ID      int  `json:"id"`
		Enabled bool `json:"enabled"`
	}
	if err := ValidateJsonRequiredFieldsWithRaw(reflect.ValueOf(&second), []byte(`{"id":1,"enabled":false}`)); err != nil {
		t.Fatalf("second anonymous struct should use its own cached fields, got: %v", err)
	}
}

// TestDecodeMultipartFormRequiredAllowsExplicitZeroValues 验证 multipart/form 显式零值不应被当作缺失字段。
func TestDecodeMultipartFormRequiredAllowsExplicitZeroValues(t *testing.T) {
	type payload struct {
		Count   int     `name:"count"`
		Enabled bool    `name:"enabled"`
		Ratio   float64 `name:"ratio"`
	}

	var got payload
	form := &multipart.Form{
		Value: map[string][]string{
			"count":   {"0"},
			"enabled": {"false"},
			"ratio":   {"0"},
		},
	}

	if err := decodeMultipartForm(form, &got); err != nil {
		t.Fatalf("explicit zero values should be accepted as present form fields, got error: %v", err)
	}
}

// TestDecodeMultipartFormPresenceSemantics 验证 multipart/form 按字段是否出现处理必填和默认值。
func TestDecodeMultipartFormPresenceSemantics(t *testing.T) {
	t.Run("explicit empty string is present", func(t *testing.T) {
		type payload struct {
			Name     string  `name:"name"`
			Optional string  `name:"optional,omitempty" default:"fallback"`
			Pointer  *string `name:"pointer"`
			Age      int     `name:"age,omitempty" default:"18"`
		}

		var got payload
		form := &multipart.Form{
			Value: map[string][]string{
				"name":     {""},
				"optional": {""},
				"pointer":  {""},
			},
		}

		if err := decodeMultipartForm(form, &got); err != nil {
			t.Fatalf("explicit empty form fields should be treated as present, got error: %v", err)
		}
		if got.Name != "" {
			t.Fatalf("expected empty name, got %q", got.Name)
		}
		if got.Optional != "" {
			t.Fatalf("expected explicit empty optional value to skip default, got %q", got.Optional)
		}
		if got.Pointer == nil || *got.Pointer != "" {
			t.Fatalf("expected pointer to explicit empty string, got %#v", got.Pointer)
		}
		if got.Age != 18 {
			t.Fatalf("expected missing age to use default 18, got %d", got.Age)
		}
	})

	t.Run("empty non-string value is conversion error", func(t *testing.T) {
		type payload struct {
			Age int `name:"age"`
		}

		var got payload
		form := &multipart.Form{
			Value: map[string][]string{
				"age": {""},
			},
		}

		err := decodeMultipartForm(form, &got)
		if err == nil {
			t.Fatal("expected empty integer form field to fail conversion")
		}
		if !strings.Contains(err.Error(), "invalid parameter 'age'") {
			t.Fatalf("expected invalid parameter error, got: %v", err)
		}
	})

	t.Run("missing required slice is missing", func(t *testing.T) {
		type payload struct {
			Tags []string `name:"tags"`
		}

		var got payload
		err := decodeMultipartForm(&multipart.Form{Value: map[string][]string{}}, &got)
		if err == nil {
			t.Fatal("expected missing required form slice to fail")
		}
		if !strings.Contains(err.Error(), "missing required parameter 'tags' in form") {
			t.Fatalf("expected missing required slice error, got: %v", err)
		}
	})
}

// TestSetFieldValueRejectsNarrowIntegerOverflow 验证窄数值类型溢出应返回错误而不是 panic 或截断。
func TestSetFieldValueRejectsNarrowIntegerOverflow(t *testing.T) {
	tests := []struct {
		name  string
		value string
		field reflect.Value
	}{
		{
			name:  "int8",
			value: "128",
			field: func() reflect.Value {
				var holder struct {
					N int8
				}
				return reflect.ValueOf(&holder).Elem().Field(0)
			}(),
		},
		{
			name:  "uint8",
			value: "256",
			field: func() reflect.Value {
				var holder struct {
					N uint8
				}
				return reflect.ValueOf(&holder).Elem().Field(0)
			}(),
		},
		{
			name:  "float32",
			value: "3.5e39",
			field: func() reflect.Value {
				var holder struct {
					N float32
				}
				return reflect.ValueOf(&holder).Elem().Field(0)
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("setFieldValue should return an overflow error instead of panicking: %v", recovered)
				}
			}()

			err := setFieldValue(tt.field, tt.value, "n", tt.field.Type())
			if err == nil {
				t.Fatalf("expected overflow error for %s, got nil", tt.field.Type())
			}
		})
	}
}

func TestContentDispositionEscapesFilename(t *testing.T) {
	header := contentDisposition(DispositionAttachment, `report "一".txt`)
	if !strings.Contains(header, `attachment`) {
		t.Fatalf("expected attachment disposition, got %q", header)
	}
	if !strings.Contains(header, `filename*=`) {
		t.Fatalf("expected RFC 5987 filename parameter, got %q", header)
	}
}

func TestRenderAPIEscapesAttachmentFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &TestAttachmentHandler{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/download", nil)

	renderAPI(handler, "test.attachment")(ctx)

	header := recorder.Header().Get("Content-Disposition")
	if !strings.Contains(header, `filename*=`) {
		t.Fatalf("expected escaped Content-Disposition filename, got %q", header)
	}
}

func TestBindParamsInitializesEmbeddedPointerFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("QueryAndBodyJSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test?q=foo", strings.NewReader(`{"value":"bar"}`))
		req.Header.Set("Content-Type", "application/json")

		bound := bindHandlerForTest(t, &TestEmbeddedPointerHandler{}, req, nil).(*TestEmbeddedPointerHandler)

		if bound.Body == nil || bound.Body.Value != "bar" {
			t.Fatalf("expected body value 'bar', got %+v", bound.Body)
		}

		levelOne := bound.TestLevelOne
		if levelOne == nil {
			t.Fatalf("embedded pointer chain was not initialized: %+v", bound)
		}
		levelTwo := levelOne.TestLevelTwo
		if levelTwo == nil || levelTwo.TestQueryLayer == nil {
			t.Fatalf("embedded pointer chain was not initialized: %+v", bound)
		}
		queryLayer := levelTwo.TestQueryLayer
		if queryLayer.Query != "foo" {
			t.Fatalf("expected query parameter to be 'foo', got %q", queryLayer.Query)
		}
	})

	t.Run("BodyJSONValueType", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/value-body?tag=tv", strings.NewReader(`{"name":"value","count":3}`))
		req.Header.Set("Content-Type", "application/json")

		bound := bindHandlerForTest(t, &TestBodyValueHandler{}, req, nil).(*TestBodyValueHandler)

		if bound.Tag != "tv" {
			t.Fatalf("expected query tag 'tv', got %q", bound.Tag)
		}

		if bound.Body.Name != "value" || bound.Body.Count != 3 {
			t.Fatalf("expected body value 'value',3 got %+v", bound.Body)
		}
	})

	t.Run("PathHeaderAndDefaults", func(t *testing.T) {
		handler := &TestPathHeaderHandler{}
		req := httptest.NewRequest(http.MethodGet, "/resources/placeholder", nil)

		bound := bindHandlerForTest(t, handler, req, func(ctx *gin.Context) {
			ctx.Params = gin.Params{{Key: "id", Value: "77"}}
		}).(*TestPathHeaderHandler)

		pathLayer := bound.TestPathLayer
		if pathLayer == nil || pathLayer.TestOptionalLayer == nil {
			t.Fatalf("expected embedded pointer chain to be initialized: %+v", bound)
		}
		optionalLayer := pathLayer.TestOptionalLayer

		if pathLayer.PathValue != 77 {
			t.Fatalf("expected path parameter 77, got %d", pathLayer.PathValue)
		}

		if optionalLayer.QueryWithDefault != "fallback" {
			t.Fatalf("expected default query value 'fallback', got %q", optionalLayer.QueryWithDefault)
		}

		if optionalLayer.HeaderWithDefault != "trace" {
			t.Fatalf("expected default header value 'trace', got %q", optionalLayer.HeaderWithDefault)
		}
	})

	t.Run("BodyJSONDefaults", func(t *testing.T) {
		prev := HandleBodyJsonOmitEmptyAndDefault()
		SetHandleBodyJsonOmitEmptyAndDefault(true)
		t.Cleanup(func() {
			SetHandleBodyJsonOmitEmptyAndDefault(prev)
		})

		req := httptest.NewRequest(http.MethodPost, "/json-default", strings.NewReader(`{"required":"req"}`))
		req.Header.Set("Content-Type", "application/json")

		bound := bindHandlerForTest(t, &TestBodyDefaultHandler{}, req, nil).(*TestBodyDefaultHandler)

		if bound.Body == nil || bound.Body.Optional != "opt" {
			t.Fatalf("expected optional field default 'opt', got %+v", bound.Body)
		}

		bodyEmbedding := bound.TestBodyDefaultEmbedding
		if bodyEmbedding == nil || bodyEmbedding.TestBodyDefaultHeader == nil {
			t.Fatalf("expected embedded header chain initialized: %+v", bound)
		}
		bodyHeader := bodyEmbedding.TestBodyDefaultHeader

		if bodyHeader.Trace != "trace" {
			t.Fatalf("expected default header trace value 'trace', got %q", bodyHeader.Trace)
		}
	})

	t.Run("QueryOmitemptyWithoutDefault", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/optional?req=ok", nil)

		bound := bindHandlerForTest(t, &TestOptionalQueryHandler{}, req, nil).(*TestOptionalQueryHandler)

		if bound.Required != "ok" {
			t.Fatalf("expected required query 'ok', got %q", bound.Required)
		}

		if bound.Optional != "" {
			t.Fatalf("expected optional query to remain empty, got %q", bound.Optional)
		}
	})

	t.Run("QueryAndHeaderPresenceWithEmptyValues", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/presence?query=&defaultQuery=&pointer=", nil)
		req.Header.Set("X-Required", "")
		req.Header.Set("X-Default", "")

		bound := bindHandlerForTest(t, &TestPresenceHandler{}, req, nil).(*TestPresenceHandler)

		if bound.Query != "" {
			t.Fatalf("expected explicit empty query value, got %q", bound.Query)
		}
		if bound.DefaultQuery != "" {
			t.Fatalf("expected explicit empty query to skip default, got %q", bound.DefaultQuery)
		}
		if bound.Header != "" {
			t.Fatalf("expected explicit empty header value, got %q", bound.Header)
		}
		if bound.DefaultHeader != "" {
			t.Fatalf("expected explicit empty header to skip default, got %q", bound.DefaultHeader)
		}
		if bound.Pointer == nil || *bound.Pointer != "" {
			t.Fatalf("expected pointer to explicit empty string, got %#v", bound.Pointer)
		}
	})

	t.Run("MissingValuesStillUseDefaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/presence?query=ok&pointer=value", nil)
		req.Header.Set("X-Required", "header")

		bound := bindHandlerForTest(t, &TestPresenceHandler{}, req, nil).(*TestPresenceHandler)

		if bound.DefaultQuery != "fallback" {
			t.Fatalf("expected missing query to use default, got %q", bound.DefaultQuery)
		}
		if bound.DefaultHeader != "trace" {
			t.Fatalf("expected missing header to use default, got %q", bound.DefaultHeader)
		}
	})

	t.Run("EmptyNonStringQueryFailsConversion", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/presence?age=", nil)

		err := bindHandlerErrorForTest(t, &TestEmptyIntQueryHandler{}, req, nil)
		if err == nil {
			t.Fatal("expected empty integer query to fail conversion")
		}
		if !strings.Contains(err.Error(), "invalid parameter 'age'") {
			t.Fatalf("expected invalid parameter error, got: %v", err)
		}
	})

	t.Run("MultipartBodyAndQuery", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		if err := writer.WriteField("field", "upload"); err != nil {
			t.Fatalf("write multipart field failed: %v", err)
		}
		contentType := writer.FormDataContentType()
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart writer failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/upload?token=t1", body)
		req.Header.Set("Content-Type", contentType)

		bound := bindHandlerForTest(t, &TestMultipartHandler{}, req, nil).(*TestMultipartHandler)

		if bound.Body == nil || bound.Body.Field != "upload" {
			t.Fatalf("expected multipart body to be parsed, got %+v", bound.Body)
		}

		multipartLevel := bound.TestMultipartLevelOne
		if multipartLevel == nil || multipartLevel.TestMultipartQuery == nil {
			t.Fatalf("expected multipart handler embedded pointers initialized: %+v", bound)
		}
		multipartQuery := multipartLevel.TestMultipartQuery

		if multipartQuery.Token != "t1" {
			t.Fatalf("expected token 't1', got %q", multipartQuery.Token)
		}
	})

	t.Run("MultipartFileFields", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		singleWriter, err := writer.CreateFormFile("file", "single.txt")
		if err != nil {
			t.Fatalf("create single file field failed: %v", err)
		}
		if _, err := singleWriter.Write([]byte("single")); err != nil {
			t.Fatalf("write single file failed: %v", err)
		}

		for _, name := range []string{"multi-1.txt", "multi-2.txt"} {
			multiWriter, err := writer.CreateFormFile("files", name)
			if err != nil {
				t.Fatalf("create multi file field failed: %v", err)
			}
			if _, err := multiWriter.Write([]byte(name)); err != nil {
				t.Fatalf("write multi file content failed: %v", err)
			}
		}

		contentType := writer.FormDataContentType()
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart writer failed: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/upload-files", body)
		req.Header.Set("Content-Type", contentType)

		bound := bindHandlerForTest(t, &TestMultipartFilesHandler{}, req, nil).(*TestMultipartFilesHandler)

		if bound.Body == nil || bound.Body.Single == nil {
			t.Fatalf("expected single file to be bound, got %+v", bound.Body)
		}
		if bound.Body.Single.Filename != "single.txt" {
			t.Fatalf("expected single file name 'single.txt', got %q", bound.Body.Single.Filename)
		}

		if len(bound.Body.Multi) != 2 {
			t.Fatalf("expected two multi files, got %d", len(bound.Body.Multi))
		}
		if bound.Body.Multi[0].Filename != "multi-1.txt" || bound.Body.Multi[1].Filename != "multi-2.txt" {
			t.Fatalf("unexpected multi filenames: %q, %q", bound.Body.Multi[0].Filename, bound.Body.Multi[1].Filename)
		}
	})
}

// TestBindParamsAcceptsValueHandler 验证公开接口允许的结构体值可以正常绑定。
func TestBindParamsAcceptsValueHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/value?name=easygin", nil)
	bound := bindHandlerForTest(t, TestValueHandler{}, req, nil)

	handler, ok := bound.(*TestValueHandler)
	if !ok {
		t.Fatalf("bound handler type = %T, want *TestValueHandler", bound)
	}
	if handler.Name != "easygin" {
		t.Fatalf("bound name = %q, want easygin", handler.Name)
	}
}

// TestBindParamsRejectsTypedNilHandler 验证 typed nil 在反射绑定前返回明确错误。
func TestBindParamsRejectsTypedNilHandler(t *testing.T) {
	var handler *TestValueHandler
	req := httptest.NewRequest(http.MethodGet, "/value?name=easygin", nil)
	err := bindHandlerErrorForTest(t, handler, req, nil)
	if err == nil || !strings.Contains(err.Error(), "is nil") {
		t.Fatalf("typed nil error = %v, want nil handler error", err)
	}
}

// TestBindParamsRejectsAnonymousHandler 验证匿名结构体不会污染基于命名类型的字段缓存。
func TestBindParamsRejectsAnonymousHandler(t *testing.T) {
	handler := struct{ TestValueHandler }{}
	req := httptest.NewRequest(http.MethodGet, "/value?name=easygin", nil)
	err := bindHandlerErrorForTest(t, handler, req, nil)
	if err == nil || !strings.Contains(err.Error(), "named struct") {
		t.Fatalf("anonymous handler error = %v, want named struct error", err)
	}
}

// TestMultipartFormCleanupRemovesTemporaryFiles 验证请求结束后清理由上传触发的磁盘临时文件。
func TestMultipartFormCleanupRemovesTemporaryFiles(t *testing.T) {
	previousLimit := GetMultipartMemoryLimit()
	SetMultipartMemoryLimit(1)
	t.Cleanup(func() {
		SetMultipartMemoryLimit(previousLimit)
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "large.bin")
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 64*1024)); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	var temporaryPath string
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", contentType)
	req = req.WithContext(context.WithValue(req.Context(), testMultipartPathKey{}, &temporaryPath))

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(multipartFormCleanup())
	engine.POST("/upload", renderAPI(&TestMultipartCleanupHandler{}, "test.multipartCleanup"))

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if temporaryPath == "" {
		t.Fatal("multipart file was not stored on disk")
	}
	if _, err := os.Stat(temporaryPath); !os.IsNotExist(err) {
		t.Fatalf("temporary file still exists at %s: %v", temporaryPath, err)
	}
}

func bindHandlerForTest(t *testing.T, handler RouterHandler, req *http.Request, customize func(*gin.Context)) RouterHandler {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req

	if customize != nil {
		customize(ctx)
	}

	newHandler, err := bindParams(ctx, handler)
	if err != nil {
		t.Fatalf("bindParams returned error: %v", err)
	}
	return newHandler
}

func bindHandlerErrorForTest(t *testing.T, handler RouterHandler, req *http.Request, customize func(*gin.Context)) error {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = req

	if customize != nil {
		customize(ctx)
	}

	_, err := bindParams(ctx, handler)
	return err
}

type TestEmbeddedPointerHandler struct {
	*TestLevelOne
	Body *TestBodyPayload `in:"body"`
}

type TestBodyPayload struct {
	Value string `json:"value"`
}

type TestBodyValueHandler struct {
	Tag  string               `in:"query" name:"tag"`
	Body TestBodyValuePayload `in:"body"`
}

type TestBodyValuePayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type TestLevelOne struct {
	*TestLevelTwo
}

type TestLevelTwo struct {
	*TestQueryLayer
}

type TestQueryLayer struct {
	Query string `in:"query" name:"q"`
}

type TestBodyDefaultHandler struct {
	*TestBodyDefaultEmbedding
	Body *TestBodyDefaultPayload `in:"body"`
}

type TestBodyDefaultEmbedding struct {
	*TestBodyDefaultHeader
}

type TestBodyDefaultHeader struct {
	Trace string `in:"header" name:"X-Body-Trace,omitempty" default:"trace"`
}

type TestBodyDefaultPayload struct {
	Required string `json:"required"`
	Optional string `json:"optional,omitempty" default:"opt"`
}

type TestOptionalQueryHandler struct {
	Required string `in:"query" name:"req"`
	Optional string `in:"query" name:"optional,omitempty"`
}

type TestPresenceHandler struct {
	Query         string  `in:"query" name:"query"`
	DefaultQuery  string  `in:"query" name:"defaultQuery,omitempty" default:"fallback"`
	Pointer       *string `in:"query" name:"pointer"`
	Header        string  `in:"header" name:"X-Required"`
	DefaultHeader string  `in:"header" name:"X-Default,omitempty" default:"trace"`
}

type TestEmptyIntQueryHandler struct {
	Age int `in:"query" name:"age"`
}

// TestValueHandler 验证结构体值处理器的反射绑定。
type TestValueHandler struct {
	Name string `in:"query" name:"name"`
}

// testMultipartPathKey 用于从处理器向测试传递临时文件路径。
type testMultipartPathKey struct{}

// TestMultipartCleanupHandler 验证 Multipart 文件的请求级清理。
type TestMultipartCleanupHandler struct {
	Body *TestMultipartCleanupBody `in:"body" mime:"multipart"`
}

// TestMultipartCleanupBody 定义清理测试的上传字段。
type TestMultipartCleanupBody struct {
	File *multipart.FileHeader `name:"file"`
}

type TestPathHeaderHandler struct {
	*TestPathLayer
}

type TestPathLayer struct {
	*TestOptionalLayer
	PathValue int `in:"path" name:"id"`
}

type TestOptionalLayer struct {
	QueryWithDefault  string `in:"query" name:"opt,omitempty" default:"fallback"`
	HeaderWithDefault string `in:"header" name:"X-Trace,omitempty" default:"trace"`
}

type TestMultipartHandler struct {
	*TestMultipartLevelOne
	Body *TestMultipartBody `in:"body" mime:"multipart"`
}

type TestMultipartLevelOne struct {
	*TestMultipartQuery
}

type TestMultipartQuery struct {
	Token string `in:"query" name:"token"`
}

type TestMultipartBody struct {
	Field string `name:"field"`
}

type TestMultipartFilesHandler struct {
	Body *TestMultipartFilesBody `in:"body" mime:"multipart"`
}

type TestAttachmentHandler struct{}

type TestMultipartFilesBody struct {
	Single *multipart.FileHeader   `name:"file"`
	Multi  []*multipart.FileHeader `name:"files"`
}

func (h *TestEmbeddedPointerHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestBodyDefaultHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestPathHeaderHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestMultipartHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestBodyValueHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestOptionalQueryHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestPresenceHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestEmptyIntQueryHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (TestValueHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestMultipartCleanupHandler) Output(ctx context.Context) (any, error) {
	file, err := h.Body.File.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	osFile, ok := file.(*os.File)
	if !ok {
		return nil, fmt.Errorf("multipart file type = %T, want *os.File", file)
	}
	path, ok := ctx.Value(testMultipartPathKey{}).(*string)
	if !ok {
		return nil, fmt.Errorf("temporary path capture missing")
	}
	*path = osFile.Name()
	return "ok", nil
}

func (h *TestMultipartFilesHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}

func (h *TestAttachmentHandler) Output(ctx context.Context) (any, error) {
	return &AttachmentFromReader{
		Disposition:   DispositionAttachment,
		ContentType:   "text/plain",
		Filename:      `report "一".txt`,
		Reader:        strings.NewReader("ok"),
		ContentLength: int64(len("ok")),
	}, nil
}
