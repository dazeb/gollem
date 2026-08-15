package appserver

import (
	"context"
	"errors"
	"strings"
	"testing"

	appcache "github.com/fugue-labs/gollem/appserver/cache"
	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/modelutil"
)

func TestServerRuntimeRecordsLiveProviderCacheTelemetry(t *testing.T) {
	ctx := context.Background()
	st := newRuntimeTestStore(t)
	hit := core.TextResponse("cached answer")
	hit.Usage = core.Usage{InputTokens: 12, CacheReadTokens: 8, CacheWriteTokens: 2}
	miss := core.TextResponse("uncached answer")
	miss.Usage = core.Usage{InputTokens: 9}
	model := core.NewTestModel(hit, miss)
	cacheSvc := appcache.NewService()
	server := readyServer(
		WithStore(st),
		WithCache(cacheSvc),
		WithRuntimeService(NewRuntimeService(
			WithRuntimeModel(model, RuntimeModelInfo{ProviderID: "openai", Model: "gpt-test"}),
			WithRuntimeCacheEventHandler(cacheSvc.RecordLiveProviderEvent),
		)),
	)

	start := server.HandleRequest(ctx, request("thread/start", map[string]any{
		"prompt": "first cacheable prompt",
	}))
	if start.Error != nil {
		t.Fatalf("thread/start error: %v", start.Error)
	}
	var started struct {
		ThreadID string `json:"threadId"`
		Thread   struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	decodeResult(t, start, &started)
	waitForNotificationSet(t, server, "turn/completed")

	resume := server.HandleRequest(ctx, request("thread/resume", map[string]any{
		"threadId": started.Thread.ID,
		"prompt":   "second cacheable prompt",
	}))
	if resume.Error != nil {
		t.Fatalf("thread/resume error: %v", resume.Error)
	}
	waitForNotificationSet(t, server, "turn/completed")

	stats := cacheSvc.Stats()
	if stats.TotalRequests != 2 || stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("live cache stats = %#v, want one hit and one miss", stats)
	}
	if len(stats.Providers) != 1 || stats.Providers[0].Provider != "openai" {
		t.Fatalf("live cache providers = %#v", stats.Providers)
	}
	if len(stats.RecentEvents) != 2 {
		t.Fatalf("live cache events = %#v", stats.RecentEvents)
	}
	first := stats.RecentEvents[0]
	if first.Type != "cache.hit" || first.Source != "provider-usage" || first.CacheReadTokens != 8 || first.CacheWriteTokens != 2 {
		t.Fatalf("live cache hit event = %#v", first)
	}
	if len(first.Key) != 64 || strings.Contains(first.Key, "first cacheable prompt") {
		t.Fatalf("live cache key is not a safe stable fingerprint: %q", first.Key)
	}
	if stats.RecentEvents[1].Type != "cache.miss" || stats.RecentEvents[1].Source != "provider-usage" || stats.RecentEvents[1].CacheReadTokens != 0 {
		t.Fatalf("live cache miss event = %#v", stats.RecentEvents[1])
	}
}

func TestRuntimeCacheTelemetrySkipsIncompleteStream(t *testing.T) {
	var events []modelutil.CacheEvent
	model := newRuntimeCacheTelemetryModel(
		runtimeCacheIncompleteModel{},
		RuntimeModelInfo{ProviderID: "openai", Model: "gpt-test"},
		func(event modelutil.CacheEvent) { events = append(events, event) },
	)
	stream, err := model.RequestStream(context.Background(), nil, nil, nil)
	if err != nil {
		t.Fatalf("RequestStream: %v", err)
	}
	if _, err := stream.Next(); !errors.Is(err, context.Canceled) {
		t.Fatalf("stream error = %v, want context canceled", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("stream.Close: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("incomplete stream recorded cache events: %#v", events)
	}
}

type runtimeCacheIncompleteModel struct{}

func (runtimeCacheIncompleteModel) ModelName() string { return "gpt-test" }

func (runtimeCacheIncompleteModel) Request(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return nil, context.Canceled
}

func (runtimeCacheIncompleteModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return runtimeCacheIncompleteStream{}, nil
}

type runtimeCacheIncompleteStream struct{}

func (runtimeCacheIncompleteStream) Next() (core.ModelResponseStreamEvent, error) {
	return nil, context.Canceled
}

func (runtimeCacheIncompleteStream) Response() *core.ModelResponse { return nil }

func (runtimeCacheIncompleteStream) Usage() core.Usage { return core.Usage{} }

func (runtimeCacheIncompleteStream) Close() error { return nil }
