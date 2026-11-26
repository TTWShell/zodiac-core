package humatransport

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/TTWShell/zodiac-core/requestctx"
	"github.com/TTWShell/zodiac-core/response"
)

// NewError creates a Zodiac error for Huma. Install it as huma.NewError at
// process startup, before creating APIs or serving requests. Messages are public;
// details become data.errors for client errors and an internal cause for 5xx.
func NewError(status int, message string, errs ...error) huma.StatusError {
	err := response.NewError(status, message)
	if status >= http.StatusInternalServerError {
		err.Cause = errors.Join(errs...)
		return err
	}
	var details []*huma.ErrorDetail
	for _, cause := range errs {
		if cause == nil {
			continue
		}
		var detailer huma.ErrorDetailer
		if errors.As(cause, &detailer) {
			details = append(details, detailer.ErrorDetail())
		} else {
			details = append(details, &huma.ErrorDetail{Message: cause.Error()})
		}
	}
	if len(details) > 0 {
		err.Data = struct {
			Errors []*huma.ErrorDetail `json:"errors"`
		}{details}
	}
	return err
}

// NewErrorWithContext also logs internal causes with request metadata. Install
// it as huma.NewErrorWithContext alongside NewError for framework errors.
func NewErrorWithContext(ctx huma.Context, status int, message string, errs ...error) huma.StatusError {
	if status >= http.StatusInternalServerError {
		if cause := errors.Join(errs...); cause != nil {
			requestctx.Logger(ctx.Context()).ErrorContext(ctx.Context(), "Huma error", "error", cause)
		}
	}
	return NewError(status, message, errs...)
}

func normalizeError(ctx context.Context, err error) error {
	var public huma.StatusError
	var result *response.Error
	if !errors.As(err, &public) {
		requestctx.Logger(ctx).ErrorContext(ctx, "Unhandled error", "error", err)
		result = response.NewError(http.StatusInternalServerError, "")
	} else {
		switch public := public.(type) {
		case *response.Error:
			result = public
		case *huma.ErrorModel:
			details := make([]error, 0, len(public.Errors))
			for _, detail := range public.Errors {
				if detail != nil {
					details = append(details, detail)
				}
			}
			message := public.Detail
			if message == "" {
				message = public.Title
			}
			result = NewError(public.Status, message, details...).(*response.Error)
		default:
			result = response.NewError(public.GetStatus(), public.Error())
		}
	}
	if result.Cause != nil {
		requestctx.Logger(ctx).ErrorContext(ctx, "Unhandled error", "error", result.Cause)
	}
	if result == public {
		// Keep existing Zodiac errors and their wrappers intact.
		return err
	}
	var headers huma.HeadersError
	if errors.As(err, &headers) {
		return huma.ErrorWithHeaders(result, headers.GetHeaders())
	}
	return result
}
