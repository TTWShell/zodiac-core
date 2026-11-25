// Package response defines Zodiac's framework-independent API responses.
package response

import "net/http"

// Response is the standard API envelope. Data is never omitted, including null.
type Response[T any] struct {
	Code    int    `json:"code"`
	Data    T      `json:"data"`
	Message string `json:"message"`
}

// OK creates a successful response with business code zero.
func OK[T any](data T) Response[T] {
	return Response[T]{Data: data, Message: "Success"}
}

// Created follows Python's response_created default business code of 201.
func Created[T any](data T) Response[T] {
	return Response[T]{Code: http.StatusCreated, Data: data, Message: "Created"}
}

// Error carries a public response and a separate HTTP status and internal cause.
// Set Code and Data to supply application-specific error details.
type Error struct {
	Response[any]
	Status int   `json:"-"`
	Cause  error `json:"-"`
}

// NewError defaults the business code and empty message from the HTTP status.
func NewError(status int, message string) *Error {
	if message == "" {
		message = http.StatusText(status)
	}
	return &Error{Status: status, Response: Response[any]{Code: status, Message: message}}
}

func (e *Error) Error() string { return e.Message }
func (e *Error) Unwrap() error { return e.Cause }

// GetStatus returns the HTTP status without exposing it in the response body.
func (e *Error) GetStatus() int { return e.Status }
