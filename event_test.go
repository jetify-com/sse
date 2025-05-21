package sse

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvent_MarshalText(t *testing.T) {
	tests := []struct {
		name            string
		event           Event
		expected        string
		expectedErr     string
		isValidationErr bool // Flag to check if the error should be a ValidationError
	}{
		// Valid cases
		{
			name: "simple message event",
			event: Event{
				ID:   "1",
				Data: "hello world",
			},
			expected: "id: 1\ndata: \"hello world\"\n\n",
		},
		{
			name: "full event with all fields",
			event: Event{
				ID:    "42",
				Event: "update",
				Data:  map[string]string{"status": "ok"},
				Retry: 5000 * time.Millisecond,
			},
			expected: "id: 42\nretry: 5000\nevent: update\ndata: {\"status\":\"ok\"}\n\n",
		},
		{
			name: "multiline data as slice",
			event: Event{
				Data: []string{"line1", "line2", "line3"},
			},
			expected: "data: [\"line1\",\"line2\",\"line3\"]\n\n",
		},
		{
			name: "raw data with split lines",
			event: Event{
				Data:  Raw("line1\nline2\nline3"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "raw data without split",
			event: Event{
				Data:  Raw("line1\nline2\nline3"),
				Split: false,
			},
			expectedErr:     "raw data contains newlines but Split is false",
			isValidationErr: true,
		},
		{
			name: "raw data with newlines requires split",
			event: Event{
				Data:  Raw("line1\nline2\nline3"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "empty data field",
			event: Event{
				Data: "",
			},
			expected: "data: \"\"\n\n",
		},
		{
			name:     "zero values",
			event:    Event{},
			expected: "\n", // Just the terminating newline
		},
		{
			name: "custom event type only",
			event: Event{
				Event: "custom",
			},
			expected: "event: custom\n\n",
		},
		{
			name: "retry only",
			event: Event{
				Retry: 1000 * time.Millisecond,
			},
			expected: "retry: 1000\n\n",
		},
		{
			name: "complex nested data",
			event: Event{
				Data: map[string]interface{}{
					"users": []map[string]interface{}{
						{"id": 1, "name": "Alice"},
						{"id": 2, "name": "Bob"},
					},
				},
			},
			expected: "data: {\"users\":[{\"id\":1,\"name\":\"Alice\"},{\"id\":2,\"name\":\"Bob\"}]}\n\n",
		},
		{
			name: "data with special characters",
			event: Event{
				Data: "line1\nline2\rline3",
			},
			expected: "data: \"line1\\nline2\\rline3\"\n\n",
		},
		{
			name: "message event type",
			event: Event{
				Event: "message",
				Data:  "hello",
			},
			expected: "data: \"hello\"\n\n", // "message" event type should be omitted
		},
		{
			name: "raw data with special characters",
			event: Event{
				Data:  Raw("hello: world"),
				Split: false,
			},
			expected: "data: hello: world\n\n",
		},
		{
			name: "data with trailing newline",
			event: Event{
				Data:  Raw("line1\n"),
				Split: true,
			},
			expected: "data: line1\n\n", // spec 9.2.6: remove trailing LF from data buffer
		},
		{
			name: "multiple data lines with trailing newlines",
			event: Event{
				Data:  Raw("line1\n\nline2\n"),
				Split: true,
			},
			expected: "data: line1\ndata: \ndata: line2\n\n",
		},
		{
			name: "retry with different duration units",
			event: Event{
				Retry: 5 * time.Second,
			},
			expected: "retry: 5000\n\n",
		},
		{
			name: "retry with minutes",
			event: Event{
				Retry: 2 * time.Minute,
			},
			expected: "retry: 120000\n\n",
		},
		{
			name: "retry with hours",
			event: Event{
				Retry: time.Hour,
			},
			expected: "retry: 3600000\n\n",
		},
		{
			name: "retry with decimal milliseconds",
			event: Event{
				Retry: 2*time.Second + 500*time.Millisecond + 500*time.Microsecond,
			},
			expected: "retry: 2500\n\n", // Should round 2500.5ms to 2500ms, spec only allows integers
		},

		// Error cases
		{
			name: "invalid data type",
			event: Event{
				Data: complex(1, 2),
			},
			expectedErr:     "json encoding failed",
			isValidationErr: true,
		},
		{
			name: "negative ID is valid per spec",
			event: Event{
				ID: "-1",
			},
			expected: "id: -1\n\n",
		},
		{
			name: "invalid retry",
			event: Event{
				Retry: -1000 * time.Millisecond,
			},
			expectedErr:     "retry must be >=0",
			isValidationErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.event.MarshalText()
			if tt.expectedErr != "" {
				assert.Error(t, err)
				if tt.isValidationErr {
					assert.True(t, errors.Is(err, ErrValidation), "Expected ValidationError but got: %v", err)
				}
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, string(result))
			}
		})
	}
}
