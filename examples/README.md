# SSE Examples

This directory contains example applications demonstrating various features of the SSE package.

## Examples Overview

### [1. Basic Server](1_basic_server/main.go)
A minimal SSE server implementation showing:
- Basic connection setup
- Simple text message sending
- Context handling for client disconnection
- [View Source](1_basic_server/main.go)

### [2. Custom Events](2_custom_events/main.go)
Demonstrates advanced event features:
- Custom event types
- Structured JSON data
- Heartbeat configuration
- Close message handling
- Multiple event streams
- [View Source](2_custom_events/main.go)

### [3. Reconnection Client](3_reconnection_client/main.go)
Shows how to build a robust SSE client with:
- Proper reconnection handling
- Last-Event-ID tracking
- Server retry delay handling
- Different event type handling
- Error management
- [View Source](3_reconnection_client/main.go)

### [4. Middleware Integration](4_middleware_integration/main.go)
Illustrates integration with HTTP middleware:
- Custom response writer implementation
- Request logging middleware
- Proper response writer unwrapping
- Status code tracking
- [View Source](4_middleware_integration/main.go)

## Running the Examples

Each example can be run independently. Start the server examples with:

```bash
# Run the basic server example
go run ./1_basic_server/main.go

# Run the custom events example
go run ./2_custom_events/main.go

# Run the middleware example
go run ./4_middleware_integration/main.go
```

For the client example, first start one of the server examples, then in another terminal:

```bash
go run ./3_reconnection_client/main.go
```

You can also test the servers using curl:

```bash
# Test the basic server and middleware examples
curl -N http://localhost:8080/events

# Test the custom events example
curl -N http://localhost:8080/stocks
```

## Example Features Matrix

| Example | Source | JSON Data | Custom Events | Reconnection | Middleware | Heartbeat |
|---------|--------|-----------|---------------|--------------|------------|-----------|
| Basic Server | [View](1_basic_server/main.go) | ❌ | ❌ | ❌ | ❌ | ❌ |
| Custom Events | [View](2_custom_events/main.go) | ✅ | ✅ | ❌ | ❌ | ✅ |
| Reconnection Client | [View](3_reconnection_client/main.go) | ✅ | ✅ | ✅ | ❌ | ❌ |
| Middleware Integration | [View](4_middleware_integration/main.go) | ❌ | ❌ | ❌ | ✅ | ❌ | 