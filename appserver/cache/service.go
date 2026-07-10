package cache

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/modelutil"
)

const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"

	defaultBenchmarkIterations = 2
	defaultBenchmarkTarget     = 0.90
	defaultRecentEventLimit    = 200
)

// Service stores cache telemetry and runs deterministic normalization
// benchmarks for app-server JSON-RPC handlers.
type Service struct {
	mu           sync.Mutex
	total        counter
	providers    map[string]counter
	recentEvents []Event
	eventLimit   int
}

type counter struct {
	Hits   int64
	Misses int64
}

// NewService constructs an in-memory cache telemetry service.
func NewService() *Service {
	return &Service{
		providers:  make(map[string]counter),
		eventLimit: defaultRecentEventLimit,
	}
}

// Event is the typed cache hit/miss telemetry shape surfaced by app-server.
type Event struct {
	Type      modelutil.CacheEventType `json:"type"`
	Provider  string                   `json:"provider,omitempty"`
	Model     string                   `json:"model,omitempty"`
	Key       string                   `json:"key"`
	Fixture   string                   `json:"fixture,omitempty"`
	Iteration int                      `json:"iteration,omitempty"`
	At        time.Time                `json:"at"`
}

// StatsResponse is returned by cache/stats.
type StatsResponse struct {
	TotalRequests int64           `json:"totalRequests"`
	Hits          int64           `json:"hits"`
	Misses        int64           `json:"misses"`
	HitRate       float64         `json:"hitRate"`
	Providers     []ProviderStats `json:"providers"`
	RecentEvents  []Event         `json:"recentEvents,omitempty"`
}

// ProviderStats is the per-provider cache telemetry summary.
type ProviderStats struct {
	Provider      string  `json:"provider"`
	TotalRequests int64   `json:"totalRequests"`
	Hits          int64   `json:"hits"`
	Misses        int64   `json:"misses"`
	HitRate       float64 `json:"hitRate"`
}

// BenchmarkParams configures cache/benchmark.
type BenchmarkParams struct {
	Providers     []string `json:"providers,omitempty"`
	Iterations    int      `json:"iterations,omitempty"`
	IncludeEvents bool     `json:"includeEvents,omitempty"`
	TargetHitRate *float64 `json:"targetHitRate,omitempty"`
}

// BenchmarkResponse is returned by cache/benchmark.
type BenchmarkResponse struct {
	Passed        bool                      `json:"passed"`
	TargetHitRate float64                   `json:"targetHitRate"`
	Iterations    int                       `json:"iterations"`
	Providers     []ProviderBenchmarkResult `json:"providers"`
	Totals        ProviderStats             `json:"totals"`
	Events        []Event                   `json:"events,omitempty"`
}

// ProviderBenchmarkResult reports one provider's deterministic benchmark
// outcome.
type ProviderBenchmarkResult struct {
	Provider      string                   `json:"provider"`
	Model         string                   `json:"model"`
	Passed        bool                     `json:"passed"`
	TotalRequests int64                    `json:"totalRequests"`
	Hits          int64                    `json:"hits"`
	Misses        int64                    `json:"misses"`
	HitRate       float64                  `json:"hitRate"`
	Fixtures      []FixtureBenchmarkResult `json:"fixtures"`
}

// FixtureBenchmarkResult reports benchmark counters for one semantic request
// fixture and its unstable variants.
type FixtureBenchmarkResult struct {
	Name          string  `json:"name"`
	Variants      int     `json:"variants"`
	TotalRequests int64   `json:"totalRequests"`
	Hits          int64   `json:"hits"`
	Misses        int64   `json:"misses"`
	HitRate       float64 `json:"hitRate"`
}

type benchmarkFixture struct {
	name     string
	model    string
	variants []any
}

// Stats returns a stable cache telemetry snapshot.
func (s *Service) Stats() StatsResponse {
	if s == nil {
		return StatsResponse{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statsLocked()
}

// RecordModelEvent folds a modelutil cache event into app-server telemetry.
func (s *Service) RecordModelEvent(event modelutil.CacheEvent) {
	if s == nil || event.Type == "" {
		return
	}
	provider := event.Provider
	record := Event{
		Type:     event.Type,
		Provider: provider,
		Model:    event.Model,
		Key:      event.Key,
		At:       event.At,
	}
	if record.At.IsZero() {
		record.At = time.Now().UTC()
	}
	s.recordEvents([]Event{record})
}

// Benchmark runs deterministic provider fixtures against the stable cache key
// normalizer and records the hit/miss events in service telemetry.
func (s *Service) Benchmark(params BenchmarkParams) (BenchmarkResponse, error) {
	providers, err := normalizeProviders(params.Providers)
	if err != nil {
		return BenchmarkResponse{}, err
	}
	iterations := params.Iterations
	if iterations == 0 {
		iterations = defaultBenchmarkIterations
	}
	if iterations < 0 {
		return BenchmarkResponse{}, errors.New("iterations must be non-negative")
	}
	target := defaultBenchmarkTarget
	if params.TargetHitRate != nil {
		target = *params.TargetHitRate
	}
	if target < 0 || target > 1 {
		return BenchmarkResponse{}, errors.New("targetHitRate must be between 0 and 1")
	}

	response := BenchmarkResponse{
		Passed:        true,
		TargetHitRate: target,
		Iterations:    iterations,
	}
	var events []Event
	for _, provider := range providers {
		providerResult, providerEvents, err := runProviderBenchmark(provider, iterations, target)
		if err != nil {
			return BenchmarkResponse{}, err
		}
		if !providerResult.Passed {
			response.Passed = false
		}
		response.Providers = append(response.Providers, providerResult)
		events = append(events, providerEvents...)
		response.Totals.Hits += providerResult.Hits
		response.Totals.Misses += providerResult.Misses
		response.Totals.TotalRequests += providerResult.TotalRequests
	}
	response.Totals.Provider = "all"
	response.Totals.HitRate = hitRate(response.Totals.Hits, response.Totals.Misses)
	if params.IncludeEvents {
		response.Events = append([]Event(nil), events...)
	}
	s.recordEvents(events)
	return response, nil
}

func (s *Service) recordEvents(events []Event) {
	if s == nil || len(events) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range events {
		switch event.Type {
		case modelutil.CacheEventHit:
			s.total.Hits++
			provider := s.providers[event.Provider]
			provider.Hits++
			s.providers[event.Provider] = provider
		case modelutil.CacheEventMiss:
			s.total.Misses++
			provider := s.providers[event.Provider]
			provider.Misses++
			s.providers[event.Provider] = provider
		default:
			continue
		}
		s.recentEvents = append(s.recentEvents, event)
		if s.eventLimit > 0 && len(s.recentEvents) > s.eventLimit {
			copy(s.recentEvents, s.recentEvents[len(s.recentEvents)-s.eventLimit:])
			s.recentEvents = s.recentEvents[:s.eventLimit]
		}
	}
}

func (s *Service) statsLocked() StatsResponse {
	response := StatsResponse{
		TotalRequests: s.total.Hits + s.total.Misses,
		Hits:          s.total.Hits,
		Misses:        s.total.Misses,
		HitRate:       hitRate(s.total.Hits, s.total.Misses),
		RecentEvents:  append([]Event(nil), s.recentEvents...),
	}
	providers := make([]string, 0, len(s.providers))
	for provider := range s.providers {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	for _, provider := range providers {
		c := s.providers[provider]
		response.Providers = append(response.Providers, ProviderStats{
			Provider:      provider,
			TotalRequests: c.Hits + c.Misses,
			Hits:          c.Hits,
			Misses:        c.Misses,
			HitRate:       hitRate(c.Hits, c.Misses),
		})
	}
	return response
}

func runProviderBenchmark(provider string, iterations int, target float64) (ProviderBenchmarkResult, []Event, error) {
	fixtures, err := fixturesForProvider(provider)
	if err != nil {
		return ProviderBenchmarkResult{}, nil, err
	}
	result := ProviderBenchmarkResult{
		Provider: provider,
		Model:    fixtures[0].model,
		Passed:   true,
	}
	seen := make(map[string]struct{})
	var events []Event
	for _, fixture := range fixtures {
		fixtureResult := FixtureBenchmarkResult{Name: fixture.name, Variants: len(fixture.variants)}
		for iteration := 1; iteration <= iterations; iteration++ {
			for _, variant := range fixture.variants {
				key, err := modelutil.StableCacheKeyFromJSON(provider, fixture.model, variant)
				if err != nil {
					return ProviderBenchmarkResult{}, nil, err
				}
				eventType := modelutil.CacheEventHit
				if _, ok := seen[key]; !ok {
					eventType = modelutil.CacheEventMiss
					seen[key] = struct{}{}
				}
				event := Event{
					Type:      eventType,
					Provider:  provider,
					Model:     fixture.model,
					Key:       key,
					Fixture:   fixture.name,
					Iteration: iteration,
					At:        time.Now().UTC(),
				}
				events = append(events, event)
				if eventType == modelutil.CacheEventHit {
					fixtureResult.Hits++
					result.Hits++
				} else {
					fixtureResult.Misses++
					result.Misses++
				}
				fixtureResult.TotalRequests++
				result.TotalRequests++
			}
		}
		fixtureResult.HitRate = hitRate(fixtureResult.Hits, fixtureResult.Misses)
		result.Fixtures = append(result.Fixtures, fixtureResult)
	}
	result.HitRate = hitRate(result.Hits, result.Misses)
	result.Passed = result.HitRate >= target
	return result, events, nil
}

func normalizeProviders(input []string) ([]string, error) {
	if len(input) == 0 {
		return []string{ProviderOpenAI, ProviderAnthropic}, nil
	}
	seen := make(map[string]struct{}, len(input))
	var providers []string
	for _, provider := range input {
		switch provider {
		case ProviderOpenAI, ProviderAnthropic:
		default:
			return nil, fmt.Errorf("unsupported provider %q", provider)
		}
		if _, ok := seen[provider]; ok {
			continue
		}
		seen[provider] = struct{}{}
		providers = append(providers, provider)
	}
	return providers, nil
}

func hitRate(hits, misses int64) float64 {
	total := hits + misses
	if total == 0 {
		return 0
	}
	return float64(hits) / float64(total)
}

func fixturesForProvider(provider string) ([]benchmarkFixture, error) {
	switch provider {
	case ProviderOpenAI:
		return []benchmarkFixture{
			{
				name:     "openai-chat-tool-call",
				model:    "gpt-4o-mini",
				variants: variants(6, openAIChatToolPayload),
			},
			{
				name:     "openai-structured-output",
				model:    "gpt-4o",
				variants: variants(6, openAIStructuredOutputPayload),
			},
		}, nil
	case ProviderAnthropic:
		return []benchmarkFixture{
			{
				name:     "anthropic-messages-tool-use",
				model:    "claude-sonnet-4-6",
				variants: variants(6, anthropicToolUsePayload),
			},
			{
				name:     "anthropic-thinking-document",
				model:    "claude-sonnet-4-6",
				variants: variants(6, anthropicThinkingDocumentPayload),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", provider)
	}
}

func variants(count int, fn func(int) any) []any {
	out := make([]any, 0, count)
	for i := range count {
		out = append(out, fn(i))
	}
	return out
}

func openAIChatToolPayload(i int) any {
	callID := fmt.Sprintf("call_openai_%d", i)
	args := `{"city":"Paris","unit":"c"}`
	if i%2 == 0 {
		args = `{"unit":"c","city":"Paris"}`
	}
	tools := []any{weatherTool(), lookupTool()}
	if i%2 == 0 {
		tools = []any{lookupTool(), weatherTool()}
	}
	return map[string]any{
		"model":                  "gpt-4o-mini",
		"request_id":             fmt.Sprintf("req-openai-%d", i),
		"created":                1783039000 + i,
		"seed":                   i + 1000,
		"prompt_cache_key":       fmt.Sprintf("trace-openai-%d", i),
		"prompt_cache_retention": "24h",
		"service_tier":           "auto",
		"stream":                 i%2 == 0,
		"stream_options":         map[string]any{"include_usage": i%2 == 0},
		"messages": []any{
			map[string]any{"role": "system", "content": "You are terse.", "timestamp": fmt.Sprintf("2026-07-02T10:00:%02dZ", i)},
			map[string]any{"role": "user", "content": "Get weather for Paris."},
			map[string]any{
				"role": "assistant",
				"tool_calls": []any{
					map[string]any{
						"id":   callID,
						"type": "function",
						"function": map[string]any{
							"name":      "weather",
							"arguments": args,
						},
					},
				},
			},
			map[string]any{"role": "tool", "tool_call_id": callID, "content": `{"temperature":21}`},
		},
		"tools":       tools,
		"temperature": 0.2,
	}
}

func openAIStructuredOutputPayload(i int) any {
	required := []any{"title", "status"}
	if i%2 == 0 {
		required = []any{"status", "title"}
	}
	return map[string]any{
		"model":                  "gpt-4o",
		"request_id":             fmt.Sprintf("req-structured-%d", i),
		"prompt_cache_key":       fmt.Sprintf("structured-cache-%d", i),
		"prompt_cache_retention": "1h",
		"messages": []any{
			map[string]any{"role": "system", "content": "Return only JSON."},
			map[string]any{"role": "user", "content": "Classify the task."},
		},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "task_classification",
				"strict": true,
				"schema": map[string]any{
					"type":     "object",
					"required": required,
					"properties": map[string]any{
						"status": map[string]any{"type": "string", "enum": []any{"blocked", "ready"}},
						"title":  map[string]any{"type": "string"},
					},
				},
			},
		},
		"stream":         i%2 == 1,
		"stream_options": map[string]any{"include_usage": true},
	}
}

func anthropicToolUsePayload(i int) any {
	toolID := fmt.Sprintf("toolu_%d", i)
	tools := []any{lookupToolAnthropic(), weatherToolAnthropic()}
	if i%2 == 0 {
		tools = []any{weatherToolAnthropic(), lookupToolAnthropic()}
	}
	return map[string]any{
		"model":      "claude-sonnet-4-6",
		"request_id": fmt.Sprintf("req-anthropic-%d", i),
		"stream":     i%2 == 0,
		"system": []any{
			map[string]any{
				"type":          "text",
				"text":          "You are terse.",
				"cache_control": map[string]any{"type": "ephemeral"},
				"timestamp":     fmt.Sprintf("2026-07-02T10:01:%02dZ", i),
			},
		},
		"messages": []any{
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "Use lookup."}}},
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "tool_use", "id": toolID, "name": "lookup", "input": map[string]any{"query": "cache", "limit": 3}},
					map[string]any{"type": "thinking", "thinking": "Lookup is needed.", "signature": fmt.Sprintf("sig-%d", i)},
				},
			},
			map[string]any{"role": "user", "content": []any{map[string]any{"type": "tool_result", "tool_use_id": toolID, "content": "ok"}}},
		},
		"tools":       tools,
		"temperature": 0.2,
	}
}

func anthropicThinkingDocumentPayload(i int) any {
	return map[string]any{
		"model":      "claude-sonnet-4-6",
		"request_id": fmt.Sprintf("req-anthropic-doc-%d", i),
		"system": []any{
			map[string]any{"type": "text", "text": "Extract document status.", "cache_control": map[string]any{"type": "ephemeral"}},
		},
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "document", "source": map[string]any{"type": "url", "url": "https://example.test/spec.pdf"}, "title": "Spec"},
					map[string]any{"type": "text", "text": "Summarize the release state."},
				},
			},
			map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "thinking", "thinking": "Read the document.", "signature": fmt.Sprintf("doc-sig-%d", i)},
					map[string]any{"type": "text", "text": "The release is ready."},
				},
			},
		},
		"thinking": map[string]any{"type": "adaptive"},
		"stream":   i%2 == 1,
	}
}

func weatherTool() any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "weather",
			"description": "Read weather.",
			"parameters": map[string]any{
				"type":     "object",
				"required": []any{"city", "unit"},
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
					"unit": map[string]any{"type": "string", "enum": []any{"f", "c"}},
				},
			},
		},
	}
}

func lookupTool() any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "lookup",
			"description": "Lookup facts.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"limit": map[string]any{"type": "integer"},
				},
			},
		},
	}
}

func lookupToolAnthropic() any {
	return map[string]any{
		"name":          "lookup",
		"description":   "Lookup facts.",
		"cache_control": map[string]any{"type": "ephemeral"},
		"input_schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
		},
	}
}

func weatherToolAnthropic() any {
	return map[string]any{
		"name":        "weather",
		"description": "Read weather.",
		"input_schema": map[string]any{
			"type":     "object",
			"required": []any{"unit", "city"},
			"properties": map[string]any{
				"unit": map[string]any{"type": "string", "enum": []any{"c", "f"}},
				"city": map[string]any{"type": "string"},
			},
		},
	}
}
