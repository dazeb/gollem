package appserver

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/modelutil"
)

// runtimeCacheTelemetryModel preserves streamed turn behavior while emitting a
// stable, secret-safe request fingerprint when a provider reports its final
// cache usage. It does not serve responses from the cache.
type runtimeCacheTelemetryModel struct {
	model     core.Model
	provider  string
	modelName string
	handler   RuntimeCacheEventHandler
}

func newRuntimeCacheTelemetryModel(model core.Model, info RuntimeModelInfo, handler RuntimeCacheEventHandler) core.Model {
	if model == nil || handler == nil {
		return model
	}
	return &runtimeCacheTelemetryModel{
		model:     model,
		provider:  firstRuntimeNonEmpty(info.ProviderID, info.Provider),
		modelName: firstRuntimeNonEmpty(info.Model, model.ModelName()),
		handler:   handler,
	}
}

func (m *runtimeCacheTelemetryModel) ModelName() string {
	return m.model.ModelName()
}

func (m *runtimeCacheTelemetryModel) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	return m.model.Request(ctx, messages, settings, params)
}

func (m *runtimeCacheTelemetryModel) RequestStream(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (core.StreamedResponse, error) {
	key, err := modelutil.StableCacheKey(modelutil.StableCacheKeyInput{
		Provider: m.provider,
		Model:    m.modelName,
		Messages: messages,
		Settings: settings,
		Params:   params,
	})
	if err != nil {
		return m.model.RequestStream(ctx, messages, settings, params)
	}
	stream, err := m.model.RequestStream(ctx, messages, settings, params)
	if err != nil {
		return nil, err
	}
	return &runtimeCacheTelemetryStream{
		stream: stream,
		event: modelutil.CacheEvent{
			Key:      key,
			Provider: m.provider,
			Model:    m.modelName,
		},
		handler: m.handler,
	}, nil
}

func (m *runtimeCacheTelemetryModel) Close() error {
	closer, ok := m.model.(interface{ Close() error })
	if !ok {
		return nil
	}
	return closer.Close()
}

type runtimeCacheTelemetryStream struct {
	stream  core.StreamedResponse
	event   modelutil.CacheEvent
	handler RuntimeCacheEventHandler
	once    sync.Once
}

func (s *runtimeCacheTelemetryStream) Next() (core.ModelResponseStreamEvent, error) {
	event, err := s.stream.Next()
	if errors.Is(err, io.EOF) {
		s.record()
	}
	return event, err
}

func (s *runtimeCacheTelemetryStream) Response() *core.ModelResponse {
	return s.stream.Response()
}

func (s *runtimeCacheTelemetryStream) Usage() core.Usage {
	return s.stream.Usage()
}

func (s *runtimeCacheTelemetryStream) Close() error {
	return s.stream.Close()
}

func (s *runtimeCacheTelemetryStream) record() {
	s.once.Do(func() {
		usage := s.stream.Usage()
		if response := s.stream.Response(); response != nil {
			if response.Usage.CacheReadTokens > usage.CacheReadTokens {
				usage.CacheReadTokens = response.Usage.CacheReadTokens
			}
			if response.Usage.CacheWriteTokens > usage.CacheWriteTokens {
				usage.CacheWriteTokens = response.Usage.CacheWriteTokens
			}
		}
		s.event.Type = modelutil.CacheEventMiss
		if usage.CacheReadTokens > 0 {
			s.event.Type = modelutil.CacheEventHit
		}
		s.event.CacheReadTokens = usage.CacheReadTokens
		s.event.CacheWriteTokens = usage.CacheWriteTokens
		s.event.At = time.Now().UTC()
		s.handler(s.event)
	})
}

var _ core.Model = (*runtimeCacheTelemetryModel)(nil)
var _ core.StreamedResponse = (*runtimeCacheTelemetryStream)(nil)
