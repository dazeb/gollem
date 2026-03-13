package core

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
	"unicode/utf8"
)

// RequestMiddlewareFunc wraps a non-streaming model call within the agent loop.
// next is the actual model call — middleware can modify inputs, outputs, or skip the call.
type RequestMiddlewareFunc func(
	ctx context.Context,
	messages []ModelMessage,
	settings *ModelSettings,
	params *ModelRequestParameters,
	next func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error),
) (*ModelResponse, error)

// AgentStreamFunc is the handler shape for streaming model requests.
type AgentStreamFunc func(
	ctx context.Context,
	messages []ModelMessage,
	settings *ModelSettings,
	params *ModelRequestParameters,
) (StreamedResponse, error)

// AgentStreamMiddleware wraps a streaming model call within the agent loop.
// next is the actual streaming model call.
type AgentStreamMiddleware func(
	ctx context.Context,
	messages []ModelMessage,
	settings *ModelSettings,
	params *ModelRequestParameters,
	next AgentStreamFunc,
) (StreamedResponse, error)

// AgentMiddleware bundles request and stream wrappers behind one registration API.
// Either side may be nil.
type AgentMiddleware struct {
	Request RequestMiddlewareFunc
	Stream  AgentStreamMiddleware
}

// IsZero reports whether neither request nor stream middleware is configured.
func (m AgentMiddleware) IsZero() bool {
	return m.Request == nil && m.Stream == nil
}

// RequestOnlyMiddleware registers middleware only for Request/Run/Iter paths.
func RequestOnlyMiddleware(mw RequestMiddlewareFunc) AgentMiddleware {
	return AgentMiddleware{Request: mw}
}

// StreamOnlyMiddleware registers middleware only for RequestStream/RunStream paths.
func StreamOnlyMiddleware(mw AgentStreamMiddleware) AgentMiddleware {
	return AgentMiddleware{Stream: mw}
}

// DualMiddleware registers both request and stream middleware with one value.
func DualMiddleware(request RequestMiddlewareFunc, stream AgentStreamMiddleware) AgentMiddleware {
	return AgentMiddleware{
		Request: request,
		Stream:  stream,
	}
}

// WithAgentMiddleware adds middleware to the agent's model call chain.
// Middleware is applied in order (first registered = outermost wrapper).
func WithAgentMiddleware[T any](mw AgentMiddleware) AgentOption[T] {
	return func(a *Agent[T]) {
		if mw.Request != nil {
			a.middleware = append(a.middleware, mw.Request)
		}
		if mw.Stream != nil {
			a.streamMiddleware = append(a.streamMiddleware, mw.Stream)
		}
	}
}

// WithAgentStreamMiddleware adds middleware only to the agent's streaming model call chain.
// Prefer WithAgentMiddleware(StreamOnlyMiddleware(...)) for new code.
func WithAgentStreamMiddleware[T any](mw AgentStreamMiddleware) AgentOption[T] {
	return WithAgentMiddleware[T](StreamOnlyMiddleware(mw))
}

// LoggingMiddleware logs model request/response summaries.
func LoggingMiddleware(logger func(msg string)) AgentMiddleware {
	return DualMiddleware(
		func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters, next func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error)) (*ModelResponse, error) {
			logger("model request: " + summarizeMessages(messages))
			resp, err := next(ctx, messages, settings, params)
			if err != nil {
				logger("model error: " + err.Error())
			} else {
				logger("model response: " + resp.TextContent())
			}
			return resp, err
		},
		func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters, next AgentStreamFunc) (StreamedResponse, error) {
			logger("model request: " + summarizeMessages(messages))
			stream, err := next(ctx, messages, settings, params)
			if err != nil {
				logger("model error: " + err.Error())
				return nil, err
			}
			return &streamFinalizeWrapper{
				inner: stream,
				onDone: func(resp *ModelResponse, streamErr error) {
					if streamErr != nil && !errors.Is(streamErr, io.EOF) {
						logger("model error: " + streamErr.Error())
						return
					}
					if resp == nil {
						logger("model response: ")
						return
					}
					logger("model response: " + resp.TextContent())
				},
			}, nil
		},
	)
}

// MaxTokensMiddleware limits the number of tokens per request via ModelSettings.
func MaxTokensMiddleware(maxTokens int) AgentMiddleware {
	return DualMiddleware(
		func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters, next func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error)) (*ModelResponse, error) {
			if settings == nil {
				settings = &ModelSettings{}
			}
			s := *settings
			s.MaxTokens = &maxTokens
			return next(ctx, messages, &s, params)
		},
		func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters, next AgentStreamFunc) (StreamedResponse, error) {
			if settings == nil {
				settings = &ModelSettings{}
			}
			s := *settings
			s.MaxTokens = &maxTokens
			return next(ctx, messages, &s, params)
		},
	)
}

// TimingMiddleware records request duration and calls a callback.
func TimingMiddleware(callback func(duration time.Duration)) AgentMiddleware {
	return DualMiddleware(
		func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters, next func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error)) (*ModelResponse, error) {
			start := time.Now()
			resp, err := next(ctx, messages, settings, params)
			callback(time.Since(start))
			return resp, err
		},
		func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters, next AgentStreamFunc) (StreamedResponse, error) {
			start := time.Now()
			stream, err := next(ctx, messages, settings, params)
			if err != nil {
				callback(time.Since(start))
				return nil, err
			}
			return &streamFinalizeWrapper{
				inner: stream,
				onDone: func(_ *ModelResponse, _ error) {
					callback(time.Since(start))
				},
			}, nil
		},
	)
}

// LoggingStreamMiddleware logs streaming model request/response summaries.
// Prefer WithAgentMiddleware(LoggingMiddleware(...)) for new code.
func LoggingStreamMiddleware(logger func(msg string)) AgentStreamMiddleware {
	return LoggingMiddleware(logger).Stream
}

// MaxTokensStreamMiddleware limits the number of tokens per streaming request via ModelSettings.
// Prefer WithAgentMiddleware(MaxTokensMiddleware(...)) for new code.
func MaxTokensStreamMiddleware(maxTokens int) AgentStreamMiddleware {
	return MaxTokensMiddleware(maxTokens).Stream
}

// TimingStreamMiddleware records total streaming duration and calls a callback
// when the stream completes or is closed. Prefer WithAgentMiddleware(TimingMiddleware(...))
// for new code.
func TimingStreamMiddleware(callback func(duration time.Duration)) AgentStreamMiddleware {
	return TimingMiddleware(callback).Stream
}

// summarizeMessages creates a brief summary of a message list.
func summarizeMessages(messages []ModelMessage) string {
	if len(messages) == 0 {
		return "(empty)"
	}
	last := messages[len(messages)-1]
	if req, ok := last.(ModelRequest); ok {
		for i := len(req.Parts) - 1; i >= 0; i-- {
			if up, ok := req.Parts[i].(UserPromptPart); ok {
				if len(up.Content) > 50 {
					n := 50
					for n > 0 && !utf8.RuneStart(up.Content[n]) {
						n--
					}
					return up.Content[:n] + "..."
				}
				return up.Content
			}
		}
	}
	return "(messages)"
}

// buildMiddlewareChain wraps a model.Request call with the configured middleware.
func buildMiddlewareChain(
	middleware []RequestMiddlewareFunc,
	model Model,
) func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error) {
	// Innermost: actual model call.
	chain := model.Request

	// Wrap in reverse order so first middleware is outermost.
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		next := chain
		chain = func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters) (*ModelResponse, error) {
			return mw(ctx, messages, settings, params, next)
		}
	}

	return chain
}

// buildStreamMiddlewareChain wraps a model.RequestStream call with the configured middleware.
func buildStreamMiddlewareChain(
	middleware []AgentStreamMiddleware,
	model Model,
) AgentStreamFunc {
	chain := model.RequestStream

	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		next := chain
		chain = func(ctx context.Context, messages []ModelMessage, settings *ModelSettings, params *ModelRequestParameters) (StreamedResponse, error) {
			return mw(ctx, messages, settings, params, next)
		}
	}

	return chain
}

type streamFinalizeWrapper struct {
	inner    StreamedResponse
	onDone   func(resp *ModelResponse, err error)
	doneOnce sync.Once
}

func (w *streamFinalizeWrapper) Next() (ModelResponseStreamEvent, error) {
	event, err := w.inner.Next()
	if err != nil {
		w.finalize(err)
	}
	return event, err
}

func (w *streamFinalizeWrapper) Response() *ModelResponse {
	return w.inner.Response()
}

func (w *streamFinalizeWrapper) Usage() Usage {
	return w.inner.Usage()
}

func (w *streamFinalizeWrapper) Close() error {
	err := w.inner.Close()
	w.finalize(err)
	return err
}

func (w *streamFinalizeWrapper) finalize(err error) {
	w.doneOnce.Do(func() {
		if w.onDone != nil {
			w.onDone(w.inner.Response(), err)
		}
	})
}
