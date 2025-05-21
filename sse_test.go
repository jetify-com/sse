package sse

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDeadline(t *testing.T) {
	// Save and restore the original now function
	originalNow := now
	defer func() { now = originalNow }()

	// Mock time
	mockNow := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	now = func() time.Time {
		return mockNow
	}

	tests := []struct {
		name           string
		ctx            context.Context
		defaultTimeout time.Duration
		want           time.Time
	}{
		{
			name: "context with deadline",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(t.Context(), time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC))
				defer cancel()
				return ctx
			}(),
			defaultTimeout: time.Second,
			want:           time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC),
		},
		{
			name:           "context without deadline",
			ctx:            t.Context(),
			defaultTimeout: time.Second,
			want:           time.Date(2024, 1, 1, 0, 0, 1, 0, time.UTC), // mockNow + 1 second
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := getDeadline(testCase.ctx, testCase.defaultTimeout)
			assert.Equal(t, testCase.want, got)
		})
	}
}

// mockWriter implements all required interfaces
type mockWriter struct {
	http.ResponseWriter
	http.Flusher
	writeDeadliner
}

// mockUnwrapper implements ResponseWriterUnwrapper
type mockUnwrapper struct {
	mockWriter
	wrapped http.ResponseWriter
}

func (m *mockUnwrapper) Unwrap() http.ResponseWriter {
	return m.wrapped
}

// basicWriter is a minimal ResponseWriter implementation
type basicWriter struct {
	headers http.Header
}

func (b *basicWriter) Header() http.Header {
	if b.headers == nil {
		b.headers = make(http.Header)
	}
	return b.headers
}

func (b *basicWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (b *basicWriter) WriteHeader(int) {}

func TestUnwrapResponseWriter(t *testing.T) {
	tests := []struct {
		name      string
		writer    http.ResponseWriter
		wantFlush bool
		wantDL    bool
	}{
		{
			name:      "simple writer",
			writer:    &mockWriter{},
			wantFlush: true,
			wantDL:    true,
		},
		{
			name: "wrapped writer",
			writer: &mockUnwrapper{
				wrapped: &mockWriter{},
			},
			wantFlush: true,
			wantDL:    true,
		},
		{
			name:      "basic response writer",
			writer:    &basicWriter{},
			wantFlush: false,
			wantDL:    false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			flush, dl := unwrapResponseWriter(testCase.writer)
			assert.Equal(t, testCase.wantFlush, flush != nil)
			assert.Equal(t, testCase.wantDL, dl != nil)
		})
	}
}

func TestSendHeartbeat(t *testing.T) {
	w := httptest.NewRecorder()
	conn := &Conn{
		enc:       NewEncoder(w),
		flush:     w,
		hbComment: "heartbeat",
	}

	err := conn.sendHeartbeat(t.Context())
	assert.NoError(t, err)

	output := w.Body.String()
	assert.Contains(t, output, ": heartbeat\n")
}

func TestSSEIntegration(t *testing.T) {
	tests := []struct {
		name   string
		opts   []Option
		events []*Event
	}{
		{
			name: "simple message",
			opts: []Option{
				WithRetryDelay(0), // Disable default retry
			},
			events: []*Event{
				{Data: "hello"},
			},
		},
		{
			name: "multiple events",
			opts: []Option{
				WithRetryDelay(0), // Disable default retry
			},
			events: []*Event{
				{Data: "first"},
				{Data: "second"},
			},
		},
		{
			name: "event with all fields",
			opts: []Option{
				WithRetryDelay(0), // Disable default retry
			},
			events: []*Event{
				{
					ID:    "123",
					Event: "test",
					Data:  "hello",
					Retry: 1000 * time.Millisecond,
				},
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := Upgrade(r.Context(), w, testCase.opts...)
				require.NoError(t, err)
				defer conn.Close()

				// Send all events
				for _, event := range testCase.events {
					err := conn.SendEvent(r.Context(), event)
					require.NoError(t, err)
				}
			}))
			defer server.Close()

			// Make request to test server
			resp, err := http.Get(server.URL)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Verify headers
			assert.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
			assert.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
			assert.Equal(t, "keep-alive", resp.Header.Get("Connection"))

			// Create decoder to read events
			dec := NewDecoder(resp.Body)

			// Read and verify each event
			for _, want := range testCase.events {
				var got Event
				err := dec.Decode(&got)
				require.NoError(t, err)
				assert.Equal(t, want, &got)
			}
		})
	}
}

func TestSSEConnectionClose(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(r.Context(), w, WithRetryDelay(0), WithCloseMessage(&Event{Data: "goodbye"}))
		require.NoError(t, err)
		defer conn.Close()

		// Send a message
		err = conn.SendEvent(r.Context(), &Event{Data: "hello"})
		require.NoError(t, err)
	}))
	defer server.Close()

	// Make request to test server
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Create decoder to read events
	dec := NewDecoder(resp.Body)

	// Read and verify first event
	var firstEvent Event
	err = dec.Decode(&firstEvent)
	require.NoError(t, err)
	wantFirst := &Event{Data: "hello"}
	assert.Equal(t, wantFirst, &firstEvent)

	// Read and verify close message
	var closeEvent Event
	err = dec.Decode(&closeEvent)
	require.NoError(t, err)
	wantClose := &Event{Data: "goodbye"}
	assert.Equal(t, wantClose, &closeEvent)

	// Next read should return EOF
	var extraEvent Event
	err = dec.Decode(&extraEvent)
	assert.True(t, errors.Is(err, io.EOF))
}

func TestSSEContextCancellation(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		conn, err := Upgrade(ctx, w, WithRetryDelay(0))
		require.NoError(t, err)
		defer conn.Close()

		// Send a message
		err = conn.SendEvent(ctx, &Event{Data: "hello"})
		require.NoError(t, err)

		// Explicitly cancel the context
		cancel()

		// Try to send another message after context is cancelled
		err = conn.SendEvent(ctx, &Event{Data: "should not be sent"})
		assert.True(t, errors.Is(err, context.Canceled))
	}))
	defer server.Close()

	// Make request to test server
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Create decoder to read events
	dec := NewDecoder(resp.Body)

	// Read and verify first event
	var firstEvent Event
	err = dec.Decode(&firstEvent)
	require.NoError(t, err)
	wantFirst := &Event{Data: "hello"}
	assert.Equal(t, wantFirst, &firstEvent)

	// Next read should return EOF or ErrUnexpectedEOF
	var extraEvent Event
	err = dec.Decode(&extraEvent)
	assert.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
}

func TestConn_SendData(t *testing.T) {
	tests := []struct {
		name            string
		data            interface{}
		expected        string
		isValidationErr bool
	}{
		{
			name:     "simple string data",
			data:     "hello world",
			expected: "data: \"hello world\"\n\n",
		},
		{
			name:     "complex data type",
			data:     map[string]interface{}{"key": "value"},
			expected: "data: {\"key\":\"value\"}\n\n",
		},
		{
			name:            "raw data with newlines",
			data:            Raw("line1\nline2"),
			isValidationErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			conn, err := Upgrade(t.Context(), w, WithRetryDelay(0))
			require.NoError(t, err)
			defer conn.Close()

			err = conn.SendData(t.Context(), tt.data)

			if tt.isValidationErr {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, ErrValidation), "Expected a ValidationError but got: %v", err)
			} else {
				require.NoError(t, err)
				// Verify the exact output format
				assert.Equal(t, tt.expected, w.Body.String())
			}
		})
	}
}

func TestConn_SendComment(t *testing.T) {
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
			name:     "comment with newlines",
			comment:  "line1\nline2",
			expected: ": line1\nline2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			conn, err := Upgrade(t.Context(), w, WithRetryDelay(0))
			require.NoError(t, err)
			defer conn.Close()

			err = conn.SendComment(t.Context(), tt.comment)
			require.NoError(t, err)

			// Verify the exact output format
			assert.Equal(t, tt.expected, w.Body.String())
		})
	}
}

func TestConn_SendComment_ContextCanceled(t *testing.T) {
	w := httptest.NewRecorder()
	conn, err := Upgrade(t.Context(), w, WithRetryDelay(0))
	require.NoError(t, err)
	defer conn.Close()

	// Create a canceled context
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Try to send with canceled context
	err = conn.SendComment(ctx, "test")
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestConn_Heartbeat(t *testing.T) {
	w := httptest.NewRecorder()
	comment := "heartbeat-test"
	interval := 50 * time.Millisecond

	conn, err := Upgrade(t.Context(), w,
		WithHeartbeatInterval(interval),
		WithHeartbeatComment(comment),
		WithRetryDelay(0))
	require.NoError(t, err)

	// Let it run for just enough time to send at least one heartbeat
	time.Sleep(interval + 20*time.Millisecond)

	// Close the connection to stop heartbeats
	conn.Close()

	// Verify output contains the heartbeat comment
	output := w.Body.String()
	assert.Equal(t, ": "+comment+"\n", output)
}

// mockFlushWriter implements both io.Writer and http.Flusher interfaces
// but allows us to control the behavior for testing error paths
type mockFlushWriter struct {
	writeErr  error
	writeBuf  bytes.Buffer
	flushCall int
}

func (m *mockFlushWriter) Write(p []byte) (int, error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return m.writeBuf.Write(p)
}

func (m *mockFlushWriter) Flush() {
	m.flushCall++
	// We can't return an error from Flush as the interface doesn't support it
}

func TestConn_SendComment_EncoderError(t *testing.T) {
	// Create a writer that will fail on write
	failWriter := &mockFlushWriter{
		writeErr: errors.New("simulated write failure"),
	}

	// Create a connection with the failing writer
	conn := &Conn{
		enc:   NewEncoder(failWriter),
		flush: failWriter,
	}

	// Try to send a comment
	err := conn.SendComment(t.Context(), "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "simulated write failure")
}

func TestConn_RunHeartbeat_Error(t *testing.T) {
	// Create a writer that will fail on write
	failWriter := &mockFlushWriter{
		writeErr: errors.New("simulated write failure"),
	}

	// Create a connection with the failing writer
	conn := &Conn{
		enc:       NewEncoder(failWriter),
		flush:     failWriter,
		hbComment: "heartbeat",
		ctx:       t.Context(),
		closed:    make(chan struct{}),
	}

	// Instead of using a real ticker, use a channel we control
	tickChan := make(chan time.Time)

	// Run heartbeat in a goroutine
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() {
		conn.runHeartbeat(ctx, tickChan)
		// This goroutine should exit when the heartbeat fails
	}()

	// Manually simulate a tick
	tickChan <- time.Now()

	// Give a little time for the error to process
	time.Sleep(10 * time.Millisecond)

	// Cancel the context to make sure the test completes
	cancel()
}
