package sse

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Package-level variable for time access - can be replaced in tests
var now = func() time.Time {
	return time.Now()
}

//──────────────────────────────────────────────────────────────────────────────
// Upgrade — performs headers + flush and returns a *Conn
//──────────────────────────────────────────────────────────────────────────────

// Upgrade switches the HTTP connection to an SSE stream and returns a *Conn
// for sending events. The returned Conn is safe for concurrent use.
//
//	conn, err := sse.Upgrade(r.Context(), w)
//	if err != nil { … }
//
// It automatically closes when the client disconnects, but you may call
// Close() yourself to stop heartbeats promptly.
func Upgrade(ctx context.Context, w http.ResponseWriter, opts ...Option) (*Conn, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	// Discover flusher & deadline.
	flush, dl := unwrapResponseWriter(w)
	if flush == nil {
		return nil, fmt.Errorf("sse: ResponseWriter lacks http.Flusher")
	}

	// Mandatory headers.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	for k, vv := range cfg.headers {
		for _, v := range vv {
			h.Add(k, v)
		}
	}

	w.WriteHeader(cfg.status)

	// Create encoder for consistent output
	enc := NewEncoder(w)

	// Send the retry delay if configured
	if cfg.retryDelay > 0 {
		retryEvent := &Event{
			Retry: cfg.retryDelay,
		}
		if err := enc.EncodeEvent(retryEvent); err != nil {
			return nil, fmt.Errorf("sse: failed to encode retry: %w", err)
		}
	}

	flush.Flush()

	// Build connection.
	c := &Conn{
		enc:          enc,
		flush:        flush,
		dl:           dl,
		mu:           sync.Mutex{},
		ctx:          ctx,
		closed:       make(chan struct{}),
		closeMessage: cfg.closeMessage,
		writeTimeout: cfg.writeTimeout,
	}

	if cfg.heartbeatInterval > 0 {
		c.ticker = time.NewTicker(cfg.heartbeatInterval)
		c.hbComment = cfg.heartbeatComment
		go c.heartbeatLoop()
	}

	// Auto-close when client disconnects.
	go func() {
		<-c.ctx.Done()
		_ = c.Close()
	}()

	return c, nil
}

// unwrapResponseWriter extracts the HTTP interfaces we need from a ResponseWriter.
// It iteratively unwraps the ResponseWriter to find the innermost implementations
// of http.Flusher and writeDeadliner.
func unwrapResponseWriter(w http.ResponseWriter) (flush http.Flusher, dl writeDeadliner) {
	currentWriter := w

	for currentWriter != nil {
		// Check for http.Flusher
		if f, ok := currentWriter.(http.Flusher); ok {
			flush = f // Found a Flusher, might be overwritten by a deeper one
		}

		// Check for writeDeadliner
		if d, ok := currentWriter.(writeDeadliner); ok {
			dl = d // Found a Deadliner, might be overwritten by a deeper one
		}

		// Attempt to unwrap further
		unwrapper, ok := currentWriter.(responseWriterUnwrapper)
		if !ok {
			// Not an unwrapper, so this is as deep as we can go
			break
		}

		nextWriter := unwrapper.Unwrap()

		// If Unwrap() returns the same writer, break to prevent an infinite loop.
		// If nextWriter is nil, the loop condition (currentWriter != nil)
		// will handle termination when currentWriter is updated.
		if nextWriter == currentWriter {
			break
		}
		currentWriter = nextWriter
	}

	return flush, dl
}

//──────────────────────────────────────────────────────────────────────────────
// Conn shared type
//──────────────────────────────────────────────────────────────────────────────

type Conn struct {
	enc          *Encoder
	flush        http.Flusher
	dl           writeDeadliner
	mu           sync.Mutex
	ticker       *time.Ticker
	hbComment    string
	ctx          context.Context
	closed       chan struct{}
	closeMessage *Event // Optional event to send when closing
	writeTimeout time.Duration
}

// getDeadline extracts a deadline from the context or returns a default deadline
func getDeadline(ctx context.Context, defaultTimeout time.Duration) time.Time {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = now().Add(defaultTimeout)
	}
	return deadline
}

// getConnDeadline gets the deadline for this connection
func (c *Conn) getConnDeadline(ctx context.Context) time.Time {
	return getDeadline(ctx, c.writeTimeout)
}

// encodeAndFlush encodes an event and flushes the response
func (c *Conn) encodeAndFlush(e *Event) error {
	if err := c.enc.EncodeEvent(e); err != nil {
		return err
	}
	c.flush.Flush()
	return nil
}

// encodeAndFlushComment encodes a comment and flushes the response
func (c *Conn) encodeAndFlushComment(comment string) error {
	if err := c.enc.EncodeComment(comment); err != nil {
		return err
	}
	c.flush.Flush()
	return nil
}

// SendEvent sends a fully customized Event to the client
func (c *Conn) SendEvent(ctx context.Context, e *Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.closed:
		return context.Canceled
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.dl != nil {
		_ = c.dl.SetWriteDeadline(c.getConnDeadline(ctx))
	}

	return c.encodeAndFlush(e)
}

// SendData sends a simple data-only message with event type "message"
func (c *Conn) SendData(ctx context.Context, v any) error {
	return c.SendEvent(ctx, &Event{Data: v})
}

// SendComment sends a comment line to the client
// Comments are useful as keepalives to prevent connection timeouts
func (c *Conn) SendComment(ctx context.Context, t string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.closed:
		return context.Canceled
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if c.dl != nil {
		_ = c.dl.SetWriteDeadline(c.getConnDeadline(ctx))
	}

	return c.encodeAndFlushComment(t)
}

// sendHeartbeat sends a heartbeat comment to keep the connection alive
func (c *Conn) sendHeartbeat(ctx context.Context) error {
	return c.SendComment(ctx, c.hbComment)
}

// sendCloseMessage sends the configured close message before closing the connection
func (c *Conn) sendCloseMessage(ctx context.Context) {
	if c.closeMessage == nil {
		return
	}

	if c.dl != nil {
		_ = c.dl.SetWriteDeadline(c.getConnDeadline(ctx))
	}

	// Try to send the close message, ignore errors
	_ = c.encodeAndFlush(c.closeMessage)
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	select {
	case <-c.closed:
		// Already closed, return immediately
		return nil
	default:
		// If we have a close message, try to send it before closing
		if c.closeMessage != nil {
			ctx, cancel := context.WithTimeout(context.Background(), c.writeTimeout)
			c.sendCloseMessage(ctx)
			cancel()
		}

		// Mark as closed, then perform cleanup
		close(c.closed)
		if c.ticker != nil {
			c.ticker.Stop()
		}
		return nil
	}
}

// runHeartbeat runs the heartbeat loop with the provided ticker channel
// This method is extracted to make testing easier
func (c *Conn) runHeartbeat(ctx context.Context, tickerC <-chan time.Time) {
	for {
		select {
		case <-tickerC:
			hbCtx, cancel := context.WithTimeout(ctx, c.writeTimeout)
			if err := c.sendHeartbeat(hbCtx); err != nil {
				cancel()
				_ = c.Close()
				return
			}
			cancel()
		case <-c.closed:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (c *Conn) heartbeatLoop() {
	c.runHeartbeat(c.ctx, c.ticker.C)
}

// writeDeadliner mirrors the interface in net.Conn but avoids importing net.
type writeDeadliner interface{ SetWriteDeadline(time.Time) error }

// responseWriterUnwrapper represents a ResponseWriter that wraps another ResponseWriter.
// This is commonly used by HTTP middleware to add functionality to the ResponseWriter.
type responseWriterUnwrapper interface{ Unwrap() http.ResponseWriter }
