// Package humatransport adapts business handlers and responses to Huma.
package humatransport

import (
	"context"
	"maps"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/TTWShell/zodiac-core/response"
)

// DefaultConfig creates an API-local configuration without response rewriting.
// Customize the returned configuration before constructing the API.
// Install NewError and NewErrorWithContext at startup for Zodiac framework errors.
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
// Both registration functions normalize public errors and hide unknown failures.
func RegisterOutput[I, O any](api huma.API, op huma.Operation, handler func(context.Context, *I) (*O, error)) {
	huma.Register(api, op, func(ctx context.Context, in *I) (*O, error) {
		out, err := handler(ctx, in)
		if err != nil {
			return nil, normalizeError(ctx, err)
		}
		return out, err
	})
}
