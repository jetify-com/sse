// Package sse implements Server‑Sent Events (SSE).
//
// It is split into two parts:
//
//  1. Frame I/O — the Encoder and Decoder types turn Event structs
//     into the textual wire format defined by WHATWG HTML § 9.2 and back.
//     They can be used directly when you already have an io.Reader/io.Writer
//
//  2. Connection — the Conn type wraps an http.ResponseWriter after a
//     successful Upgrade(...) call.  It guarantees correct headers,
//     supports automatic heart‑beats, write deadlines, graceful close
//     messages, and is safe for concurrent use.
//
// # Specification compliance
//
//   - Accepts CR, LF, or CRLF line endings.
//   - UTF‑8 only; strips a single optional BOM.
//   - Differentiates io.EOF (clean finish) from io.ErrUnexpectedEOF (partial frame).
//   - Follows WHATWG rules for id/event/retry/data fields and ignores others.
//
// # Error handling
//
// Malformed input or impossible state is reported as *ValidationError.
// Transport problems bubble up unchanged so callers can distinguish network errors
// from application mistakes.
//
// This design means the server never writes bytes that we cannot later parse.
package sse
