package appserver

import (
	"encoding/json"

	appcache "github.com/fugue-labs/gollem/appserver/cache"
	"github.com/fugue-labs/gollem/appserver/protocol"
)

func (s *Server) handleCacheStats() (any, *protocol.Error) {
	cacheSvc, rpcErr := s.requireCache("cache/stats")
	if rpcErr != nil {
		return nil, rpcErr
	}
	return cacheStatsResponse(cacheSvc.Stats()), nil
}

func cacheStatsResponse(stats appcache.StatsResponse) protocol.CacheStatsResponse {
	result := protocol.CacheStatsResponse{
		TotalRequests: stats.TotalRequests,
		Hits:          stats.Hits,
		Misses:        stats.Misses,
		HitRate:       stats.HitRate,
		Providers:     []protocol.CacheProviderStats{},
	}
	if len(stats.Providers) > 0 {
		result.Providers = make([]protocol.CacheProviderStats, len(stats.Providers))
		for index, provider := range stats.Providers {
			result.Providers[index] = protocol.CacheProviderStats{
				Provider:      provider.Provider,
				TotalRequests: provider.TotalRequests,
				Hits:          provider.Hits,
				Misses:        provider.Misses,
				HitRate:       provider.HitRate,
			}
		}
	}
	if len(stats.RecentEvents) > 0 {
		result.RecentEvents = make([]protocol.CacheEvent, len(stats.RecentEvents))
		for index, event := range stats.RecentEvents {
			result.RecentEvents[index] = protocol.CacheEvent{
				Type:             protocol.CacheEventType(event.Type),
				Provider:         event.Provider,
				Model:            event.Model,
				Key:              event.Key,
				Source:           event.Source,
				Fixture:          event.Fixture,
				Iteration:        event.Iteration,
				CacheReadTokens:  event.CacheReadTokens,
				CacheWriteTokens: event.CacheWriteTokens,
				At:               event.At,
			}
		}
	}
	return result
}

func (s *Server) handleCacheBenchmark(raw json.RawMessage) (any, *protocol.Error) {
	cacheSvc, rpcErr := s.requireCache("cache/benchmark")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params appcache.BenchmarkParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	result, err := cacheSvc.Benchmark(params)
	if err != nil {
		return nil, invalidParams("invalid cache/benchmark params", err)
	}
	s.publishCacheBenchmarkCompleted(result)
	return result, nil
}

func (s *Server) requireCache(method string) (*appcache.Service, *protocol.Error) {
	if s.cache == nil {
		return nil, protocol.MethodUnavailableErrorWithReason(method, "cache service is not configured")
	}
	return s.cache, nil
}
