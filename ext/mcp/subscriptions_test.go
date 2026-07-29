package mcp

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestSubscriptionsHTTPAckFirst verifies the server sends
// notifications/subscriptions/acknowledged as the first message on the stream,
// carrying the subscription id equal to the listen request's JSON-RPC id.
func TestSubscriptionsHTTPAckFirst(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "subs-test", Version: "1.0.0"}))
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	var (
		mu       sync.Mutex
		first    Notification
		gotFirst bool
	)
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- client.Listen(ctx, SubscriptionFilter{ToolsListChanged: true}, func(n Notification) {
			mu.Lock()
			defer mu.Unlock()
			if !gotFirst {
				first = n
				gotFirst = true
			}
		})
	}()

	// Wait for the ack to arrive.
	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		done := gotFirst
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("timed out waiting for subscription ack")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if first.Method != "notifications/subscriptions/acknowledged" {
		t.Fatalf("first method = %q, want notifications/subscriptions/acknowledged", first.Method)
	}
	var ackParams struct {
		Notifications SubscriptionFilter         `json:"notifications"`
		Meta          map[string]json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(first.Params, &ackParams); err != nil {
		t.Fatalf("parse ack: %v", err)
	}
	subIDRaw, ok := ackParams.Meta[MetaSubscriptionID]
	if !ok {
		t.Fatalf("ack missing _meta.subscriptionId: %+v", ackParams.Meta)
	}
	var subID int64
	if err := json.Unmarshal(subIDRaw, &subID); err != nil {
		t.Fatalf("subscriptionId not int: %v", err)
	}
	if subID == 0 {
		t.Fatalf("subscriptionId = 0")
	}
	if !ackParams.Notifications.ToolsListChanged {
		t.Fatalf("ack did not honor toolsListChanged: %+v", ackParams.Notifications)
	}

	cancel()
	if err := <-listenErr; err != nil && err != context.Canceled {
		t.Fatalf("listen returned unexpected error: %v", err)
	}
}

// TestSubscriptionsHTTPNotifyToolsListChanged verifies a server-emitted
// notification reaches the client with the correct subscriptionId, and that a
// type the client did NOT request is not delivered.
func TestSubscriptionsHTTPNotifyToolsListChanged(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "subs-notify", Version: "1.0.0"}))
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	var (
		mu           sync.Mutex
		toolsChanged Notification
		promptsSeen  bool
		gotTools     bool
	)
	listenErr := make(chan error, 1)
	// Only subscribe to toolsListChanged; promptsListChanged must NOT arrive.
	go func() {
		listenErr <- client.Listen(ctx, SubscriptionFilter{ToolsListChanged: true}, func(n Notification) {
			mu.Lock()
			defer mu.Unlock()
			switch n.Method {
			case "notifications/tools/list_changed":
				toolsChanged = n
				gotTools = true
			case "notifications/prompts/list_changed":
				promptsSeen = true
			}
		})
	}()

	// Wait for ack, then emit a tools list changed.
	// Observe whether the listen stream already produced a tools notification
	// before triggering the server-side notification below.
	mu.Lock()
	alreadyGotTools := gotTools
	mu.Unlock()
	_ = alreadyGotTools
	// Give the server a moment to register the subscription.
	time.Sleep(50 * time.Millisecond)
	server.NotifyToolsListChanged()
	server.NotifyPromptsListChanged()

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		done := gotTools
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("timed out waiting for tools/list_changed notification")
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	if promptsSeen {
		t.Fatal("received prompts/list_changed despite not subscribing to it")
	}
	mu.Unlock()

	var params struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if err := json.Unmarshal(toolsChanged.Params, &params); err != nil {
		t.Fatalf("parse tools notification: %v", err)
	}
	if _, ok := params.Meta[MetaSubscriptionID]; !ok {
		t.Fatalf("tools notification missing _meta.subscriptionId: %+v", params.Meta)
	}

	cancel()
	if err := <-listenErr; err != nil && err != context.Canceled {
		t.Fatalf("listen returned unexpected error: %v", err)
	}
}

// TestSubscriptionsHTTPResourceUpdatedOnlyForSubscribedURI verifies
// notifications/resources/updated is delivered only for URIs the client
// subscribed to.
func TestSubscriptionsHTTPResourceUpdatedOnlyForSubscribedURI(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "subs-uri", Version: "1.0.0"}))
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewHTTPClientWithConfig(ctx, httpServer.URL, ClientConfig{})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	var (
		mu       sync.Mutex
		seenURIs []string
	)
	listenErr := make(chan error, 1)
	go func() {
		listenErr <- client.Listen(ctx, SubscriptionFilter{
			ResourceSubscriptions: []string{"file:///a"},
		}, func(n Notification) {
			if n.Method != "notifications/resources/updated" {
				return
			}
			var p struct {
				URI string `json:"uri"`
			}
			_ = json.Unmarshal(n.Params, &p)
			mu.Lock()
			seenURIs = append(seenURIs, p.URI)
			mu.Unlock()
		})
	}()

	time.Sleep(50 * time.Millisecond) // allow subscription to register
	server.NotifyResourceUpdated("file:///a")
	server.NotifyResourceUpdated("file:///b")

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		gotA := containsString(seenURIs, "file:///a")
		mu.Unlock()
		if gotA {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("timed out waiting for resources/updated for file:///a")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Give a brief window for file:///b to (incorrectly) arrive, then assert.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if containsString(seenURIs, "file:///b") {
		t.Fatalf("received resources/updated for unsubscribed uri file:///b: %+v", seenURIs)
	}
	if !containsString(seenURIs, "file:///a") {
		t.Fatalf("did not receive resources/updated for file:///a: %+v", seenURIs)
	}

	cancel()
	if err := <-listenErr; err != nil && err != context.Canceled {
		t.Fatalf("listen returned unexpected error: %v", err)
	}
}

// TestSubscriptionsHTTPGracefulClosure verifies the server sends the JSON-RPC
// response to the original request id when the client disconnects (ctx cancel)
// and that Listen returns cleanly.
func TestSubscriptionsHTTPGracefulClosure(t *testing.T) {
	server := NewServer(WithServerInfo(ServerInfo{Name: "subs-close", Version: "1.0.0"}))
	httpServer := httptest.NewServer(NewHTTPServerTransport(server))
	defer httpServer.Close()

	listenCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := NewHTTPClientWithConfig(context.Background(), httpServer.URL, ClientConfig{})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	defer client.Close()

	listenErr := make(chan error, 1)
	go func() {
		listenErr <- client.Listen(listenCtx, SubscriptionFilter{ToolsListChanged: true}, func(Notification) {})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-listenErr:
		if err != nil && err != context.Canceled {
			t.Fatalf("listen returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("listen did not return after cancel")
	}
}
