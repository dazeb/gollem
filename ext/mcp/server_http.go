package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// HTTPServerTransport serves MCP over the stateless streamable HTTP transport.
// The 2026-07-28 protocol has no sessions: each POST carries a single
// self-describing JSON-RPC request and returns the result inline as
// application/json. There is no Mcp-Session-Id, no DELETE teardown, and no SSE
// resumability. The one long-lived stream is subscriptions/listen, which is
// delivered as a POST whose response is text/event-stream (SSE); it replaces
// the old GET endpoint.
type HTTPServerTransport struct {
	template *Server
}

// NewHTTPServerTransport binds a reusable Server template to an HTTP transport.
func NewHTTPServerTransport(server *Server) *HTTPServerTransport {
	if server == nil {
		server = NewServer()
	}
	return &HTTPServerTransport{template: server}
}

// Run blocks until ctx is cancelled. The stateless transport holds no
// per-connection resources to clean up.
func (t *HTTPServerTransport) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// Close is a no-op for the stateless transport; retained for io.Closer.
func (t *HTTPServerTransport) Close() error { return nil }

// ServeHTTP implements http.Handler for the stateless streamable HTTP transport.
func (t *HTTPServerTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		t.handleGet(w, r)
	case http.MethodPost:
		t.handlePost(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGet retains the subscriptions seam for backward compatibility. The
// 2026-07-28 protocol routes subscriptions/listen through POST (see
// handlePost), so GET returns 501.
func (t *HTTPServerTransport) handleGet(w http.ResponseWriter, r *http.Request) {
	_ = r
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte(`{"jsonrpc":"2.0","error":{"code":-32601,"message":"subscriptions/listen is delivered via POST; GET is not supported"}}`))
}

func (t *HTTPServerTransport) handlePost(w http.ResponseWriter, r *http.Request) {
	var msg jsonRPCMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		writeHTTPJSONRPCError(w, nil, http.StatusBadRequest, &jsonRPCError{
			Code:    jsonRPCCodeParseError,
			Message: "invalid JSON: " + err.Error(),
		})
		return
	}

	// Validate MCP-Protocol-Version header when present.
	if pv := r.Header.Get("MCP-Protocol-Version"); pv != "" && !isSupportedProtocolVersion(pv) {
		writeHTTPJSONRPCError(w, normalizeID(msg.ID), http.StatusBadRequest, UnsupportedProtocolVersionError(pv))
		return
	}

	// Validate Mcp-Method header when present: must match the JSON-RPC body method.
	if hMethod := r.Header.Get("Mcp-Method"); hMethod != "" && hMethod != msg.Method {
		writeHTTPJSONRPCError(w, normalizeID(msg.ID), http.StatusBadRequest, HeaderMismatchError(
			"Mcp-Method header does not match request method"))
		return
	}

	// Validate Mcp-Name header when present: must match the tool/prompt name or
	// resource uri in the request params.
	if hName := r.Header.Get("Mcp-Name"); hName != "" {
		if expected := mcpNameForRequest(msg.Method, msg.Params); expected != "" && expected != hName {
			writeHTTPJSONRPCError(w, normalizeID(msg.ID), http.StatusBadRequest, HeaderMismatchError(
				"Mcp-Name header does not match request target"))
			return
		}
	}

	// subscriptions/listen is the one long-lived stream: switch the response to
	// text/event-stream and stream notifications until the client disconnects
	// or the server shuts down.
	if msg.Method == "subscriptions/listen" {
		t.handleSubscriptionsListen(w, r, &msg)
		return
	}

	headers := collectXMCPHeaders(r.Header)
	ctx := r.Context()
	resp := t.template.HandleRequestSync(ctx, &msg, headers)

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(resp)
}

// handleSubscriptionsListen streams a subscriptions/listen response as SSE. It
// validates _meta (returning the normal JSON-RPC error response on failure),
// sends notifications/subscriptions/acknowledged as the first event, then fans
// out server notifications to the stream until the client disconnects or the
// server shuts down. On graceful closure it emits the JSON-RPC response to the
// original request id before returning. No SSE event ids / Last-Event-ID are
// emitted (no resumability).
func (t *HTTPServerTransport) handleSubscriptionsListen(w http.ResponseWriter, r *http.Request, msg *jsonRPCMessage) {
	requestID := normalizeID(msg.ID)

	meta, metaErr := parseRequestMeta(msg.Params)
	if metaErr != nil {
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: metaErr}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}

	filter := parseSubscribeParams(msg.Params)

	flusher, ok := w.(http.Flusher)
	if !ok {
		resp := jsonRPCResponse{JSONRPC: "2.0", ID: requestID, Error: &jsonRPCError{
			Code:    jsonRPCCodeInternalError,
			Message: "mcp: streaming unsupported by response writer",
		}}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(data)
		return
	}

	// Buffered channel: the ack is enqueued first so it is always the first
	// event, before any notification that arrives after registration.
	ch := make(chan []byte, 256)
	ackBytes := ackMessage(requestID, filter)
	select {
	case ch <- ackBytes:
	default:
	}

	sub := &subscription{
		id:     requestID,
		idKey:  idKeyString(requestID),
		filter: filter,
		deliver: func(data []byte) {
			select {
			case ch <- data:
			default:
				// Client is slow; drop the notification rather than block the hub.
			}
		},
	}
	deregister := t.template.hub.register(sub)
	defer deregister()

	_ = meta // client capabilities already validated; no further use needed here

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case data := <-ch:
			if _, err := writeSSEEvent(w, data); err != nil {
				return
			}
			flusher.Flush()
		case <-ctx.Done():
			// Graceful closure: emit the JSON-RPC response to the original id,
			// then close the stream.
			_, _ = writeSSEEvent(w, subscriptionClosureResponse(requestID))
			flusher.Flush()
			return
		}
	}
}

// writeSSEEvent writes a single SSE message event with the given JSON payload.
func writeSSEEvent(w http.ResponseWriter, data []byte) (int, error) {
	return w.Write(append(append([]byte("event: message\ndata: "), data...), '\n', '\n'))
}

// collectXMCPHeaders returns the x-mcp-* request headers (lowercased keys) so
// tool handlers can read SEP-2243 custom headers passed from tool params via
// RequestContext.XHeader.
func collectXMCPHeaders(header http.Header) map[string]string {
	out := make(map[string]string)
	for key, values := range header {
		if !strings.HasPrefix(strings.ToLower(key), "x-mcp-") {
			continue
		}
		if len(values) == 0 {
			continue
		}
		out[strings.ToLower(key)] = values[0]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func writeHTTPJSONRPCError(w http.ResponseWriter, id any, status int, rpcErr *jsonRPCError) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   rpcErr,
	}
	data, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}
