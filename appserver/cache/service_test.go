package cache

import (
	"testing"

	"github.com/fugue-labs/gollem/modelutil"
)

func TestBenchmarkPassesReleaseGateForOpenAIAndAnthropic(t *testing.T) {
	service := NewService()
	result, err := service.Benchmark(BenchmarkParams{IncludeEvents: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Passed {
		t.Fatalf("benchmark failed: %#v", result)
	}
	if len(result.Providers) != 2 {
		t.Fatalf("providers = %#v, want openai and anthropic", result.Providers)
	}
	if result.Totals.HitRate < defaultBenchmarkTarget {
		t.Fatalf("total hit rate = %f, want >= %f", result.Totals.HitRate, defaultBenchmarkTarget)
	}
	for _, provider := range result.Providers {
		if provider.HitRate < defaultBenchmarkTarget {
			t.Fatalf("%s hit rate = %f, want >= %f", provider.Provider, provider.HitRate, defaultBenchmarkTarget)
		}
		if provider.Misses != int64(len(provider.Fixtures)) {
			t.Fatalf("%s misses = %d, want one miss per fixture", provider.Provider, provider.Misses)
		}
	}
	if len(result.Events) == 0 {
		t.Fatal("benchmark did not return typed events")
	}
	if result.Events[0].Type != modelutil.CacheEventMiss {
		t.Fatalf("first event = %s, want miss", result.Events[0].Type)
	}
}

func TestStatsIncludesBenchmarkEvents(t *testing.T) {
	service := NewService()
	result, err := service.Benchmark(BenchmarkParams{Providers: []string{ProviderOpenAI}})
	if err != nil {
		t.Fatal(err)
	}
	stats := service.Stats()
	if stats.TotalRequests != result.Totals.TotalRequests {
		t.Fatalf("stats total = %d, want %d", stats.TotalRequests, result.Totals.TotalRequests)
	}
	if stats.Hits != result.Totals.Hits || stats.Misses != result.Totals.Misses {
		t.Fatalf("stats = %#v, benchmark totals = %#v", stats, result.Totals)
	}
	if len(stats.Providers) != 1 || stats.Providers[0].Provider != ProviderOpenAI {
		t.Fatalf("provider stats = %#v", stats.Providers)
	}
	if len(stats.RecentEvents) != int(result.Totals.TotalRequests) {
		t.Fatalf("recent events = %d, want %d", len(stats.RecentEvents), result.Totals.TotalRequests)
	}
}

func TestBenchmarkRejectsUnsupportedProvider(t *testing.T) {
	service := NewService()
	if _, err := service.Benchmark(BenchmarkParams{Providers: []string{"other"}}); err == nil {
		t.Fatal("expected unsupported provider error")
	}
}
