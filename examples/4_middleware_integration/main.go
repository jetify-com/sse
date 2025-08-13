// Example demonstrating SSE integration with HTTP middleware
package main

import (
	"log"
	"net/http"
	"time"

	"go.jetify.com/sse"
)

// customResponseWriter wraps http.ResponseWriter to add functionality
type customResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *customResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap implements responseWriterUnwrapper for proper SSE handling
func (w *customResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// loggingMiddleware logs request details and response status
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap the response writer
		wrapped := &customResponseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		// Call the next handler
		next(wrapped, r)

		// Log the request details
		log.Printf(
			"Method: %s, Path: %s, Status: %d, Duration: %v",
			r.Method,
			r.URL.Path,
			wrapped.statusCode,
			time.Since(start),
		)
	}
}

func eventsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := sse.Upgrade(r.Context(), w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = conn.Close() }()

	// Send numbered events
	count := 0
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := conn.SendData(r.Context(), count); err != nil {
				log.Printf("Failed to send event: %v", err)
				return
			}
			count++
		case <-r.Context().Done():
			return
		}
	}
}

func main() {
	// Apply middleware to the handler
	http.HandleFunc("/events", loggingMiddleware(eventsHandler))

	log.Println("SSE server with middleware starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
