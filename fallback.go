package gollem

import (
	"context"
	"fmt"
)

// FallbackModel wraps multiple models, trying each in order until one succeeds.
// If a model request fails, the next model in the chain is tried.
// This is useful for reliability (e.g., try Claude, fall back to GPT-4).
type FallbackModel struct {
	models []Model
}

// Compile-time interface check.
var _ Model = (*FallbackModel)(nil)

// NewFallbackModel creates a model that tries each model in order.
// At least two models must be provided.
func NewFallbackModel(primary Model, fallbacks ...Model) *FallbackModel {
	models := make([]Model, 0, 1+len(fallbacks))
	models = append(models, primary)
	models = append(models, fallbacks...)
	return &FallbackModel{models: models}
}

func (f *FallbackModel) Request(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters) (*ModelResponse, error) {
	var lastErr error
	for _, m := range f.models {
		resp, err := m.Request(ctx, messages, settings, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all models failed, last error: %w", lastErr)
}

func (f *FallbackModel) RequestStream(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters) (StreamedResponse, error) {
	var lastErr error
	for _, m := range f.models {
		resp, err := m.RequestStream(ctx, messages, settings, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("all models failed, last error: %w", lastErr)
}

func (f *FallbackModel) ModelName() string {
	if len(f.models) > 0 {
		return f.models[0].ModelName() + "+fallback"
	}
	return "fallback"
}
