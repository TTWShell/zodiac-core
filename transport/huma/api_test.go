package humatransport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/TTWShell/zodiac-core/response"
	httptransport "github.com/TTWShell/zodiac-core/transport/http"
	humatransport "github.com/TTWShell/zodiac-core/transport/huma"
)

func newAPI(t *testing.T, logOutput io.Writer) (*chi.Mux, huma.API) {
	t.Helper()
	original, originalWithContext := huma.NewError, huma.NewErrorWithContext
	t.Cleanup(func() {
		huma.NewError, huma.NewErrorWithContext = original, originalWithContext
	})
	huma.NewError = humatransport.NewError
	huma.NewErrorWithContext = humatransport.NewErrorWithContext
	router := chi.NewRouter()
	router.Use(httptransport.Middleware(httptransport.Options{Logger: slog.New(slog.NewTextHandler(logOutput, nil))}))
	router.NotFound(httptransport.ErrorHandler(http.StatusNotFound))
	router.MethodNotAllowed(httptransport.ErrorHandler(http.StatusMethodNotAllowed))
	return router, humachi.New(router, humatransport.DefaultConfig("Test", "1.0"))
}

func request(handler http.Handler, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(method, path, nil))
	return w
}

type payload struct {
	Name string `json:"name"`
}

// Inline payload schemas must work without relying on a registry reference.
type inlinePayload payload

func (inlinePayload) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: "object", Properties: map[string]*huma.Schema{"name": {Type: "string"}}, Required: []string{"name"}}
}

func errorData(details ...map[string]any) map[string]any {
	items := make([]any, len(details))
	for i := range details {
		items[i] = details[i]
	}
	return map[string]any{"errors": items}
}

func registerItems(t *testing.T, logs io.Writer) (*chi.Mux, huma.API, huma.Operation) {
	t.Helper()
	router, api := newAPI(t, logs)
	op := huma.Operation{OperationID: "get-item", Method: "GET", Path: "/items/{id}", Errors: []int{404, 409}}
	humatransport.Register(api, op, func(_ context.Context, in *struct {
		ID int `path:"id" minimum:"1"`
	}) (*payload, error) {
		switch in.ID {
		case 2:
			err := response.NewError(409, "Already exists")
			err.Code, err.Data = 1001, "duplicate"
			return nil, fmt.Errorf("service: %w", huma.ErrorWithHeaders(err, http.Header{"Retry-After": {"10"}}))
		case 3:
			return nil, huma.ErrorWithHeaders(errors.New("private database credentials"), http.Header{"Retry-After": {"10"}})
		case 4:
			panic("private panic details")
		case 5:
			return nil, huma.ErrorWithHeaders(huma.Error404NotFound("account missing"), http.Header{"Retry-After": {"10"}})
		case 6:
			return nil, huma.Error503ServiceUnavailable("maintenance")
		case 7:
			return nil, nil
		case 8:
			return nil, fmt.Errorf("service: %w", huma.ErrorWithHeaders(&huma.ErrorModel{
				Status: 400, Detail: "invalid account",
				Errors: []*huma.ErrorDetail{{Message: "invalid email", Location: "body.email", Value: "bad"}},
			}, http.Header{"Retry-After": {"10"}, "Set-Cookie": {"a=1", "b=2"}}))
		case 9:
			return nil, fmt.Errorf("service: %w", rateLimitError{})
		case 10:
			return nil, huma.Error500InternalServerError("operation failed", errors.New("private connection details"))
		case 11:
			return nil, huma.Error400BadRequest("bad request", errors.New("field missing"))
		case 12:
			return nil, &huma.ErrorModel{Status: http.StatusBadRequest, Title: "Bad Request"}
		}
		return &payload{Name: "test"}, nil
	})
	return router, api, op
}

func TestOperationResponses(t *testing.T) {
	var logs bytes.Buffer
	router, _, _ := registerItems(t, &logs)
	retryAfter := http.Header{"Retry-After": {"10"}}
	for _, tc := range []struct {
		path         string
		status, code int
		message      string
		data         any
		headers      http.Header
	}{
		{"/items/1", 200, 0, "Success", map[string]any{"name": "test"}, nil},
		{"/items/2", 409, 1001, "Already exists", "duplicate", retryAfter},
		{"/items/0", 422, 422, "validation failed", errorData(map[string]any{
			"message": "expected number >= 1", "location": "path.id", "value": float64(0),
		}), nil},
		{"/items/3", 500, 500, "Internal Server Error", nil, retryAfter},
		{"/items/4", 500, 500, "Internal Server Error", nil, nil},
		{"/items/5", 404, 404, "account missing", nil, retryAfter},
		{"/items/6", 503, 503, "maintenance", nil, nil},
		{"/items/7", 200, 0, "Success", nil, nil},
		{"/items/8", 400, 400, "invalid account", errorData(map[string]any{
			"message": "invalid email", "location": "body.email", "value": "bad",
		}), http.Header{"Retry-After": {"10"}, "Set-Cookie": {"a=1", "b=2"}}},
		{"/items/9", 429, 429, "slow down", nil, nil},
		{"/items/10", 500, 500, "operation failed", nil, nil},
		{"/items/11", 400, 400, "bad request", errorData(map[string]any{"message": "field missing"}), nil},
		{"/items/12", 400, 400, "Bad Request", nil, nil},
		{"/missing", 404, 404, "Not Found", nil, nil},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := request(router, "GET", tc.path)
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("X-Request-ID") == "" || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("response: %d %v %s", w.Code, w.Header(), w.Body)
			}
			if len(body) != 3 || body["code"] != float64(tc.code) || body["message"] != tc.message || !reflect.DeepEqual(body["data"], tc.data) {
				t.Fatalf("envelope: %s", w.Body)
			}
			for key, values := range tc.headers {
				if !reflect.DeepEqual(w.Header().Values(key), values) {
					t.Fatalf("header %s: %v", key, w.Header().Values(key))
				}
			}
		})
	}
	if w := request(router, "POST", "/items/1"); w.Code != 405 || strings.TrimSpace(w.Body.String()) != `{"code":405,"data":null,"message":"Method Not Allowed"}` {
		t.Fatal(w.Code, w.Body)
	}
	for _, secret := range []string{"private database credentials", "private panic details", "private connection details"} {
		if !strings.Contains(logs.String(), secret) {
			t.Fatalf("cause not logged: %s\n%s", secret, logs.String())
		}
	}
}

func TestOperationSchema(t *testing.T) {
	router, api, op := registerItems(t, io.Discard)
	registry := api.OpenAPI().Components.Schemas
	operation := api.OpenAPI().Paths["/items/{id}"].Get
	schema := operation.Responses["200"].Content["application/json"].Schema
	data := schema.Properties["data"].AnyOf
	if len(schema.Properties) != 3 || registry.SchemaFromRef(data[0].Ref).Properties["name"].Type != "string" || data[1].Type != "null" {
		t.Fatal("invalid success schema")
	}
	for _, code := range []string{"404", "409", "422", "500"} {
		content := operation.Responses[code].Content
		schema := content["application/json"].Schema
		if schema.Ref != "" {
			schema = registry.SchemaFromRef(schema.Ref)
		}
		if len(content) != 1 || schema == nil || len(schema.Properties) != 3 || schema.Properties["code"] == nil || schema.Properties["data"] == nil || schema.Properties["message"] == nil {
			t.Fatalf("invalid error schema %s", code)
		}
	}
	if len(op.Errors) != 2 || op.Responses != nil {
		t.Fatal("caller operation mutated")
	}
	for _, path := range []string{"/openapi.json", "/docs"} {
		if w := request(router, "GET", path); w.Code != 200 || strings.Contains(w.Body.String(), `"message":"Success"`) {
			t.Fatal("documentation wrapped", path)
		}
	}
}

func TestDefaultConfigDoesNotInstallErrorFactory(t *testing.T) {
	humatransport.DefaultConfig("Test", "1.0")
	if _, ok := huma.NewError(422, "validation").(*huma.ErrorModel); !ok {
		t.Fatal("global error factory changed")
	}
}

type rateLimitError struct{}

func (rateLimitError) Error() string  { return "slow down" }
func (rateLimitError) GetStatus() int { return http.StatusTooManyRequests }

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestFrameworkErrors(t *testing.T) {
	var logs bytes.Buffer
	router, api := newAPI(t, &logs)
	type input struct {
		Body struct {
			Name string `json:"name" minLength:"2"`
		}
	}
	humatransport.Register(api, huma.Operation{
		OperationID: "create-item", Method: "POST", Path: "/items", MaxBodyBytes: 64,
	}, func(_ context.Context, in *input) (payload, error) { return payload{Name: in.Body.Name}, nil })
	for _, tc := range []struct {
		name, contentType string
		body              io.Reader
		status            int
	}{
		{"required body", "application/json", nil, 400},
		{"malformed JSON", "application/json", strings.NewReader("{"), 400},
		{"validation", "application/json", strings.NewReader(`{"name":"a"}`), 422},
		{"unsupported media", "application/xml", strings.NewReader("<item/>"), 415},
		{"too large", "application/json", strings.NewReader(strings.Repeat("x", 65)), 413},
		{"read timeout", "application/json", failedReader{os.ErrDeadlineExceeded}, 408},
		{"read failure", "application/json", failedReader{errors.New("private read failure")}, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/items", tc.body)
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || w.Header().Get("Content-Type") != "application/json" || len(body) != 3 || body["code"] != float64(tc.status) || body["message"] == "" {
				t.Fatalf("unexpected response: %d %v %s", w.Code, w.Header(), w.Body)
			}
			if tc.status == 500 && (body["data"] != nil || strings.Contains(w.Body.String(), "private")) {
				t.Fatal("internal cause exposed:", w.Body)
			}
		})
	}
	if !strings.Contains(logs.String(), "private read failure") {
		t.Fatal("internal cause not logged")
	}
}

func TestPayloadTypesAndExplicitOutputs(t *testing.T) {
	router, api := newAPI(t, io.Discard)
	op := func(path string) huma.Operation {
		return huma.Operation{OperationID: path[1:], Method: "GET", Path: path}
	}
	created := op("/created")
	created.DefaultStatus = 201
	humatransport.Register(api, created, func(context.Context, *struct{}) (payload, error) { return payload{Name: "new"}, nil })
	humatransport.Register(api, op("/inline"), func(context.Context, *struct{}) (inlinePayload, error) { return inlinePayload{Name: "inline"}, nil })
	humatransport.Register(api, op("/list"), func(context.Context, *struct{}) ([]payload, error) { return []payload{{Name: "list"}}, nil })
	humatransport.Register(api, op("/scalar"), func(context.Context, *struct{}) (string, error) { return "hello", nil })
	// Register the pointer and value variants together to catch schema name collisions.
	humatransport.Register(api, op("/null"), func(context.Context, *struct{}) (*payload, error) { return nil, nil })
	type explicit struct {
		Tag  string `header:"ETag"`
		Body humatransport.Body[inlinePayload]
	}
	humatransport.RegisterOutput(api, op("/explicit"), func(context.Context, *struct{}) (*explicit, error) {
		return &explicit{Tag: "revision-1", Body: humatransport.Body[inlinePayload]{Code: 42, Data: inlinePayload{Name: "explicit"}, Message: "Custom"}}, nil
	})
	humatransport.RegisterOutput(api, op("/raw"), func(context.Context, *struct{}) (*struct {
		ContentType string `header:"Content-Type"`
		Body        []byte
	}, error) {
		return &struct {
			ContentType string `header:"Content-Type"`
			Body        []byte
		}{"text/plain", []byte("raw")}, nil
	})
	humatransport.RegisterOutput(api, op("/stream"), func(context.Context, *struct{}) (*struct{ Body func(huma.Context) }, error) {
		return &struct{ Body func(huma.Context) }{func(ctx huma.Context) {
			ctx.SetHeader("Content-Type", "text/event-stream")
			ctx.SetStatus(200)
			_, _ = io.WriteString(ctx.BodyWriter(), "data: test\n\n")
			if err := http.NewResponseController(ctx.BodyWriter().(http.ResponseWriter)).Flush(); err != nil {
				t.Error(err)
			}
		}}, nil
	})
	humatransport.RegisterOutput(api, op("/empty"), func(context.Context, *struct{}) (*struct{}, error) { return nil, nil })
	humatransport.RegisterOutput(api, op("/failure"), func(context.Context, *struct{}) (*explicit, error) { return nil, errors.New("private output failure") })
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{
		{"/created", 201, `{"code":201,"data":{"name":"new"},"message":"Created"}`},
		{"/inline", 200, `{"code":0,"data":{"name":"inline"},"message":"Success"}`},
		{"/list", 200, `{"code":0,"data":[{"name":"list"}],"message":"Success"}`},
		{"/scalar", 200, `{"code":0,"data":"hello","message":"Success"}`},
		{"/null", 200, `{"code":0,"data":null,"message":"Success"}`},
		{"/explicit", 200, `{"code":42,"data":{"name":"explicit"},"message":"Custom"}`},
		{"/raw", 200, "raw"}, {"/stream", 200, "data: test"}, {"/empty", 204, ""},
		{"/failure", 500, `{"code":500,"data":null,"message":"Internal Server Error"}`},
	} {
		w := request(router, "GET", tc.path)
		if w.Code != tc.status || strings.TrimSpace(w.Body.String()) != tc.body {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
		if tc.path == "/stream" && !w.Flushed {
			t.Fatal("stream did not flush")
		}
		if tc.path == "/explicit" && w.Header().Get("ETag") != "revision-1" {
			t.Fatal("lost header")
		}
	}
	for _, path := range []string{"/inline", "/explicit"} {
		s := api.OpenAPI().Paths[path].Get.Responses["200"].Content["application/json"].Schema
		if len(s.Properties) != 3 || s.Properties["data"].AnyOf[0].Properties["name"].Type != "string" {
			t.Fatal("inline schema wrapped twice", path)
		}
	}
	if api.OpenAPI().Paths["/created"].Get.Responses["201"] == nil {
		t.Fatal("missing 201 schema")
	}
	if len(api.OpenAPI().Paths["/empty"].Get.Responses["204"].Content) != 0 {
		t.Fatal("204 has body schema")
	}
}
