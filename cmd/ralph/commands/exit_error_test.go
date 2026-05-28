package commands

import (
	"errors"
	"testing"
)

func TestExitError_Error(t *testing.T) {
	e := &ExitError{Code: 1}
	want := "exit 1"
	if got := e.Error(); got != want {
		t.Errorf("ExitError.Error() = %q, want %q", got, want)
	}
}

func TestExitError_Code2(t *testing.T) {
	e := &ExitError{Code: 2}
	want := "exit 2"
	if got := e.Error(); got != want {
		t.Errorf("ExitError.Error() = %q, want %q", got, want)
	}
}

func TestExitError_IsError(t *testing.T) {
	var err error = &ExitError{Code: 1}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Error("expected errors.As to match ExitError")
	}
	if exitErr.Code != 1 {
		t.Errorf("expected code 1, got %d", exitErr.Code)
	}
}
