package sse

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
)

// Decoder streams SSE events from an io.Reader.
// It is safe to create multiple Decoders but **not** to use one Decoder
// concurrently from multiple goroutines.
type Decoder struct {
	reader      *bufio.Reader
	lastEventID string
	retryDelay  time.Duration
	bomSkipped  bool
}

// NewDecoder wraps r in a buffered reader and returns a ready Decoder.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{reader: bufio.NewReaderSize(r, 32<<10)} // 32 KiB buffer
}

// RetryDelay returns the last well-formed retry value from the SSE stream.
// This value represents how long clients should wait before reconnecting if
// the connection is lost (corresponds to the "retry:" field in the SSE spec).
// Returns 0 if no retry value has been set.
func (d *Decoder) RetryDelay() time.Duration {
	return d.retryDelay
}

// LastEventID exposes the most recent id: field verbatim.
func (d *Decoder) LastEventID() string { return d.lastEventID }

// Decode reads from the underlying io.Reader until it has consumed one
// complete Server-Sent Events frame (i.e. every line has been read up to and
// including the mandatory blank line delimiter defined in WHATWG HTML § 9.2.5).
//
// When a full frame is parsed the supplied *event is overwritten with
// fresh field values and Decode returns nil.
//
// The call blocks until either a frame is finished, or the underlying
// Reader returns an error.
//
// Error semantics:
//   - io.EOF: the upstream closed after the last delimiter; no further events are possible.
//   - io.ErrUnexpectedEOF: upstream closed before the blank line of the current frame (partial event was discarded).
//   - Any other error: bubbled up unchanged.
//
// Field handling:
//   - data: concatenated with "\n", exposed verbatim or auto-decoded to JSON into `Event.Data`.
//     Data is auto-decoded to JSON if it starts with '{', '[', or '"' and is valid JSON.
//     Otherwise it's stored as Raw type.
//   - id: sets Event.ID unless it contains NUL (ignored per spec).
//   - event: sets Event.Event (default "message").
//   - retry: parsed as an int › Event.Retry; invalid values are ignored.
//   - unknown / malformed fields are silently skipped.
//
// Character encoding & line endings:
//   - Input is always interpreted as UTF-8; a single leading BOM is stripped.
//   - Accepts CR, LF, or CRLF line breaks transparently.
//
// Reuse & concurrency:
//   - The *event parameter is cleared on every invocation, so callers may
//     pass the same struct repeatedly to avoid allocations.
//   - A Decoder is not safe for concurrent use without external locking.
//
// Typical usage:
//
//	resp, _ := http.Get(url)
//	dec   := sse.NewDecoder(resp.Body)
//	for {
//	    var ev sse.Event
//	    if err := dec.Decode(&ev); err != nil {
//	        if errors.Is(err, io.EOF) { break }
//	        log.Fatal(err)
//	    }
//	    handle(ev)
//	}
func (d *Decoder) Decode(event *Event) error {
	// zero out caller-supplied struct
	*event = Event{}

	var (
		dataBuf   string
		eventType string
		dataLines int // number of "data:" lines seen in current block
	)

	for {
		line, err := d.readLine()
		if err != nil {
			return d.handleReadError(err, dataBuf)
		}

		// Handle blank line (dispatch event)
		if line == "" {
			if dataBuf == "" {
				// Nothing to fire; spec still demands buffers reset.
				dataBuf, eventType = "", ""
				dataLines = 0
				continue
			}

			return d.dispatchEvent(event, dataBuf, eventType, dataLines)
		}

		// Handle comment line
		if strings.HasPrefix(line, ":") {
			continue
		}

		// Process field line
		field, val, _ := strings.Cut(line, ":")
		if len(val) > 0 && val[0] == ' ' {
			val = val[1:]
		}

		switch field {
		case "event":
			eventType = val
		case "data":
			dataBuf += val + "\n" // always append "\n" (spec 9.2.6)
			dataLines++
		case "id":
			d.processIDField(val)
		case "retry":
			d.processRetryField(val, event)
		}
	}
}

// handleReadError processes errors that occur during line reading
func (d *Decoder) handleReadError(err error, dataBuf string) error {
	if errors.Is(err, io.EOF) {
		if dataBuf == "" {
			return io.EOF // graceful end of stream
		}
		return io.ErrUnexpectedEOF // stream ended mid-event
	}
	return err
}

// dispatchEvent finalizes and populates the event before returning it
func (d *Decoder) dispatchEvent(event *Event, dataBuf, eventType string, dataLines int) error {
	// Trim exactly one trailing \n (added after every data line).
	if dataBuf[len(dataBuf)-1] == '\n' {
		dataBuf = dataBuf[:len(dataBuf)-1]
	}

	// Populate Event.
	event.ID = d.lastEventID // no numeric conversion; keep exact string
	event.Event = eventType
	if d.retryDelay > 0 {
		event.Retry = d.retryDelay
	}
	event.Split = dataLines > 1

	// Decide Data vs Comment and JSON decode if possible.
	if !strings.HasPrefix(dataBuf, ":") {
		d.parseEventData(event, dataBuf)
	}
	// else {
	// 	// If we ever want to return comments as events, we can do it here.
	// }

	return nil
}

// parseEventData parses the event data, handling JSON if applicable
func (d *Decoder) parseEventData(event *Event, dataBuf string) {
	trimmed := strings.TrimSpace(dataBuf)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[' || trimmed[0] == '"') {
		var v any
		if err := json.Unmarshal([]byte(trimmed), &v); err == nil {
			event.Data = v
			return
		}
	}
	event.Data = Raw(dataBuf) // leave as-is if invalid JSON
}

// processIDField handles the "id:" field
func (d *Decoder) processIDField(val string) {
	if !strings.ContainsRune(val, '\x00') {
		d.lastEventID = val // empty string allowed (resets header)
	}
}

// processRetryField handles the "retry:" field
func (d *Decoder) processRetryField(val string, event *Event) {
	if asciiDigits(val) {
		if ms, _ := strconv.Atoi(val); ms >= 0 {
			d.retryDelay = time.Duration(ms) * time.Millisecond
			event.Retry = d.retryDelay
		}
	}
}

// readLine consumes a single logical line (CR, LF, or CRLF terminator).
// It also strips exactly one byte-order-mark on the very first call.
func (d *Decoder) readLine() (string, error) {
	// Handle UTF-8 BOM once.
	if !d.bomSkipped {
		d.bomSkipped = true
		if r, _, err := d.reader.ReadRune(); err != nil {
			return "", err
		} else if r != '\uFEFF' {
			_ = d.reader.UnreadRune()
		}
	}

	var line []byte

	// Read until we get a complete line, properly handling CR, LF, or CRLF line endings
	for {
		c, err := d.reader.ReadByte()
		if err != nil {
			if len(line) > 0 {
				// Return the partial line we have so far
				return string(line), nil
			}
			return "", err // Propagate io.EOF etc.
		}

		if c == '\r' {
			// Read ahead to see if next char is LF
			next, err := d.reader.ReadByte()
			if err == nil && next == '\n' {
				// CRLF - consume both and end line
				break
			} else if err == nil {
				// Just CR, not followed by LF
				// Unread the next byte and end line
				_ = d.reader.UnreadByte()
				break
			} else {
				// Error reading next byte (could be EOF)
				// End line with the CR we already found
				break
			}
		} else if c == '\n' {
			// LF - end line
			break
		} else {
			// Regular character - add to line
			line = append(line, c)
		}
	}

	return string(line), nil
}

// asciiDigits reports whether s is a non-empty string of ASCII 0-9.
func asciiDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
