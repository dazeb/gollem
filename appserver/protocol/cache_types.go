package protocol

import "time"

// CacheEventType identifies a cache hit or miss recorded by cache/stats.
type CacheEventType string

const (
	CacheEventHit  CacheEventType = "cache.hit"
	CacheEventMiss CacheEventType = "cache.miss"
)

// CacheEvent is one bounded cache telemetry record. Cache keys are opaque
// stable fingerprints, not request content.
type CacheEvent struct {
	Type             CacheEventType `json:"type" jsonschema:"enum=cache.hit|cache.miss"`
	Provider         string         `json:"provider,omitempty"`
	Model            string         `json:"model,omitempty"`
	Key              string         `json:"key"`
	Source           string         `json:"source,omitempty"`
	Fixture          string         `json:"fixture,omitempty"`
	Iteration        int            `json:"iteration,omitempty"`
	CacheReadTokens  int            `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int            `json:"cacheWriteTokens,omitempty"`
	At               time.Time      `json:"at"`
}

// CacheProviderStats is an aggregate cache telemetry summary for one provider.
type CacheProviderStats struct {
	Provider      string  `json:"provider"`
	TotalRequests int64   `json:"totalRequests"`
	Hits          int64   `json:"hits"`
	Misses        int64   `json:"misses"`
	HitRate       float64 `json:"hitRate"`
}

// CacheStatsResponse is the complete cache/stats result. Consumers that do
// not need raw event metadata should project only the aggregate fields.
type CacheStatsResponse struct {
	TotalRequests int64                `json:"totalRequests"`
	Hits          int64                `json:"hits"`
	Misses        int64                `json:"misses"`
	HitRate       float64              `json:"hitRate"`
	Providers     []CacheProviderStats `json:"providers" jsonschema:"nonnullable=true"`
	RecentEvents  []CacheEvent         `json:"recentEvents,omitempty" jsonschema:"nonnullable=true"`
}
