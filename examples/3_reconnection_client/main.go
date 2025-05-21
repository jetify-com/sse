// Example of a robust SSE client with reconnection handling
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"go.jetify.com/sse"
)

func main() {
	ctx := context.Background()
	lastEventID := ""
	retryDelay := 3 * time.Second // Default retry delay

	for {
		if err := connectAndProcess(ctx, &lastEventID, &retryDelay); err != nil {
			if !errors.Is(err, io.EOF) {
				log.Printf("Connection error: %v, reconnecting in %v...", err, retryDelay)
				time.Sleep(retryDelay)
				continue
			}
			log.Println("Stream ended normally")
			break
		}
	}
}

func connectAndProcess(ctx context.Context, lastEventID *string, retryDelay *time.Duration) error {
	// Create request with Last-Event-ID if we have one
	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8080/stocks", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if *lastEventID != "" {
		req.Header.Set("Last-Event-ID", *lastEventID)
	}

	// Make the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	// Create decoder and process events
	decoder := sse.NewDecoder(resp.Body)

	for {
		var event sse.Event
		err := decoder.Decode(&event)
		if err != nil {
			return err
		}

		// Update reconnection state
		if event.ID != "" {
			*lastEventID = event.ID
		}
		if serverDelay := decoder.RetryDelay(); serverDelay > 0 {
			*retryDelay = serverDelay
			log.Printf("Server requested retry delay: %v", *retryDelay)
		}

		// Handle different event types
		switch event.Event {
		case "stock_update":
			fmt.Printf("Stock Update - ID: %s, Data: %v\n", event.ID, event.Data)
		case "close":
			fmt.Printf("Server message: %v\n", event.Data)
		default:
			fmt.Printf("Unknown event type: %s - Data: %v\n", event.Event, event.Data)
		}
	}
}
