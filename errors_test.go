package sse

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncoder_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		event       *Event
		expectError bool
	}{
		// Valid cases
		{
			name: "valid event",
			event: &Event{
				ID:    "1",
				Event: "update",
				Data:  "Hello, world!",
			},
			expectError: false,
		},
		{
			name: "zero retry is valid",
			event: &Event{
				Retry: 0,
				Data:  "test",
			},
			expectError: false,
		},
		{
			name: "empty event type is valid",
			event: &Event{
				Event: "",
				Data:  "test",
			},
			expectError: false,
		},
		{
			name: "message event type is valid",
			event: &Event{
				Event: "message",
				Data:  "test",
			},
			expectError: false,
		},
		{
			name: "raw data with newlines and split enabled",
			event: &Event{
				Data:  Raw("line1\nline2"),
				Split: true,
			},
			expectError: false,
		},

		// Invalid cases
		{
			name:        "nil event",
			event:       nil,
			expectError: true,
		},
		{
			name: "negative retry value",
			event: &Event{
				Retry: -1000 * time.Millisecond,
				Data:  "test",
			},
			expectError: true,
		},
		{
			name: "id contains null character",
			event: &Event{
				ID:   "test\x00id",
				Data: "test",
			},
			expectError: true,
		},
		{
			name: "id contains newline",
			event: &Event{
				ID:   "test\nid",
				Data: "test",
			},
			expectError: true,
		},
		{
			name: "id contains carriage return",
			event: &Event{
				ID:   "test\rid",
				Data: "test",
			},
			expectError: true,
		},
		{
			name: "event contains newline",
			event: &Event{
				Event: "test\nevent",
				Data:  "test",
			},
			expectError: true,
		},
		{
			name: "event contains carriage return",
			event: &Event{
				Event: "test\revent",
				Data:  "test",
			},
			expectError: true,
		},
		{
			name: "raw data with newlines but no split",
			event: &Event{
				Data:  Raw("line1\nline2"),
				Split: false,
			},
			expectError: true,
		},
		{
			name: "raw data with invalid UTF-8",
			event: &Event{
				Data: Raw("\xFF\xFE"),
			},
			expectError: true,
		},
		{
			name: "json encoding failure",
			event: &Event{
				Data: complex(1, 2), // complex numbers can't be marshaled to JSON
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writer := new(strings.Builder)
			encoder := NewEncoder(writer)
			err := encoder.EncodeEvent(tt.event)

			if tt.expectError {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrValidation), "expected validation error")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestEncoder_WriteErrors(t *testing.T) {
	validEvent := &Event{
		ID:    "1",
		Event: "update",
		Data:  "Hello, world!",
	}

	t.Run("failing writer", func(t *testing.T) {
		encoder := NewEncoder(&failingWriter{})
		err := encoder.EncodeEvent(validEvent)

		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrValidation), "expected non-validation error")
		assert.Equal(t, "failed to write", err.Error())
	})
}

func TestValidationError_Unwrap(t *testing.T) {
	// Create sentinel errors that can be used with errors.Is
	innerErr := fmt.Errorf("inner error")
	rootErr := fmt.Errorf("root")

	tests := []struct {
		name          string
		err           error
		expectedCause error
		shouldUnwrap  bool
	}{
		{
			name:          "validation error with cause",
			err:           &validationError{Message: "outer error", Cause: innerErr},
			expectedCause: innerErr,
			shouldUnwrap:  true,
		},
		{
			name:          "validation error without cause",
			err:           &validationError{Message: "simple error"},
			expectedCause: nil,
			shouldUnwrap:  false,
		},
		{
			name:          "nested validation errors",
			err:           &validationError{Message: "outer", Cause: &validationError{Message: "inner", Cause: rootErr}},
			expectedCause: &validationError{Message: "inner", Cause: rootErr},
			shouldUnwrap:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test direct unwrapping
			unwrapped := errors.Unwrap(tt.err)
			if tt.shouldUnwrap {
				require.NotNil(t, unwrapped)
				assert.Equal(t, tt.expectedCause.Error(), unwrapped.Error())
			} else {
				assert.Nil(t, unwrapped)
			}

			// Test errors.Is behavior for root errors
			if tt.shouldUnwrap && tt.name == "validation error with cause" {
				assert.True(t, errors.Is(tt.err, innerErr), "should find the inner error")
			}

			if tt.shouldUnwrap && tt.name == "nested validation errors" {
				assert.True(t, errors.Is(tt.err, rootErr), "should find the root error")
			}
		})
	}
}

func TestValidationError_As(t *testing.T) {
	baseErr := &validationError{Message: "test error"}

	tests := []struct {
		name          string
		err           error
		shouldSucceed bool
		expectedMsg   string
	}{
		{
			name:          "direct validation error",
			err:           baseErr,
			shouldSucceed: true,
			expectedMsg:   "test error",
		},
		{
			name:          "wrapped validation error",
			err:           fmt.Errorf("wrapped: %w", baseErr),
			shouldSucceed: true,
			expectedMsg:   "test error",
		},
		{
			name:          "deeply wrapped validation error",
			err:           fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", baseErr)),
			shouldSucceed: true,
			expectedMsg:   "test error",
		},
		{
			name:          "non-validation error",
			err:           fmt.Errorf("regular error"),
			shouldSucceed: false,
		},
		{
			name:          "nil error",
			err:           nil,
			shouldSucceed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var target *validationError
			result := errors.As(tt.err, &target)
			assert.Equal(t, tt.shouldSucceed, result)

			if tt.shouldSucceed {
				require.NotNil(t, target)
				assert.Equal(t, tt.expectedMsg, target.Message)
			} else {
				assert.Nil(t, target)
			}
		})
	}
}

func TestValidationError_ErrorString(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "simple error",
			err:      &validationError{Message: "test error"},
			expected: "sse: test error",
		},
		{
			name:     "wrapped error",
			err:      &validationError{Message: "outer", Cause: fmt.Errorf("inner")},
			expected: "sse: outer: inner",
		},
		{
			name:     "nested validation errors",
			err:      &validationError{Message: "first", Cause: &validationError{Message: "second", Cause: fmt.Errorf("third")}},
			expected: "sse: first: sse: second: third",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}
