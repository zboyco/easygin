package easygin

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindParamsInitializesEmbeddedPointerFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("QueryAndBodyJSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test?q=foo", strings.NewReader(`{"value":"bar"}`))
		req.Header.Set("Content-Type", "application/json")

		bound := bindHandlerForTest(t, &TestEmbeddedPointerHandler{}, req, nil).(*TestEmbeddedPointerHandler)

		if bound.Body == nil || bound.Body.Value != "bar" {
			t.Fatalf("expected body value 'bar', got %+v", bound.Body)
		}

		if bound.TestLevelOne == nil ||
			bound.TestLevelOne.TestLevelTwo == nil ||
			bound.TestLevelOne.TestLevelTwo.TestQueryLayer == nil {
			t.Fatalf("embedded pointer chain was not initialized: %+v", bound)
		}

		if bound.TestLevelOne.TestLevelTwo.TestQueryLayer.Query != "foo" {
			t.Fatalf("expected query parameter to be 'foo', got %q", bound.TestLevelOne.TestLevelTwo.TestQueryLayer.Query)
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

		if bound.TestPathLayer == nil ||
			bound.TestPathLayer.TestOptionalLayer == nil {
			t.Fatalf("expected embedded pointer chain to be initialized: %+v", bound)
		}

		if bound.TestPathLayer.PathValue != 77 {
			t.Fatalf("expected path parameter 77, got %d", bound.TestPathLayer.PathValue)
		}

		if bound.TestPathLayer.TestOptionalLayer.QueryWithDefault != "fallback" {
			t.Fatalf("expected default query value 'fallback', got %q", bound.TestPathLayer.TestOptionalLayer.QueryWithDefault)
		}

		if bound.TestPathLayer.TestOptionalLayer.HeaderWithDefault != "trace" {
			t.Fatalf("expected default header value 'trace', got %q", bound.TestPathLayer.TestOptionalLayer.HeaderWithDefault)
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

		if bound.TestBodyDefaultEmbedding == nil ||
			bound.TestBodyDefaultEmbedding.TestBodyDefaultHeader == nil {
			t.Fatalf("expected embedded header chain initialized: %+v", bound)
		}

		if bound.TestBodyDefaultEmbedding.TestBodyDefaultHeader.Trace != "trace" {
			t.Fatalf("expected default header trace value 'trace', got %q", bound.TestBodyDefaultEmbedding.TestBodyDefaultHeader.Trace)
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

		if bound.TestMultipartLevelOne == nil ||
			bound.TestMultipartLevelOne.TestMultipartQuery == nil {
			t.Fatalf("expected multipart handler embedded pointers initialized: %+v", bound)
		}

		if bound.TestMultipartLevelOne.TestMultipartQuery.Token != "t1" {
			t.Fatalf("expected token 't1', got %q", bound.TestMultipartLevelOne.TestMultipartQuery.Token)
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

func (h *TestMultipartFilesHandler) Output(ctx context.Context) (any, error) {
	return nil, nil
}
