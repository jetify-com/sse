package sse

import (
	"net/http"
	"time"
)

type config struct {
	heartbeatInterval time.Duration
	heartbeatComment  string
	headers           http.Header
	status            int
	retryDelay        time.Duration
	writeTimeout      time.Duration
	closeMessage      *Event // Optional event to send when closing the connection
}

type Option func(*config)

// WithHeartbeatInterval configures the interval at which heartbeat comments are sent.
// Set to 0 to disable heartbeats. Heartbeats help keep the connection alive
// through proxies and load balancers that might otherwise timeout idle connections.
func WithHeartbeatInterval(d time.Duration) Option {
	return func(c *config) { c.heartbeatInterval = d }
}

// WithHeaders adds custom HTTP headers to the SSE response.
// These headers are sent along with the standard SSE headers.
func WithHeaders(h http.Header) Option { return func(c *config) { c.headers = h } }

// WithHeartbeatComment sets the content of heartbeat comments.
// These comments are sent at the interval specified by WithHeartbeatInterval.
// The default value is "keep-alive" if this is not specified.
func WithHeartbeatComment(s string) Option { return func(c *config) { c.heartbeatComment = s } }

// WithStatus sets the HTTP status code for the SSE response.
// Default is 200 OK. Some applications might use different status codes
// for specific purposes.
func WithStatus(status int) Option { return func(c *config) { c.status = status } }

// WithCloseMessage specifies an event to be sent when the connection is closed.
// This allows servers to send a final message to clients before terminating the stream.
func WithCloseMessage(event *Event) Option { return func(c *config) { c.closeMessage = event } }

// WithRetryDelay prepends a stand‑alone `retry:` field once right after the
// headers. This controls how long the client will wait before attempting to reconnect
// if the connection is lost.
//
// The value is sent to clients in milliseconds as per the SSE specification.
func WithRetryDelay(delay time.Duration) Option {
	return func(c *config) { c.retryDelay = delay }
}

// WithWriteTimeout sets the timeout for write operations when
// no deadline is specified in the context.
func WithWriteTimeout(d time.Duration) Option { return func(c *config) { c.writeTimeout = d } }

func defaultConfig() config {
	return config{
		heartbeatInterval: 15 * time.Second, // Common practice is ~15s to prevent proxy timeouts
		heartbeatComment:  "keep-alive",     // Standard term for connection maintenance
		headers:           http.Header{},    // Initialize empty header map
		status:            http.StatusOK,    // Default 200 status code
		retryDelay:        3 * time.Second,  // Default 3s retry delay
		writeTimeout:      5 * time.Second,  // Default write timeout
		closeMessage:      nil,              // No default close message
	}
}
