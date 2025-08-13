// Basic SSE server example demonstrating minimal setup
package main

import (
	"log"
	"net/http"
	"time"

	"go.jetify.com/sse"
)

func main() {
	http.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		// Upgrade the connection to SSE
		conn, err := sse.Upgrade(r.Context(), w)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = conn.Close() }()

		// Send a simple text message every second
		for {
			select {
			case <-time.After(time.Second):
				if err := conn.SendData(r.Context(), "Hello SSE!"); err != nil {
					return
				}
			case <-r.Context().Done():
				return
			}
		}
	})

	log.Println("Basic SSE server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
