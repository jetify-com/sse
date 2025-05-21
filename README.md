# sse – Server‑Sent Events for Go

### A tiny, dependency‑free package that makes it easy to use SSE streaming with any HTTP framework.

[![Version](https://img.shields.io/github/v/release/jetify-com/sse?color=green&label=version&sort=semver)](https://github.com/jetify-com/sse/releases)
[![Go Reference](https://pkg.go.dev/badge/go.jetify.com/sse)](https://pkg.go.dev/go.jetify.com/sse)
[![License](https://img.shields.io/github/license/jetify-com/sse)]()
[![Join Discord](https://img.shields.io/discord/903306922852245526?color=7389D8&label=discord&logo=discord&logoColor=ffffff&cacheSeconds=1800)](https://discord.gg/jetify)

*Primary Author(s)*: [Daniel Loreto](https://github.com/loreto)

---

## ✨ Features

* **Spec‑compliant**: Follows the [official WHATWG spec](https://html.spec.whatwg.org/multipage/server-sent-events.html) for server-side events.
* **Zero dependencies**: Depends only on the go standard library.
* **Well-tested** Comprehensive test suite with >90% coverage, including end-to-end tests.
* **Framework Agnostic**: Designed to be used with any Go HTTP framework.
* **Flexible Encoding**: Supports both JSON and raw data encoding.

## Quick Start

### Server (Sending Events)

```go
package main

import (
    "fmt"
    "log"
    "net/http"
    "time"
    
    "go.jetify.com/sse"
)

func eventsHandler(w http.ResponseWriter, r *http.Request) {
    // Upgrade HTTP connection to SSE
    conn, err := sse.Upgrade(r.Context(), w)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer conn.Close()
    
    // Send events until client disconnects
    count := 0
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            // Send a simple event with JSON data
            event := &sse.Event{
                ID:    fmt.Sprintf("%d", count),
                Data:  map[string]any{"count": count, "time": time.Now().Format(time.RFC3339)},
            }
            
            if err := conn.SendEvent(r.Context(), event); err != nil {
                log.Printf("Error: %v", err)
                return
            }
            count++
            
        case <-r.Context().Done():
            return // Client disconnected
        }
    }
}

func main() {
    http.HandleFunc("/events", eventsHandler)
    log.Println("SSE server starting on :8080")
    log.Fatal(http.ListenAndServe(":8080", nil))
}
```

### Client (Receiving Events)

```go
package main

import (
    "context"
    "fmt"
    "log"
    "net/http"
    
    "go.jetify.com/sse"
)

func main() {
    // Make request to SSE endpoint
    resp, err := http.Get("http://localhost:8080/events")
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer resp.Body.Close()
    
    // Create decoder for the SSE stream
    decoder := sse.NewDecoder(resp.Body)
    
    // Process events as they arrive
    for {
        var event sse.Event
        err := decoder.Decode(&event)
        if err != nil {
            log.Printf("Stream ended: %v", err)
            break
        }
        
        fmt.Printf("Event ID: %s, Data: %v\n", event.ID, event.Data)
    }
}
```

## Documentation

The following dcumentation is available:

* **[API Reference](https://pkg.go.dev/go.jetify.com/sse)** - Complete Go package documentation
* **[Examples](examples/)** - Real-world usage patterns

## Community & Support

Join our community and get help:

* **Discord** – [https://discord.gg/jetify](https://discord.gg/jetify) (best for quick questions & showcase)
* **GitHub Discussions** – [Discussions](https://github.com/jetify-com/sse/discussions) (best for ideas & design questions)
* **Issues** – [Bug reports & feature requests](https://github.com/jetify-com/sse/issues)

## Contributing

We 💖 contributions! Please read [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Licensed under the **Apache 2.0 License** – see [LICENSE](LICENSE) for details.