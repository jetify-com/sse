// Example demonstrating custom event types and structured data
package main

import (
	"log"
	"net/http"
	"time"

	"go.jetify.com/sse"
)

// StockUpdate represents a stock price update
type StockUpdate struct {
	Symbol string  `json:"symbol"`
	Price  float64 `json:"price"`
	Time   string  `json:"time"`
}

func main() {
	http.HandleFunc("/stocks", func(w http.ResponseWriter, r *http.Request) {
		conn, err := sse.Upgrade(r.Context(), w,
			sse.WithHeartbeatInterval(5*time.Second),
			sse.WithRetryDelay(2*time.Second),
		)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer func() { _ = conn.Close() }()

		// Simulate stock updates for different symbols
		symbols := []string{"AAPL", "GOOGL", "MSFT"}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				for _, symbol := range symbols {
					// Create a custom event with type "stock_update"
					event := &sse.Event{
						ID:    time.Now().Format(time.RFC3339Nano),
						Event: "stock_update",
						Data: StockUpdate{
							Symbol: symbol,
							Price:  float64(time.Now().Unix() % 1000), // Simulated price
							Time:   time.Now().Format(time.RFC3339),
						},
					}

					if err := conn.SendEvent(r.Context(), event); err != nil {
						log.Printf("Failed to send stock update: %v", err)
						return
					}
				}
			case <-r.Context().Done():
				// Send a close message when client disconnects
				_ = conn.SendEvent(r.Context(), &sse.Event{
					Event: "close",
					Data:  "Stream closed by client",
				})
				return
			}
		}
	})

	log.Println("Stock updates server starting on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
