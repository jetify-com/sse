package sse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Encoder writes SSE events to an io.Writer.
// It is not safe for concurrent use; each instance should be
// confined to a single goroutine or callers must serialize access.
type Encoder struct {
	w   io.Writer
	buf bytes.Buffer
}

// NewEncoder creates a new SSE encoder that writes to the provided io.Writer.
func NewEncoder(w io.Writer) *Encoder { return &Encoder{w: w} }

// EncodeEvent encodes an SSE event and writes it to the underlying io.Writer.
// It returns an error if the event is invalid or if writing fails.
func (enc *Encoder) EncodeEvent(e *Event) error {
	if e == nil {
		return &validationError{Message: "nil event"}
	}
	enc.buf.Reset()
	if err := writeEvent(&enc.buf, e); err != nil {
		// All validation and JSON errors from writeEvent are already properly typed
		return err
	}
	_, err := enc.w.Write(enc.buf.Bytes())
	return err
}

// EncodeComment writes an SSE comment to the underlying io.Writer.
// Comments in SSE are lines that start with a colon (:).
func (enc *Encoder) EncodeComment(c string) error {
	enc.buf.Reset()
	if err := writeComment(&enc.buf, c); err != nil {
		return err
	}
	_, err := enc.w.Write(enc.buf.Bytes())
	return err
}

// writeEvent writes a single SSE event to the provided io.Writer.
// It handles all SSE event fields (id, retry, event, data) according to the SSE specification.
func writeEvent(w io.Writer, e *Event) error {
	if err := e.Validate(); err != nil {
		return err
	}

	// ----- id -----
	if e.ID != "" {
		fmt.Fprintf(w, "id: %s\n", e.ID)
	}

	// ----- retry -----
	if e.Retry > 0 {
		fmt.Fprintf(w, "retry: %d\n", e.Retry.Milliseconds())
	}

	// ----- event -----
	if e.Event != "" && e.Event != "message" {
		fmt.Fprintf(w, "event: %s\n", e.Event)
	}

	// ----- data -----
	switch v := e.Data.(type) {
	case nil:
		// allowed ― meta-only block
	case Raw:
		writeRaw(w, v, e.Split)
	default:
		jb, err := json.Marshal(v)
		if err != nil {
			return &validationError{Message: "json encoding failed", Cause: err}
		}
		writeData(w, jb)
	}

	// ----- blank line -----
	fmt.Fprint(w, "\n")
	return nil
}

// writeComment writes an SSE comment to the provided io.Writer.
// Comments in SSE are lines that start with a colon (:).
func writeComment(w io.Writer, c string) error {
	fmt.Fprintf(w, ": %s\n", c)
	return nil
}

// writeRaw writes raw data as SSE data lines.
// If split is true, it splits the data on newline boundaries.
// If split is false, it writes the data as a single line (validation ensures it contains no newlines).
// When split=true, trailing newlines are properly handled - they are excluded from output
// per the SSE spec (section 9.2.6) which states that if a data buffer's last character
// is a newline, it should be removed.
func writeRaw(w io.Writer, v Raw, split bool) {
	if len(v) == 0 {
		return
	}

	if split {
		splitAndWriteLines(w, v)
		return
	}

	// split==false: we've already validated there are no newlines
	writeData(w, v)
}

// splitAndWriteLines processes raw data and splits it at line boundaries,
// handling CR, LF, and CRLF line endings according to the SSE spec.
// Each line is written as a separate "data:" field.
//
// This implementation mirrors the line ending handling in the decoder's readLine method,
// ensuring consistent behavior with how the decoder processes incoming events.
// All three line endings specified in the SSE spec (section 9.2.5) are properly handled:
// - CR (\r)
// - LF (\n)
// - CRLF (\r\n)
//
// All line endings are normalized to LF in the output as per the SSE spec.
func splitAndWriteLines(w io.Writer, v Raw) {
	start := 0
	for i := 0; i < len(v); i++ {
		// Handle CR, LF, or CRLF as line endings
		if v[i] == '\n' {
			// Handle LF and CRLF (trim any preceding CR)
			end := i
			if end > start && v[end-1] == '\r' {
				end-- // Don't include the CR in output
			}
			writeData(w, v[start:end])
			start = i + 1
			// Note: if the last character was a newline, after this loop completes,
			// 'start' will equal len(v), causing the final check to be skipped.
			// This correctly handles trailing newlines per the SSE spec.
		} else if v[i] == '\r' {
			// Check if this CR is part of CRLF
			if i+1 < len(v) && v[i+1] == '\n' {
				// This is part of CRLF, will be handled in the next iteration
				continue
			}
			// Standalone CR
			writeData(w, v[start:i])
			start = i + 1
			// Same note applies for trailing CR: it will result in 'start' being
			// set to len(v), causing no additional content to be written.
		}
	}

	// Write any remaining data after the last newline
	if start < len(v) {
		writeData(w, v[start:])
	}
}

// writeData writes a single SSE data line to the provided io.Writer.
// It prefixes the data with "data: " and appends a newline.
func writeData(w io.Writer, b []byte) {
	fmt.Fprintf(w, "data: %s\n", b)
}
