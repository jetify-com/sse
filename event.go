package sse

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Raw represents pre-encoded bytes data for SSE events
type Raw []byte

// Event represents a Server-Sent Event (SSE) message.
type Event struct {
	// ID is a unique identifier used for reconnection logic.
	// When empty, the id field is omitted from the SSE output.
	ID string

	// Event specifies the event type. If empty or set to "message",
	// the event field is omitted from the SSE output, defaulting to
	// a standard message event.
	Event string

	// Data contains the event payload. It will be JSON-encoded unless
	// it is of type Raw. If nil, creates a comment-only or meta-only event.
	Data any

	// Retry specifies the reconnection delay.
	// When zero, the retry field is omitted from the SSE output.
	Retry time.Duration

	// Split controls whether Raw text data should be split on newlines
	// into multiple data: fields in the output. Only applies to Raw data.
	Split bool

	// Timestamp records the server time of the event.
	// This field is only used for TTL-based filtering and is not output
	// in the wire format.
	Timestamp time.Time
}

// Validate performs validation checks on the Event fields.
// Returns an error if any validation fails.
func (e Event) Validate() error {
	// ----- id validation -----
	if e.ID != "" {
		if strings.ContainsAny(e.ID, "\x00\r\n") {
			return &validationError{Message: "id contains forbidden char"}
		}
	}

	// ----- retry validation -----
	if e.Retry < 0 {
		return &validationError{Message: "retry must be >=0"}
	}

	// ----- event validation -----
	if e.Event != "" && e.Event != "message" {
		if strings.ContainsAny(e.Event, "\r\n") {
			return &validationError{Message: "event contains newline"}
		}
	}

	// ----- data validation -----
	if raw, ok := e.Data.(Raw); ok {
		if !utf8.Valid(raw) {
			return &validationError{Message: "raw payload not valid UTF-8"}
		}
		// Fail validation if Split is false and data contains newlines
		if !e.Split && bytes.ContainsAny(raw, "\r\n") {
			return &validationError{Message: "raw data contains newlines but Split is false"}
		}
	}

	return nil
}

// MarshalText implements encoding.TextMarshaler, returning UTF‑8 bytes that
// represent a complete event including the trailing blank line.
func (e Event) MarshalText() ([]byte, error) {
	var buf bytes.Buffer
	if err := writeEvent(&buf, &e); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// UnmarshalText implements encoding.TextUnmarshaler, parsing a single SSE event
// from the provided text. The text should include the trailing blank line.
func (e *Event) UnmarshalText(text []byte) error {
	// Create a temporary decoder to parse the event
	dec := NewDecoder(bytes.NewReader(text))

	// Parse the event
	if err := dec.Decode(e); err != nil {
		if errors.Is(err, io.EOF) {
			// EOF is expected after a complete event
			return nil
		}
		return fmt.Errorf("sse: failed to unmarshal event: %w", err)
	}

	// Validate the parsed event
	return e.Validate()
}
