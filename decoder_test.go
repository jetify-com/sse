package sse

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoder(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []Event
		expectedErr string
	}{
		// Valid cases
		{
			name:  "simple message event",
			input: "id: 1\ndata: \"hello world\"\n\n",
			expected: []Event{
				{
					ID:   "1",
					Data: "hello world",
				},
			},
		},
		{
			name:     "comment only",
			input:    ": heartbeat\n\n",
			expected: nil,
		},
		{
			name:  "full event with all fields",
			input: "id: 42\nretry: 5000\nevent: update\ndata: {\"status\":\"ok\"}\n\n",
			expected: []Event{
				{
					ID:    "42",
					Event: "update",
					Data:  map[string]interface{}{"status": "ok"},
					Retry: 5000 * time.Millisecond,
				},
			},
		},
		{
			name:  "multiline data with different line endings",
			input: "data: line1\rdata: line2\r\ndata: line3\n\n",
			expected: []Event{
				{
					Data:  Raw("line1\nline2\nline3"),
					Split: true,
				},
			},
		},
		{
			name:  "data without space after colon",
			input: "data:test\n\n",
			expected: []Event{
				{
					Data: Raw("test"),
				},
			},
		},
		{
			name:  "data with space after colon",
			input: "data: test\n\n",
			expected: []Event{
				{
					Data: Raw("test"),
				},
			},
		},
		{
			name:  "data after comment",
			input: ": test\ndata: hello\n\n",
			expected: []Event{
				{
					Data: Raw("hello"),
				},
			},
		},
		{
			name:     "multiple comments",
			input:    ": test\n: another\n\n",
			expected: nil,
		},
		{
			name:  "multiple colons in data",
			input: "data: key: value: test\n\n",
			expected: []Event{
				{
					Data: Raw("key: value: test"),
				},
			},
		},
		{
			name:  "mixed line endings",
			input: "data: test\r\ndata: hello\rdata: world\n\n",
			expected: []Event{
				{
					Data:  Raw("test\nhello\nworld"),
					Split: true,
				},
			},
		},
		{
			name:     "no space after colon in comment",
			input:    ":comment\n\n",
			expected: nil,
		},
		{
			name:  "valid BOM at start",
			input: "\xEF\xBB\xBFdata: test\n\n",
			expected: []Event{
				{
					Data: Raw("test"),
				},
			},
		},
		{
			name:  "valid BOM at start with multiple fields",
			input: "\xEF\xBB\xBFid: 1\ndata: test\n\n",
			expected: []Event{
				{
					ID:   "1",
					Data: Raw("test"),
				},
			},
		},
		{
			name:  "multiple data fields concatenated",
			input: "data:first line\ndata:second line\n\n",
			expected: []Event{
				{
					Data:  Raw("first line\nsecond line"),
					Split: true,
				},
			},
		},
		{
			name:  "data field with trailing newline",
			input: "data:line with trailing newline\n\n\n",
			expected: []Event{
				{
					Data: Raw("line with trailing newline"),
				},
			},
		},
		{
			name:  "multiple data fields with empty lines",
			input: "data:first\ndata:\ndata:last\n\n",
			expected: []Event{
				{
					Data:  Raw("first\n\nlast"),
					Split: true,
				},
			},
		},
		{
			name:  "BOM in middle treated as data",
			input: "data: first\n\ndata: \xEF\xBB\xBFsecond\n\n",
			expected: []Event{
				{Data: Raw("first")},
				{Data: Raw("\uFEFFsecond")}, // BOM must be preserved
			},
		},
		{
			name:     "event field with no data is ignored",
			input:    "event: ping\n\n",
			expected: nil, // data buffer is empty → no dispatch
		},
		{
			name:  "data field without colon ⇒ empty string payload",
			input: "data\n\n",
			expected: []Event{
				{Data: Raw("")}, // spec § 9.2.6 example
			},
		},
		{
			name:  "two consecutive events",
			input: "data: one\n\nid: 2\ndata: two\n\n",
			expected: []Event{
				{Data: Raw("one")},
				{ID: "2", Data: Raw("two")},
			},
		},
		// Error cases
		{
			name:     "invalid retry value",
			input:    "retry: not_a_number\n\n",
			expected: nil, // No event should be created if all fields are ignored
		},
		{
			name:  "invalid JSON in data field",
			input: "data: {invalid json}\n\n",
			expected: []Event{
				{
					Data: Raw("{invalid json}"),
				},
			},
		},
		{
			name:        "no terminating newline",
			input:       "data: test",
			expectedErr: "unexpected EOF",
		},
		{
			name:  "null character in id field",
			input: "id: test\x00id\ndata: hello\n\n",
			expected: []Event{
				{
					Data: Raw("hello"),
				},
			},
		},
		{
			name:     "CR in id field only",
			input:    "id: test\rid\n\n",
			expected: nil, // No event should be created if all fields are ignored
		},
		{
			name:     "LF in id field only",
			input:    "id: test\nid\n\n",
			expected: nil, // No event should be created if all fields are ignored
		},
		{
			name:  "CR in id field with valid data",
			input: "id: test\rid\ndata: hello\nevent: update\n\n",
			expected: []Event{
				{
					Data:  Raw("hello"),
					Event: "update",
				},
			},
		},
		{
			name:  "LF in id field with valid data",
			input: "id: test\nid\ndata: hello\nevent: update\n\n",
			expected: []Event{
				{
					Data:  Raw("hello"),
					Event: "update",
				},
			},
		},
		{
			name:     "colon_in_field_name",
			input:    "da:ta: test\n\n",
			expected: nil, // Field names cannot contain colons per spec
		},
		{
			name:     "extra_spaces_before_colon",
			input:    "data   : test\n\n",
			expected: nil, // Field names cannot contain extra spaces before colon per spec
		},
		{
			name:  "invalid UTF-8 sequence",
			input: "data: \xFF\xFE test\n\n",
			expected: []Event{
				{
					Data: Raw("\xFF\xFE test"),
				},
			},
		},
		{
			name:     "retry with decimal value",
			input:    "retry: 1000.5\n\n",
			expected: nil,
		},
		{
			name:     "retry with negative value",
			input:    "retry: -1000\n\n",
			expected: nil, // No event should be created if all fields are ignored
		},
		{
			name:        "incomplete event - no final newline",
			input:       "data: test\n",
			expected:    nil,
			expectedErr: "unexpected EOF",
		},
		{
			name:        "incomplete event - partial field",
			input:       "data: test\ndata",
			expected:    nil,
			expectedErr: "unexpected EOF",
		},
		{
			name:        "incomplete event - partial field with colon",
			input:       "data: test\ndata:",
			expected:    nil,
			expectedErr: "unexpected EOF",
		},
		{
			name:  "field ordering - data before id",
			input: "data: test\nid: 123\n\n",
			expected: []Event{
				{
					ID:   "123",
					Data: Raw("test"),
				},
			},
		},
		{
			name:  "field ordering - event before data",
			input: "event: update\ndata: test\n\n",
			expected: []Event{
				{
					Event: "update",
					Data:  Raw("test"),
				},
			},
		},
		{
			name:  "multiple fields with same name",
			input: "data: line1\ndata: line2\nid: 123\nid: 456\nretry: 1000\nretry: 2000\n\n",
			expected: []Event{
				{
					ID:    "456",
					Data:  Raw("line1\nline2"),
					Split: true,
					Retry: 2000 * time.Millisecond,
				},
			},
		},
		{
			name:  "empty field values",
			input: "data:\nid:\nevent:\nretry: 0\n\n",
			expected: []Event{
				{
					Data:  Raw(""),
					ID:    "",
					Event: "",
					Retry: 0,
				},
			},
		},
		{
			name:  "only whitespace in field values",
			input: "data:  \nid:  \nevent:  \n\n",
			expected: []Event{
				{
					Data:  Raw(" "),
					ID:    " ",
					Event: " ",
				},
			},
		},
		{
			name:  "mixed empty and non-empty fields",
			input: "data:\ndata: test\nid:\nid: 123\n\n",
			expected: []Event{
				{
					Data:  Raw("\ntest"),
					ID:    "123",
					Split: true,
				},
			},
		},
		{
			name:     "incomplete event",
			input:    "event: update\n",
			expected: nil,
		},
		{
			name:  "only carriage return",
			input: "data: test\r\r",
			expected: []Event{
				{
					Data: Raw("test"),
				},
			},
		},
		{
			name:  "non-numeric retry with valid data",
			input: "retry: not_a_number\ndata: hello\nevent: update\n\n",
			expected: []Event{
				{
					Data:  Raw("hello"),
					Event: "update",
				},
			},
		},
		{
			name:  "decimal retry with valid data",
			input: "retry: 1000.5\ndata: hello\nevent: update\n\n",
			expected: []Event{
				{
					Data:  Raw("hello"),
					Event: "update",
				},
			},
		},
		{
			name:  "negative retry with valid data",
			input: "retry: -1000\ndata: hello\nevent: update\n\n",
			expected: []Event{
				{
					Data:  Raw("hello"),
					Event: "update",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			var events []Event
			var err error

			// Read all events from the input
			for {
				var ev Event
				err = decoder.Decode(&ev)
				if errors.Is(err, io.EOF) {
					err = nil // EOF is a normal stream termination
					break
				}
				if err != nil {
					break
				}
				events = append(events, ev)
			}

			if tt.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, events)
			}
		})
	}
}

func TestQuick_DataConcatenation(t *testing.T) {
	// Property: any non-empty slice of "safe" strings should round-trip
	// so that the Event.Data equals strings.Join(lines, "\n").
	dataConcatenationProperty := func(lines []string) bool {
		if len(lines) == 0 {
			return true // vacuous truth; quick may supply empty slice
		}

		// Filter out strings containing CR/LF/colon because those would
		// change control-flow or become field-names.
		for _, s := range lines {
			if strings.ContainsAny(s, "\r\n:") {
				return true
			}
		}

		// Build an SSE frame.
		var sb strings.Builder
		for _, s := range lines {
			sb.WriteString("data: ")
			sb.WriteString(s)
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n') // frame delimiter

		dec := NewDecoder(strings.NewReader(sb.String()))
		var ev Event
		if err := dec.Decode(&ev); err != nil {
			return false
		}

		want := strings.Join(lines, "\n")

		// Your Event.Data may be string or Raw(string); handle both.
		switch v := ev.Data.(type) {
		case string:
			return v == want
		case Raw:
			return string(v) == want
		default:
			return false
		}
	}

	cfg := &quick.Config{
		MaxCount:      500, // plenty; adjust to taste
		MaxCountScale: 1.0, // default scale
	}

	if err := quick.Check(dataConcatenationProperty, cfg); err != nil {
		t.Errorf("property check failed: %v", err)
	}
}

func TestDecoder_RetryDelay(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "no retry field",
			input:    "data: test\n\n",
			expected: 0,
		},
		{
			name:     "valid retry value",
			input:    "retry: 5000\ndata: test\n\n",
			expected: 5000 * time.Millisecond,
		},
		{
			name:     "invalid retry value",
			input:    "retry: invalid\ndata: test\n\n",
			expected: 0,
		},
		{
			name:     "negative retry value",
			input:    "retry: -1000\ndata: test\n\n",
			expected: 0,
		},
		{
			name:     "decimal retry value",
			input:    "retry: 1000.5\ndata: test\n\n",
			expected: 0,
		},
		{
			name:     "multiple retry values - last valid wins",
			input:    "retry: 3000\nretry: 5000\ndata: test\n\n",
			expected: 5000 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			var ev Event
			err := decoder.Decode(&ev)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, decoder.RetryDelay())
		})
	}
}

func TestDecoder_LastEventID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no id field",
			input:    "data: test\n\n",
			expected: "",
		},
		{
			name:     "single id",
			input:    "id: 123\ndata: test\n\n",
			expected: "123",
		},
		{
			name:     "multiple ids - last one wins",
			input:    "id: 123\nid: 456\ndata: test\n\n",
			expected: "456",
		},
		{
			name:     "id with special characters",
			input:    "id: test-123_xyz\ndata: test\n\n",
			expected: "test-123_xyz",
		},
		{
			name:     "empty id",
			input:    "id:\ndata: test\n\n",
			expected: "",
		},
		{
			name:     "id persists across events",
			input:    "id: 123\ndata: first\n\ndata: second\n\n",
			expected: "123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewDecoder(strings.NewReader(tt.input))
			var ev Event
			err := decoder.Decode(&ev)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, decoder.LastEventID())
		})
	}
}

func TestEvent_UnmarshalText(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    Event
		expectError bool
	}{
		{
			name:  "simple event",
			input: "data: test\n\n",
			expected: Event{
				Data: Raw("test"),
			},
		},
		{
			name:  "full event with JSON object",
			input: "id: 123\nevent: update\nretry: 5000\ndata: {\"status\":\"ok\"}\n\n",
			expected: Event{
				ID:    "123",
				Event: "update",
				Retry: 5000 * time.Millisecond,
				Data:  map[string]interface{}{"status": "ok"},
			},
		},
		{
			name:  "JSON array data",
			input: "data: [1,2,3]\n\n",
			expected: Event{
				Data: []interface{}{float64(1), float64(2), float64(3)},
			},
		},
		{
			name:  "JSON string data",
			input: "data: \"hello world\"\n\n",
			expected: Event{
				Data: "hello world",
			},
		},
		{
			name:  "non-JSON data starting with brace",
			input: "data: {not valid json}\n\n",
			expected: Event{
				Data: Raw("{not valid json}"),
			},
		},
		{
			name:  "data that looks like JSON but isn't properly formatted",
			input: "data: status: ok\n\n",
			expected: Event{
				Data: Raw("status: ok"),
			},
		},
		{
			name:  "multiline data",
			input: "data: line1\ndata: line2\ndata: line3\n\n",
			expected: Event{
				Data:  Raw("line1\nline2\nline3"),
				Split: true,
			},
		},
		{
			name:        "incomplete event",
			input:       "data: test",
			expectError: true,
		},
		{
			name:        "invalid retry value",
			input:       "retry: invalid\ndata: test\n\n",
			expectError: false, // Invalid retry is ignored per spec
			expected: Event{
				Data: Raw("test"),
			},
		},
		{
			name:     "empty event with just newlines",
			input:    "\n\n",
			expected: Event{},
		},
		{
			name:     "comment only event",
			input:    ": this is a comment\n\n",
			expected: Event{},
		},
		{
			name:  "event with BOM",
			input: "\xEF\xBB\xBFdata: test\n\n",
			expected: Event{
				Data: Raw("test"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ev Event
			err := ev.UnmarshalText([]byte(tt.input))
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, ev)
			}
		})
	}
}
