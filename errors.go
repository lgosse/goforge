package goforge

import (
	"fmt"
	"runtime"
	"strings"
)

// NewError creates a new goforge error with a default 500 status code and the
// given cause.
func NewError(cause error) *Error {
	return &Error{
		HTTPStatus: 500,
		Code:       "ERR_INTERNAL_SERVER_ERROR",
		Message:    cause.Error(),
		Cause:      cause,
		Stack:      CaptureStackTrace(1),
	}
}

// StackFrame is a single call frame captured when a goforge error is created.
type StackFrame struct {
	Function       string
	File           string
	Line           int
	ProgramCounter uintptr
}

// Error is the standard contract for all goforge errors.
type Error struct {
	HTTPStatus int          // E.g., 400, 404, 500
	Code       string       // Internal tracking code, e.g., "ERR_USER_FORBIDDEN_PASSWORD_CHANGE", can be used for user-facing translations.
	Message    string       // Safe, user-facing message
	Cause      error        // The underlying wrapped error for logs (should not be exposed to clients)
	Stack      []StackFrame // Captured call stack for logs and error reporters such as Sentry.
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

// WithStack replaces the captured stack frames.
func (e *Error) WithStack(stack []StackFrame) *Error {
	e.Stack = append([]StackFrame(nil), stack...)
	return e
}

// WithCapturedStack captures a fresh stack trace. The skip value excludes
// additional caller frames above WithCapturedStack.
func (e *Error) WithCapturedStack(skip int) *Error {
	e.Stack = CaptureStackTrace(skip + 1)
	return e
}

// StackTrace returns a defensive copy of the captured stack frames.
func (e *Error) StackTrace() []StackFrame {
	return append([]StackFrame(nil), e.Stack...)
}

// StackTraceString formats the captured stack in the same file:line/function
// shape most log sinks and error reporters expect.
func (e *Error) StackTraceString() string {
	return FormatStackTrace(e.Stack)
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

// CaptureStackTrace captures the current goroutine stack. The skip value
// excludes additional caller frames above CaptureStackTrace.
func CaptureStackTrace(skip int) []StackFrame {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(skip+2, pcs)
	if n == 0 {
		return nil
	}

	frames := runtime.CallersFrames(pcs[:n])
	stack := make([]StackFrame, 0, n)
	for {
		frame, more := frames.Next()
		stack = append(stack, StackFrame{
			Function:       frame.Function,
			File:           frame.File,
			Line:           frame.Line,
			ProgramCounter: frame.PC,
		})
		if !more {
			break
		}
	}

	return stack
}

// FormatStackTrace formats captured stack frames into a multi-line string.
func FormatStackTrace(stack []StackFrame) string {
	if len(stack) == 0 {
		return ""
	}

	var builder strings.Builder
	for idx, frame := range stack {
		if idx > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(frame.File)
		builder.WriteByte(':')
		fmt.Fprint(&builder, frame.Line)
		builder.WriteByte(' ')
		builder.WriteString(frame.Function)
	}

	return builder.String()
}
