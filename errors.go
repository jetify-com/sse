package sse

import (
	"fmt"
)

// ErrValidation represents errors related to event validation or encoding
// that don't necessarily require terminating the event stream.
var ErrValidation error = &validationError{}

type validationError struct {
	Message string
	Cause   error // Wrapped underlying error
}

// Error implements the error interface
func (e *validationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("sse: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("sse: %s", e.Message)
}

// Unwrap returns the underlying cause if present
func (e *validationError) Unwrap() error {
	return e.Cause
}

// Is implements error matching and returns true for any validationError
func (e *validationError) Is(target error) bool { return target == ErrValidation }
