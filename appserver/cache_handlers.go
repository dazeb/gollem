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
	return cacheSvc.Stats(), nil
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
