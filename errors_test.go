package goforge

import (
	"errors"
	"strings"
	"testing"
)

func TestNewErrorCapturesStackTrace(t *testing.T) {
	err := newErrorFromHelper()

	if len(err.StackTrace()) == 0 {
		t.Fatal("expected stack trace to be captured")
	}
	if err.StackTrace()[0].ProgramCounter == 0 {
		t.Fatal("expected stack trace frame to include a program counter")
	}

	formatted := err.StackTraceString()
	if !strings.Contains(formatted, "newErrorFromHelper") {
		t.Fatalf("expected formatted stack trace to include helper frame, got %q", formatted)
	}
}

func TestErrorWithStackUsesDefensiveCopies(t *testing.T) {
	stack := []StackFrame{{
		Function:       "example",
		File:           "example.go",
		Line:           12,
		ProgramCounter: 123,
	}}

	err := NewError(errors.New("boom")).WithStack(stack)
	stack[0].Function = "mutated"

	got := err.StackTrace()
	if got[0].Function != "example" {
		t.Fatalf("expected stored stack to be immutable from input, got %q", got[0].Function)
	}

	got[0].Function = "mutated again"
	if err.StackTrace()[0].Function != "example" {
		t.Fatal("expected StackTrace to return a defensive copy")
	}
}

func TestFormatStackTrace(t *testing.T) {
	formatted := FormatStackTrace([]StackFrame{{
		Function: "example.Func",
		File:     "/tmp/example.go",
		Line:     42,
	}})

	if formatted != "/tmp/example.go:42 example.Func" {
		t.Fatalf("unexpected stack trace format: %q", formatted)
	}
}

func newErrorFromHelper() *Error {
	return NewError(errors.New("boom"))
}
