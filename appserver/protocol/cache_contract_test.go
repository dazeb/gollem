package protocol

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCacheStatsContractIsGeneratedAndBound(t *testing.T) {
	assertBinding(t, WireTypeBindings(), "cache/stats", SurfaceGollemExtension, "CacheStatsResponse")

	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{"CacheEventType", "CacheEvent", "CacheProviderStats", "CacheStatsResponse"} {
		if _, ok := defs[name]; !ok {
			t.Errorf("$defs missing %s", name)
		}
	}
	event := defs["CacheEvent"].(Schema)
	eventType := event["properties"].(Schema)["type"].(Schema)
	if got, want := eventType["enum"], []any{"cache.hit", "cache.miss"}; !sameSchemaValues(got, want) {
		t.Errorf("CacheEvent.type enum = %#v, want %#v", got, want)
	}

	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	source := string(generated)
	for _, want := range []string{
		`"cache/stats": undefined;`,
		`"cache/stats": CacheStatsResponse;`,
		"export type CacheStatsResponse = {\n",
		`"providers": Array<CacheProviderStats>;`,
		`"recentEvents"?: Array<CacheEvent>;`,
		"export type CacheEvent = {\n",
		`"key": string;`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated TypeScript missing %q", want)
		}
	}
}

func sameSchemaValues(got any, want []any) bool {
	values, ok := got.([]any)
	if !ok || len(values) != len(want) {
		return false
	}
	for index := range want {
		if values[index] != want[index] {
			return false
		}
	}
	return true
}

func TestCacheStatsResponseRoundTripsExactWireShape(t *testing.T) {
	now := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	response := CacheStatsResponse{
		TotalRequests: 2,
		Hits:          1,
		Misses:        1,
		HitRate:       0.5,
		Providers: []CacheProviderStats{{
			Provider:      "openai",
			TotalRequests: 2,
			Hits:          1,
			Misses:        1,
			HitRate:       0.5,
		}},
		RecentEvents: []CacheEvent{{
			Type:             CacheEventHit,
			Provider:         "openai",
			Model:            "gpt-test",
			Key:              "0123456789abcdef",
			Source:           "provider-usage",
			CacheReadTokens:  8,
			CacheWriteTokens: 2,
			At:               now,
		}},
	}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(encoded), `"key":"0123456789abcdef"`) {
		t.Fatalf("encoded response lost cache key: %s", encoded)
	}
	var decoded CacheStatsResponse
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := decoded.RecentEvents[0]; got != response.RecentEvents[0] {
		t.Fatalf("event = %#v, want %#v", got, response.RecentEvents[0])
	}
}
