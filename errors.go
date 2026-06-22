package goforge

import "fmt"

// @TODO: add stacktrace handling to it for easy sentry forwarding.

// NewError creates a new goforge error with a default 500 status code and the
// given cause.
func NewError(cause error) *Error {
	return &Error{
		HTTPStatus: 500,
		Code:       "ERR_INTERNAL_SERVER_ERROR",
		Message:    cause.Error(),
		Cause:      cause,
	}
}

// Error is the standard contract for all goforge errors.
type Error struct {
	HTTPStatus int    // E.g., 400, 404, 500
	Code       string // Internal tracking code, e.g., "ERR_USER_FORBIDDEN_PASSWORD_CHANGE", can be used for user-facing translations.
	Message    string // Safe, user-facing message
	Cause      error  // The underlying wrapped error for logs (should not be exposed to clients)
}

// WithHTTPStatus sets the HTTP status code for the error.
func (e *Error) WithHTTPStatus(status int) *Error {
	e.HTTPStatus = status
	return e
}

// WithCode sets the internal tracking code for the error.
func (e *Error) WithCode(code string) *Error {
	e.Code = code
	return e
}

// WithMessage sets the user-facing message for the error.
func (e *Error) WithMessage(message string) *Error {
	e.Message = message
	return e
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap allows standard errors.Is and errors.As to work perfectly.
func (e *Error) Unwrap() error {
	return e.Cause
}