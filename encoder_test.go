package sse

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncoder_NewEncoder(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	assert.NotNil(t, enc)
	assert.Equal(t, &buf, enc.w)
	assert.Zero(t, enc.buf.Len())
}

func TestEncoder_EncodeEvent(t *testing.T) {
	tests := []struct {
		name            string
		event           *Event
		expected        string
		isValidationErr bool // Check for ValidationError type instead of string content
	}{
		{
			name: "simple message event",
			event: &Event{
				ID:   "1",
				Data: "hello world",
			},
			expected: "id: 1\ndata: \"hello world\"\n\n",
		},
		{
			name: "full event with all fields",
			event: &Event{
				ID:    "42",
				Event: "update",
				Data:  map[string]string{"status": "ok"},
				Retry: 5000 * time.Millisecond,
			},
			expected: "id: 42\nretry: 5000\nevent: update\ndata: {\"status\":\"ok\"}\n\n",
		},
		{
			name: "raw data with split lines",
			event: &Event{
				Data:  Raw("line1\nline2\nline3"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "raw data with CRLF line endings and split=true",
			event: &Event{
				Data:  Raw("line1\r\nline2\r\nline3"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "raw data with standalone CR line endings and split=true",
			event: &Event{
				Data:  Raw("line1\rline2\rline3"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\ndata: line3\n\n",
		},
		{
			name: "raw data with mixed line endings and split=true",
			event: &Event{
				Data:  Raw("line1\nline2\rline3\r\nline4"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\ndata: line3\ndata: line4\n\n",
		},
		{
			name: "raw data with trailing newline when split=true",
			event: &Event{
				Data:  Raw("line1\nline2\n"),
				Split: true,
			},
			expected: "data: line1\ndata: line2\n\n",
		},
		{
			name: "raw data without split and no newlines",
			event: &Event{
				Data:  Raw("simple text"),
				Split: false,
			},
			expected: "data: simple text\n\n",
		},
		{
			name: "empty data field",
			event: &Event{
				Data: "",
			},
			expected: "data: \"\"\n\n",
		},
		{
			name:            "nil event",
			event:           nil,
			isValidationErr: true,
		},
		{
			name: "invalid event - raw data with newlines but no split",
			event: &Event{
				Data:  Raw("line1\nline2"),
				Split: false,
			},
			isValidationErr: true,
		},
		{
			name: "meta-only event",
			event: &Event{
				ID:    "meta1",
				Event: "meta",
				Retry: 1000 * time.Millisecond,
			},
			expected: "id: meta1\nretry: 1000\nevent: meta\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewEncoder(&buf)

			err := enc.EncodeEvent(tt.event)

			if tt.isValidationErr {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrValidation), "Expected a ValidationError but got: %v", err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, buf.String())
			}
		})
	}
}

// TestEncoder_EncodeEvent_Nil tests that encoding a nil event returns a ValidationError
func TestEncoder_EncodeEvent_Nil(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	err := enc.EncodeEvent(nil)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation), "Expected a ValidationError")
	assert.Equal(t, "sse: nil event", err.Error())
}

func TestEncoder_EncodeComment(t *testing.T) {
	tests := []struct {
		name     string
		comment  string
		expected string
	}{
		{
			name:     "simple comment",
			comment:  "test comment",
			expected: ": test comment\n",
		},
		{
			name:     "empty comment",
			comment:  "",
			expected: ": \n",
		},
		{
			name:     "comment with special chars",
			comment:  "keep-alive: 15s",
			expected: ": keep-alive: 15s\n",
		},
		{
			name:     "multiline comment still encoded as single line",
			comment:  "line1\nline2", // Comments are not split on newlines
			expected: ": line1\nline2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewEncoder(&buf)

			err := enc.EncodeComment(tt.comment)

			require.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}

func TestEncoder_Reuse(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// First event
	event1 := &Event{ID: "1", Data: "first"}
	err := enc.EncodeEvent(event1)
	require.NoError(t, err)

	// Comment
	err = enc.EncodeComment("test")
	require.NoError(t, err)

	// Second event
	event2 := &Event{ID: "2", Data: "second"}
	err = enc.EncodeEvent(event2)
	require.NoError(t, err)

	// Verify all content was written in sequence
	expected := "id: 1\ndata: \"first\"\n\n: test\nid: 2\ndata: \"second\"\n\n"
	assert.Equal(t, expected, buf.String())
}

func TestEncoder_WriteFailure(t *testing.T) {
	// Create a writer that always fails
	failWriter := &failingWriter{}
	enc := NewEncoder(failWriter)

	// Try to encode a valid event
	event := &Event{Data: "test"}
	err := enc.EncodeEvent(event)

	// Should return the write error (not a ValidationError)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrValidation), "IO errors should not be ValidationErrors")
	assert.Contains(t, err.Error(), "failed to write")

	// Try to encode a comment
	err = enc.EncodeComment("test")
	assert.Error(t, err)
	assert.False(t, errors.Is(err, ErrValidation), "IO errors should not be ValidationErrors")
	assert.Contains(t, err.Error(), "failed to write")
}

// TestEncoder_JSONError tests that a JSON error is wrapped as a ValidationError
func TestEncoder_JSONError(t *testing.T) {
	var buf bytes.Buffer
	enc := NewEncoder(&buf)

	// Create an event with data that can't be marshaled to JSON
	event := &Event{
		Data: complex(1, 2), // complex numbers can't be marshaled to JSON
	}

	err := enc.EncodeEvent(event)

	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrValidation), "JSON errors should be wrapped as ValidationErrors")
	assert.Contains(t, err.Error(), "json encoding failed")
}

// failingWriter is a test helper that implements io.Writer but always returns an error
type failingWriter struct{}

func (fw *failingWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("failed to write")
}
