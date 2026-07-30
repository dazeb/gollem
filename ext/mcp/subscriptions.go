package mcp

import (
	"encoding/json"
	"strconv"
	"sync"
	"sync/atomic"
)

// SubscriptionFilter selects which server notifications a client wants to
// receive over a subscriptions/listen stream. Every field is optional; a field
// left false means the client does not want that notification type.
type SubscriptionFilter struct {
	ToolsListChanged      bool     `json:"toolsListChanged,omitempty"`
	PromptsListChanged    bool     `json:"promptsListChanged,omitempty"`
	ResourcesListChanged  bool     `json:"resourcesListChanged,omitempty"`
	ResourceSubscriptions []string `json:"resourceSubscriptions,omitempty"`
}

// SubscribeParams is the params payload for subscriptions/listen.
type SubscribeParams struct {
	Notifications SubscriptionFilter `json:"notifications"`
}

// subscriptionStreamMarker is returned by the subscriptions/listen dispatch case
// to signal that the subscription stream owns the response; the caller MUST NOT
// write an inline JSON-RPC response. It is internal to the dispatch layer.
type subscriptionStreamMarker struct{}

// subscription is a single active subscriptions/listen stream. deliver pushes a
// fully-built JSON-RPC notification message (bytes) to the consumer; it must be
// non-blocking so the hub can fan out without holding its lock.
type subscription struct {
	id      any    // JSON-RPC id of the subscriptions/listen request (echoed to the client)
	key     string // server-generated unique hub key
	filter  SubscriptionFilter
	deliver func([]byte)
}

// subscriptionKeySeq generates process-unique subscription keys. The hub is
// shared across every stateless HTTP stream, and independent clients commonly
// reuse the same JSON-RPC request id (e.g. 1), so keying the hub by request id
// would let a second listener overwrite the first and let one stream's
// deregistration delete another. A monotonic server-generated key keeps each
// stream independent regardless of client-chosen ids.
var subscriptionKeySeq atomic.Uint64

func nextSubscriptionKey() string {
	return "sub-" + strconv.FormatUint(subscriptionKeySeq.Add(1), 10)
}

// subscriptionHub tracks active subscriptions/listen streams so server-side
// NotifyX methods can fan out notifications to the matching subscribers.
type subscriptionHub struct {
	mu   sync.Mutex
	subs map[string]*subscription
}

func newSubscriptionHub() *subscriptionHub {
	return &subscriptionHub{subs: make(map[string]*subscription)}
}

// register adds a subscription and returns a deregister func.
func (h *subscriptionHub) register(sub *subscription) func() {
	if sub.key == "" {
		sub.key = nextSubscriptionKey()
	}
	h.mu.Lock()
	h.subs[sub.key] = sub
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.subs, sub.key)
		h.mu.Unlock()
	}
}

// deregisterByID removes and returns the subscription whose JSON-RPC request id
// matches id. Used by the framed (stdio) cancel path, where the client
// references its own listen request id on a single connection. HTTP streams
// deregister via the connection-scoped closure returned by register instead.
func (h *subscriptionHub) deregisterByID(id any) *subscription {
	want := idKeyString(id)
	h.mu.Lock()
	defer h.mu.Unlock()
	for key, sub := range h.subs {
		if idKeyString(sub.id) == want {
			delete(h.subs, key)
			return sub
		}
	}
	return nil
}

// clear drops all subscriptions. Called on server close / peer EOF so a
// transport drop does not keep delivering to stale streams.
func (h *subscriptionHub) clear() {
	h.mu.Lock()
	h.subs = make(map[string]*subscription)
	h.mu.Unlock()
}

// emit builds a JSON-RPC notification for method with baseParams, injects
// _meta.io.modelcontextprotocol/subscriptionId per subscriber, and delivers to
// every subscriber whose match(filter) returns true. baseParams may be nil.
func (h *subscriptionHub) emit(method string, baseParams map[string]any, match func(SubscriptionFilter) bool) {
	if h == nil {
		return
	}
	h.mu.Lock()
	subs := make([]*subscription, 0, len(h.subs))
	for _, sub := range h.subs {
		if match != nil && !match(sub.filter) {
			continue
		}
		subs = append(subs, sub)
	}
	h.mu.Unlock()

	for _, sub := range subs {
		params := cloneParamsMap(baseParams)
		meta, _ := params["_meta"].(map[string]any)
		if meta == nil {
			meta = map[string]any{}
		}
		meta[MetaSubscriptionID] = sub.id
		params["_meta"] = meta
		msg := notificationMessage(method, params)
		if sub.deliver != nil {
			sub.deliver(msg)
		}
	}
}

// notificationMessage marshals a JSON-RPC notification (no id) with the given
// method and params. params may be nil.
func notificationMessage(method string, params map[string]any) []byte {
	req := struct {
		JSONRPC string         `json:"jsonrpc"`
		Method  string         `json:"method"`
		Params  map[string]any `json:"params,omitempty"`
	}{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(req)
	return data
}

// ackMessage builds the notifications/subscriptions/acknowledged message that
// the server MUST send as the first message on a subscription stream. The
// notifications field echoes the honored subset (here: the requested filter,
// since the server honors every requested type).
func ackMessage(id any, filter SubscriptionFilter) []byte {
	params := map[string]any{
		"notifications": filter,
		"_meta": map[string]any{
			MetaSubscriptionID: id,
		},
	}
	return notificationMessage("notifications/subscriptions/acknowledged", params)
}

// subscriptionClosureResponse builds the graceful-closure JSON-RPC response for
// the original subscriptions/listen request id. The server SHOULD send it
// before closing the stream itself.
func subscriptionClosureResponse(id any) []byte {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: mustRawJSON([]byte(`{"resultType":"complete","_meta":{"` +
			MetaSubscriptionID + `":` + string(mustRawJSON(marshalID(id))) + `}}`)),
	}
	data, _ := json.Marshal(resp)
	return data
}

// idKeyString returns a stable, unambiguous string key for a JSON-RPC id. int64
// and string ids that happen to share a textual form still map to distinct keys
// because the id is encoded as JSON first.
func idKeyString(id any) string {
	return string(mustRawJSON(marshalID(id)))
}

// marshalID converts a normalized id value to JSON bytes.
func marshalID(id any) []byte {
	if id == nil {
		return []byte("null")
	}
	if raw, ok := id.(json.RawMessage); ok {
		return append([]byte(nil), raw...)
	}
	data, _ := json.Marshal(id)
	return data
}

// parseSubscribeParams extracts the notifications filter from a
// subscriptions/listen request's params. A missing notifications object yields
// an empty filter (no notifications selected).
func parseSubscribeParams(raw json.RawMessage) SubscriptionFilter {
	if len(raw) == 0 {
		return SubscriptionFilter{}
	}
	var probe struct {
		Notifications SubscriptionFilter `json:"notifications"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return SubscriptionFilter{}
	}
	return probe.Notifications
}

// containsString reports whether s contains v.
func containsString(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
