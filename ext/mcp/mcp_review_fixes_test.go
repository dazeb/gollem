package mcp

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestNormalizeIDPreservesLargeIntegersVerbatim(t *testing.T) {
	// An integer beyond int64 must not be rounded through float64; it is echoed
	// back to the client verbatim so the response can be correlated.
	big := json.RawMessage("123456789012345678901234567890")
	got := normalizeID(&big)
	raw, ok := got.(json.RawMessage)
	if !ok {
		t.Fatalf("normalizeID(large int) = %T, want json.RawMessage", got)
	}
	if string(raw) != string(big) {
		t.Fatalf("normalizeID(large int) = %s, want %s", raw, big)
	}

	if got := normalizeID(ptrRaw(`1`)); got != int64(1) {
		t.Fatalf("normalizeID(1) = %#v, want int64(1)", got)
	}
	if got := normalizeID(ptrRaw(`"abc"`)); got != "abc" {
		t.Fatalf("normalizeID(\"abc\") = %#v, want \"abc\"", got)
	}
}

func ptrRaw(s string) *json.RawMessage {
	raw := json.RawMessage(s)
	return &raw
}

func TestSubscriptionHubKeepsClientsWithSameRequestIDIndependent(t *testing.T) {
	hub := newSubscriptionHub()

	var mu sync.Mutex
	counts := map[string]int{}
	newSub := func(name string) (*subscription, func()) {
		sub := &subscription{
			id:     int64(1), // both stateless clients reuse request id 1
			filter: SubscriptionFilter{ToolsListChanged: true},
			deliver: func([]byte) {
				mu.Lock()
				counts[name]++
				mu.Unlock()
			},
		}
		return sub, hub.register(sub)
	}

	_, deregA := newSub("a")
	_, deregB := newSub("b")

	hub.emit("notifications/tools/list_changed", nil, func(f SubscriptionFilter) bool {
		return f.ToolsListChanged
	})

	mu.Lock()
	if counts["a"] != 1 || counts["b"] != 1 {
		mu.Unlock()
		t.Fatalf("both same-id streams should receive one notification, got %v", counts)
	}
	mu.Unlock()

	// Deregistering one stream must not remove the other.
	deregA()
	hub.emit("notifications/tools/list_changed", nil, func(f SubscriptionFilter) bool {
		return f.ToolsListChanged
	})
	mu.Lock()
	if counts["a"] != 1 {
		mu.Unlock()
		t.Fatalf("deregistered stream a still delivered: %v", counts)
	}
	if counts["b"] != 2 {
		mu.Unlock()
		t.Fatalf("stream b should still be active, got %v", counts)
	}
	mu.Unlock()
	deregB()
}

func TestClientStateMRTRRoundsConfigurable(t *testing.T) {
	if s := newClientState(); s.maxMRTRRounds != defaultMaxMRTRRounds {
		t.Fatalf("default maxMRTRRounds = %d, want %d", s.maxMRTRRounds, defaultMaxMRTRRounds)
	}
	if s := newClientState(ClientConfig{MaxMRTRRounds: 20}); s.maxMRTRRounds != 20 {
		t.Fatalf("configured maxMRTRRounds = %d, want 20", s.maxMRTRRounds)
	}
	if s := newClientState(ClientConfig{MaxMRTRRounds: -5}); s.maxMRTRRounds != defaultMaxMRTRRounds {
		t.Fatalf("negative maxMRTRRounds = %d, want default %d", s.maxMRTRRounds, defaultMaxMRTRRounds)
	}
}
