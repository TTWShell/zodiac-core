package humatransport_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

func newAPI() (*chi.Mux, huma.API) {
	router := chi.NewRouter()
	router.Use(httptransport.Middleware(httptransport.Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}))
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

func TestResponsesAndOpenAPI(t *testing.T) {
	router, api := newAPI()
	type input struct {
		ID int `path:"id" minimum:"1"`
	}
	op := huma.Operation{OperationID: "get-item", Method: "GET", Path: "/items/{id}", Errors: []int{404, 409, 422, 500}}
	humatransport.Register(api, op, func(ctx context.Context, in *input) (*payload, error) {
		switch in.ID {
		case 2:
			err := response.NewError(409, "Already exists")
			err.Code, err.Data = 1001, "duplicate"
			return nil, fmt.Errorf("service: %w", err)
		case 3:
			return nil, errors.New("private database credentials")
		case 4:
			panic("private panic details")
		case 5:
			return nil, huma.ErrorWithHeaders(huma.Error404NotFound("account missing"), http.Header{"Retry-After": {"10"}})
		case 6:
			return nil, huma.Error503ServiceUnavailable("maintenance")
		case 7:
			return nil, nil
		}
		return &payload{Name: "test"}, nil
	})
	for _, tc := range []struct {
		path         string
		status, code int
		message      string
		native       bool
	}{
		{"/items/1", 200, 0, "Success", false},
		{"/items/2", 409, 1001, "Already exists", false},
		{"/items/0", 422, 0, "validation failed", true},
		{"/items/3", 500, 500, "Internal Server Error", false},
		{"/items/4", 500, 500, "Internal Server Error", false},
		{"/items/5", 404, 0, "account missing", true},
		{"/items/6", 503, 0, "maintenance", true},
		{"/items/7", 200, 0, "Success", false},
		{"/missing", 404, 404, "Not Found", false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := request(router, "GET", tc.path)
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || w.Header().Get("X-Request-ID") == "" || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("response: %d %v %s", w.Code, w.Header(), w.Body)
			}
			if tc.native {
				if body["detail"] != tc.message || body["status"] != float64(tc.status) || w.Header().Get("Content-Type") != "application/problem+json" {
					t.Fatalf("native error changed: %s", w.Body)
				}
			} else if len(body) != 3 || body["code"] != float64(tc.code) || body["message"] != tc.message || w.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("invalid envelope: %s", w.Body)
			}
			switch tc.path {
			case "/items/1":
				if !reflect.DeepEqual(body["data"], map[string]any{"name": "test"}) {
					t.Fatal(body)
				}
			case "/items/2":
				if body["data"] != "duplicate" {
					t.Fatal(body)
				}
			case "/items/0":
				details, ok := body["errors"].([]any)
				if !ok || len(details) == 0 || details[0].(map[string]any)["location"] != "path.id" {
					t.Fatal(body)
				}
			case "/items/5":
				if w.Header().Get("Retry-After") != "10" {
					t.Fatal("lost error header")
				}
			case "/items/3", "/items/4", "/items/7":
				if body["data"] != nil {
					t.Fatal(body)
				}
			}
		})
	}
	if w := request(router, "POST", "/items/1"); w.Code != 405 {
		t.Fatal(w.Code)
	}
	registry := api.OpenAPI().Components.Schemas
	operation := api.OpenAPI().Paths["/items/{id}"].Get
	schema := operation.Responses["200"].Content["application/json"].Schema
	if len(schema.Properties) != 3 || schema.Properties["data"] == nil {
		t.Fatal("invalid envelope schema")
	}
	data := schema.Properties["data"].AnyOf
	if registry.SchemaFromRef(data[0].Ref).Properties["name"].Type != "string" || data[1].Type != "null" {
		t.Fatal("lost nullable payload schema")
	}
	for _, code := range []string{"404", "409", "422", "500", "default"} {
		content := operation.Responses[code].Content
		if content["application/json"] == nil || content["application/problem+json"] == nil {
			t.Fatal("missing error representation", code)
		}
		if len(content["application/json"].Schema.Properties) != 3 || registry.SchemaFromRef(content["application/problem+json"].Schema.Ref).Properties["detail"] == nil {
			t.Fatal("invalid error schema")
		}
	}
	if len(op.Errors) != 4 || op.Responses != nil {
		t.Fatal("caller operation mutated")
	}
	for _, path := range []string{"/openapi.json", "/docs"} {
		if w := request(router, "GET", path); w.Code != 200 || strings.Contains(w.Body.String(), `"message":"Success"`) {
			t.Fatal("documentation wrapped", path)
		}
	}
	if _, ok := huma.NewError(422, "validation").(*huma.ErrorModel); !ok {
		t.Fatal("global error factory changed")
	}
}

func TestPayloadTypesAndExplicitOutputs(t *testing.T) {
	router, api := newAPI()
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
