// Package humatransport adapts business handlers and responses to Huma.
package humatransport

import (
	"context"
	"errors"
	"maps"
	"net/http"
	"reflect"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/TTWShell/zodiac-core/requestctx"
	"github.com/TTWShell/zodiac-core/response"
)

// DefaultConfig creates an API-local configuration without response rewriting.
// Customize the returned configuration before constructing the API.
func DefaultConfig(title, version string) huma.Config {
	cfg := huma.DefaultConfig(title, version)
	// Keep the Zodiac envelope free of Huma's additional $schema property.
	cfg.CreateHooks = nil
	cfg.Formats = maps.Clone(cfg.Formats)
	return cfg
}

// Body is the Zodiac envelope for an explicit Huma output's Body field.
// Use a value Body and a nullable T when the response has no data.
type Body[T any] response.Response[T]

// Schema describes the wire type, including nullable data. The envelope stays
// inline so Huma's generic naming cannot conflate Body[T] and Body[*T].
func (Body[T]) Schema(registry huma.Registry) *huma.Schema {
	schema := huma.SchemaFromType(registry, reflect.TypeFor[response.Response[T]]())
	schema.Properties["data"] = &huma.Schema{
		AnyOf: []*huma.Schema{schema.Properties["data"], {Type: "null"}},
	}
	return schema
}

type output[T any] struct {
	Body Body[T]
}

// Register registers a business handler whose return value is the response data.
// It constructs the envelope before Huma serializes it; nil data remains inside
// the envelope. DefaultStatus 201 uses Created defaults; other statuses use OK.
// Use RegisterOutput for explicit envelopes, headers, streams or no-body output.
func Register[I, T any](api huma.API, op huma.Operation, handler func(context.Context, *I) (T, error)) {
	if op.DefaultStatus != 0 && (op.DefaultStatus < 200 || op.DefaultStatus == 204 || op.DefaultStatus == 304) {
		panic("humatransport: use RegisterOutput for responses without a body")
	}
	RegisterOutput(api, op, func(ctx context.Context, in *I) (*output[T], error) {
		data, err := handler(ctx, in)
		if err != nil {
			return nil, err
		}
		body := response.OK(data)
		if op.DefaultStatus == http.StatusCreated {
			body = response.Created(data)
		}
		return &output[T]{Body: Body[T](body)}, nil
	})
}

// RegisterOutput preserves Huma's native output contract, including headers,
// status, raw bytes, streams and no-body responses. It never wraps the Body.
// Both registration functions preserve public errors and hide unknown failures.
func RegisterOutput[I, O any](api huma.API, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	defineErrors(api, &op)
	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		out, err := handler(ctx, in)
		if err != nil {
			var public huma.StatusError
			if !errors.As(err, &public) {
				requestctx.Logger(ctx).ErrorContext(ctx, "Unhandled error", "error", err)
				return nil, response.NewError(http.StatusInternalServerError, "")
			}
		}
		return out, err
	})
}

// Declare both supported error representations from their Go types. Huma's
// Errors shorthand otherwise replaces declarations using its global factory.
// Explicit caller-supplied responses take precedence and are not mutated.
func defineErrors(api huma.API, op *huma.Operation) {
	op.Responses = maps.Clone(op.Responses)
	if op.Responses == nil {
		op.Responses = make(map[string]*huma.Response)
	}
	registry := api.OpenAPI().Components.Schemas
	add := func(status, description string) {
		if op.Responses[status] != nil {
			return
		}
		op.Responses[status] = &huma.Response{
			Description: description,
			Content: map[string]*huma.MediaType{
				"application/json":         {Schema: registry.Schema(reflect.TypeFor[Body[any]](), true, "")},
				"application/problem+json": {Schema: registry.Schema(reflect.TypeFor[huma.ErrorModel](), true, "")},
			},
		}
	}
	for _, status := range op.Errors {
		add(strconv.Itoa(status), http.StatusText(status))
	}
	add("default", "Error")
	op.Errors = nil
}
