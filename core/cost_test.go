package core

import (
	"context"
	"math"
	"sync"
	"testing"
)

func TestCostTracker_Record(t *testing.T) {
	pricing := map[string]ModelPricing{
		"test-model": {InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
	}
	tracker := NewCostTracker(pricing)

	usage := RunUsage{}
	usage.InputTokens = 1000
	usage.OutputTokens = 500
	tracker.Record("test-model", usage)

	expected := 1000*0.000003 + 500*0.000015
	if math.Abs(tracker.TotalCost()-expected) > 0.0001 {
		t.Errorf("expected cost %f, got %f", expected, tracker.TotalCost())
	}
}

func TestCostTracker_Breakdown(t *testing.T) {
	pricing := map[string]ModelPricing{
		"model-a": {InputTokenCost: 0.000001, OutputTokenCost: 0.000002},
		"model-b": {InputTokenCost: 0.000010, OutputTokenCost: 0.000020},
	}
	tracker := NewCostTracker(pricing)

	usageA := RunUsage{}
	usageA.InputTokens = 100
	usageA.OutputTokens = 50
	tracker.Record("model-a", usageA)

	usageB := RunUsage{}
	usageB.InputTokens = 200
	usageB.OutputTokens = 100
	tracker.Record("model-b", usageB)

	breakdown := tracker.CostBreakdown()
	if len(breakdown) != 2 {
		t.Errorf("expected 2 models in breakdown, got %d", len(breakdown))
	}
	if breakdown["model-a"] <= 0 {
		t.Error("expected positive cost for model-a")
	}
	if breakdown["model-b"] <= 0 {
		t.Error("expected positive cost for model-b")
	}
}

func TestCostTracker_UnknownModel(t *testing.T) {
	pricing := map[string]ModelPricing{
		"known": {InputTokenCost: 0.000001, OutputTokenCost: 0.000002},
	}
	tracker := NewCostTracker(pricing)

	usage := RunUsage{}
	usage.InputTokens = 1000
	tracker.Record("unknown-model", usage)

	if tracker.TotalCost() != 0 {
		t.Errorf("expected 0 cost for unknown model, got %f", tracker.TotalCost())
	}
}

func TestCostTracker_ThreadSafe(t *testing.T) {
	pricing := map[string]ModelPricing{
		"test-model": {InputTokenCost: 0.000001, OutputTokenCost: 0.000002},
	}
	tracker := NewCostTracker(pricing)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			usage := RunUsage{}
			usage.InputTokens = 100
			usage.OutputTokens = 50
			tracker.Record("test-model", usage)
		}()
	}
	wg.Wait()

	if tracker.TotalCost() <= 0 {
		t.Error("expected positive total cost after concurrent recording")
	}
}

func TestCostTracker_AgentIntegration(t *testing.T) {
	pricing := map[string]ModelPricing{
		"test-model": {InputTokenCost: 0.000003, OutputTokenCost: 0.000015},
	}
	tracker := NewCostTracker(pricing)

	resp := TextResponse("done")
	resp.Usage = Usage{InputTokens: 100, OutputTokens: 50}

	model := NewTestModel(resp)
	agent := NewAgent[string](model,
		WithCostTracker[string](tracker),
	)

	result, err := agent.Run(context.Background(), "test cost tracking")
	if err != nil {
		t.Fatal(err)
	}
	if result.Cost == nil {
		t.Fatal("expected Cost on RunResult")
	}
	if result.Cost.TotalCost <= 0 {
		t.Error("expected positive total cost")
	}
	if result.Cost.Currency != "USD" {
		t.Errorf("expected USD, got %q", result.Cost.Currency)
	}
}

func TestCostTracker_ZeroPricing(t *testing.T) {
	pricing := map[string]ModelPricing{
		"free-model": {InputTokenCost: 0, OutputTokenCost: 0},
	}
	tracker := NewCostTracker(pricing)

	usage := RunUsage{}
	usage.InputTokens = 10000
	usage.OutputTokens = 5000
	tracker.Record("free-model", usage)

	if tracker.TotalCost() != 0 {
		t.Errorf("expected 0 cost for zero pricing, got %f", tracker.TotalCost())
	}
}

func TestCostTracker_CacheReadDiscount(t *testing.T) {
	// Simulate Anthropic-style pricing with cache read discount.
	// InputTokens is the total (including cached), so the discount is applied
	// to the cached portion: cached_tokens * (input_rate - cached_rate).
	pricing := map[string]ModelPricing{
		"claude": {
			InputTokenCost:  0.000003,  // $3/1M
			OutputTokenCost: 0.000015,  // $15/1M
			CachedInputCost: 0.0000003, // $0.30/1M (90% discount)
		},
	}
	tracker := NewCostTracker(pricing)

	// 1500 total input tokens, 1000 of which are cached reads.
	usage := RunUsage{}
	usage.InputTokens = 1500 // total including cached
	usage.OutputTokens = 100
	usage.CacheReadTokens = 1000
	tracker.Record("claude", usage)

	// Expected: 500 non-cached * $3/1M + 1000 cached * $0.30/1M + 100 output * $15/1M
	// = 0.0015 + 0.0003 + 0.0015 = 0.0033
	expected := 500*0.000003 + 1000*0.0000003 + 100*0.000015
	cost := tracker.TotalCost()
	if math.Abs(cost-expected) > 0.0000001 {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
	if cost < 0 {
		t.Error("cost should never be negative")
	}
}

func TestCostTracker_CacheWriteSurcharge(t *testing.T) {
	// Cache writes are more expensive than regular input tokens.
	pricing := map[string]ModelPricing{
		"claude": {
			InputTokenCost:  0.000003,   // $3/1M
			OutputTokenCost: 0.000015,   // $15/1M
			CachedInputCost: 0.0000003,  // $0.30/1M
			CacheWriteCost:  0.00000375, // $3.75/1M (1.25x input)
		},
	}
	tracker := NewCostTracker(pricing)

	// 700 total input: 500 non-cached + 200 cache writes.
	usage := RunUsage{}
	usage.InputTokens = 700 // total including cache writes
	usage.OutputTokens = 50
	usage.CacheWriteTokens = 200
	tracker.Record("claude", usage)

	// Expected: 500 non-cached * $3/1M + 200 cache write * $3.75/1M + 50 output * $15/1M
	// = 0.0015 + 0.00075 + 0.00075 = 0.003
	expected := 500*0.000003 + 200*0.00000375 + 50*0.000015
	cost := tracker.TotalCost()
	if math.Abs(cost-expected) > 0.0000001 {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}

func TestCostTracker_CacheReadAndWrite(t *testing.T) {
	// Combined scenario: cache reads (discounted) + cache writes (surcharge).
	pricing := map[string]ModelPricing{
		"claude": {
			InputTokenCost:  0.000003,   // $3/1M
			OutputTokenCost: 0.000015,   // $15/1M
			CachedInputCost: 0.0000003,  // $0.30/1M
			CacheWriteCost:  0.00000375, // $3.75/1M
		},
	}
	tracker := NewCostTracker(pricing)

	// 1700 total input: 500 non-cached + 200 cache write + 1000 cache read.
	usage := RunUsage{}
	usage.InputTokens = 1700 // total
	usage.OutputTokens = 100
	usage.CacheReadTokens = 1000
	usage.CacheWriteTokens = 200
	tracker.Record("claude", usage)

	// Expected: 500 * $3/1M + 1000 * $0.30/1M + 200 * $3.75/1M + 100 * $15/1M
	expected := 500*0.000003 + 1000*0.0000003 + 200*0.00000375 + 100*0.000015
	cost := tracker.TotalCost()
	if math.Abs(cost-expected) > 0.0000001 {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
	if cost < 0 {
		t.Error("cost should never be negative")
	}
}

func TestCostTracker_NoCacheWriteCostFallback(t *testing.T) {
	// When CacheWriteCost is 0, cache write tokens are charged at InputTokenCost.
	pricing := map[string]ModelPricing{
		"model": {
			InputTokenCost:  0.000003,
			OutputTokenCost: 0.000015,
		},
	}
	tracker := NewCostTracker(pricing)

	usage := RunUsage{}
	usage.InputTokens = 700
	usage.OutputTokens = 50
	usage.CacheWriteTokens = 200
	tracker.Record("model", usage)

	// All 700 input tokens charged at input rate, no surcharge.
	expected := 700*0.000003 + 50*0.000015
	cost := tracker.TotalCost()
	if math.Abs(cost-expected) > 0.0000001 {
		t.Errorf("cost = %f, want %f", cost, expected)
	}
}
