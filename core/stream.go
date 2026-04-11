package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"sync"
	"time"
)

// StreamResult wraps a streaming model response and provides methods to
// consume the stream as text, events, or structured output.
type StreamResult[T any] struct {
	stream   StreamedResponse
	messages []ModelMessage
	resultFn func() (*RunResult[T], error)

	mu sync.Mutex
}

// newStreamResult creates a new StreamResult.
func newStreamResult[T any](stream StreamedResponse, messages []ModelMessage, resultFn func() (*RunResult[T], error)) *StreamResult[T] {
	return &StreamResult[T]{
		stream:   stream,
		messages: messages,
		resultFn: resultFn,
	}
}

// StreamText returns an iterator that yields text content from the stream.
// If delta is true, yields incremental text chunks.
// If delta is false, yields cumulative text.
func (s *StreamResult[T]) StreamText(delta bool) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		var cumulative string
		for {
			event, err := s.stream.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield("", err)
				return
			}

			switch e := event.(type) {
			case PartDeltaEvent:
				if td, ok := e.Delta.(TextPartDelta); ok {
					if delta {
						if !yield(td.ContentDelta, nil) {
							return
						}
					} else {
						cumulative += td.ContentDelta
						if !yield(cumulative, nil) {
							return
						}
					}
				}
			case PartStartEvent:
				if tp, ok := e.Part.(TextPart); ok {
					if delta {
						if tp.Content != "" {
							if !yield(tp.Content, nil) {
								return
							}
						}
					} else {
						cumulative += tp.Content
						if cumulative != "" {
							if !yield(cumulative, nil) {
								return
							}
						}
					}
				}
			}
		}
	}
}

// StreamEvents returns an iterator over raw stream events.
func (s *StreamResult[T]) StreamEvents() iter.Seq2[ModelResponseStreamEvent, error] {
	return func(yield func(ModelResponseStreamEvent, error) bool) {
		for {
			event, err := s.stream.Next()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}

// GetOutput consumes the entire stream and returns the final response.
func (s *StreamResult[T]) GetOutput() (*ModelResponse, error) {
	if _, err := s.Result(); err != nil {
		return nil, err
	}
	resp := s.stream.Response()
	if resp == nil {
		return nil, errors.New("stream completed without a response")
	}
	return resp, nil
}

// Result consumes the entire stream and returns the final typed run result.
func (s *StreamResult[T]) Result() (*RunResult[T], error) {
	if s.resultFn == nil {
		for {
			_, err := s.stream.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				return nil, err
			}
		}
		return nil, errors.New("stream does not expose a typed result")
	}
	return s.resultFn()
}

// Response returns the ModelResponse built from data received so far.
func (s *StreamResult[T]) Response() *ModelResponse {
	return s.stream.Response()
}

// Messages returns a copy of the message history at the start of this stream.
func (s *StreamResult[T]) Messages() []ModelMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]ModelMessage, len(s.messages))
	copy(result, s.messages)
	return result
}

// Close releases streaming resources.
func (s *StreamResult[T]) Close() error {
	return s.stream.Close()
}

var errStreamClosed = errors.New("stream closed before completion")

type agentStream[T any] struct {
	agent    *Agent[T]
	ctx      context.Context
	state    *agentRunState
	settings *ModelSettings
	limits   UsageLimits
	deps     any
	prompt   string
	allTools []Tool
	toolMap  map[string]*Tool
	outNames map[string]bool

	current         StreamedResponse
	currentTurnRC   *RunContext
	currentModelRC  *RunContext
	currentReqStart time.Time
	finalResponse   *ModelResponse

	done   bool
	result *RunResult[T]
	err    error

	endOnce sync.Once
}

func newAgentStream[T any](
	agent *Agent[T],
	ctx context.Context,
	state *agentRunState,
	settings *ModelSettings,
	limits UsageLimits,
	deps any,
	prompt string,
	allTools []Tool,
	toolMap map[string]*Tool,
	outNames map[string]bool,
) *agentStream[T] {
	return &agentStream[T]{
		agent:    agent,
		ctx:      ctx,
		state:    state,
		settings: settings,
		limits:   limits,
		deps:     deps,
		prompt:   prompt,
		allTools: allTools,
		toolMap:  toolMap,
		outNames: outNames,
	}
}

func (s *agentStream[T]) Next() (ModelResponseStreamEvent, error) {
	if s.done {
		if s.err != nil {
			return nil, s.err
		}
		return nil, io.EOF
	}

	for {
		if s.current == nil {
			if err := s.startTurn(); err != nil {
				s.finish(nil, err)
				return nil, err
			}
		}

		event, err := s.current.Next()
		if err == nil {
			return event, nil
		}
		if !errors.Is(err, io.EOF) {
			s.finish(nil, fmt.Errorf("model stream failed: %w", err))
			return nil, s.err
		}

		if err := s.completeTurn(); err != nil {
			s.finish(nil, err)
			return nil, err
		}
		if s.done {
			return nil, io.EOF
		}
	}
}

func (s *agentStream[T]) Response() *ModelResponse {
	if s.current != nil {
		if resp := s.current.Response(); resp != nil {
			return resp
		}
	}
	return s.finalResponse
}

func (s *agentStream[T]) Usage() Usage {
	usage := s.state.usage.Usage
	if s.current != nil {
		usage.Incr(s.current.Usage())
	}
	return usage
}

func (s *agentStream[T]) Close() error {
	if s.current != nil {
		current := s.current
		s.current = nil
		err := current.Close()
		if !s.done {
			s.finish(nil, errStreamClosed)
		}
		return err
	}
	if !s.done {
		s.finish(nil, errStreamClosed)
	}
	return nil
}

func (s *agentStream[T]) Result() (*RunResult[T], error) {
	for {
		_, err := s.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return nil, errors.New("stream completed without a result")
	}
	return s.result, nil
}

// emitStreamTurnCompleted publishes a TurnCompletedEvent for streaming runs.
func (s *agentStream[T]) emitStreamTurnCompleted(resp *ModelResponse, turnErr error) {
	if s.agent.eventBus == nil {
		return
	}
	ev := TurnCompletedEvent{
		RunID:       s.state.runID,
		ParentRunID: s.state.parentRunID,
		TurnNumber:  s.state.runStep,
		CompletedAt: time.Now(),
	}
	if resp != nil {
		ev.HasToolCalls = len(resp.ToolCalls()) > 0
		ev.HasText = resp.TextContent() != ""
	}
	if turnErr != nil {
		ev.Error = turnErr.Error()
	}
	Publish(s.agent.eventBus, ev)
}

func (s *agentStream[T]) completeActiveTurn(resp *ModelResponse, turnErr error) {
	s.emitStreamTurnCompleted(resp, turnErr)
	s.currentTurnRC = nil
	s.currentModelRC = nil
}

func (s *agentStream[T]) startTurn() error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	if err := s.limits.CheckBeforeRequest(s.state.usage); err != nil {
		return err
	}
	if s.limits.ToolCallsLimit != nil {
		if err := s.limits.CheckToolCalls(s.state.usage); err != nil {
			return err
		}
	}
	if err := checkQuota(s.agent.usageQuota, s.state.usage); err != nil {
		return err
	}

	s.state.runStep++

	turnRC := &RunContext{
		Deps:         s.deps,
		Usage:        s.state.usage,
		Prompt:       s.prompt,
		RunStep:      s.state.runStep,
		RunID:        s.state.runID,
		RunStartTime: s.state.startTime,
		EventBus:     s.agent.eventBus,
	}
	s.agent.fireHook(func(h Hook) {
		if h.OnTurnStart != nil {
			h.OnTurnStart(s.ctx, turnRC, s.state.runStep)
		}
	})
	if s.agent.eventBus != nil {
		Publish(s.agent.eventBus, TurnStartedEvent{
			RunID:       s.state.runID,
			ParentRunID: s.state.parentRunID,
			TurnNumber:  s.state.runStep,
			StartedAt:   time.Now(),
		})
	}

	preparedTools := s.agent.prepareTools(s.ctx, s.state, s.allTools, s.deps, s.prompt)
	params := buildModelRequestParams(preparedTools, s.agent.outputSchema)

	settings := s.settings
	if s.agent.toolChoice != nil {
		if settings == nil {
			settings = &ModelSettings{}
		}
		if settings.ToolChoice == nil {
			settings.ToolChoice = s.agent.toolChoice
		}
	}

	if s.agent.autoContext != nil {
		beforeCount := len(s.state.messages)
		beforeTokens := currentContextTokenCount(s.state.messages, s.state.lastInputTokens)
		compressed, compErr := autoCompressMessages(s.ctx, s.state.messages, s.agent.autoContext, s.agent.model, beforeTokens)
		if compErr == nil && len(compressed) < beforeCount {
			s.state.messages = compressed
			stats := ContextCompactionStats{
				Strategy:       CompactionStrategyAutoSummary,
				MessagesBefore: beforeCount,
				MessagesAfter:  len(compressed),
			}
			s.agent.fireHook(func(h Hook) {
				if h.OnContextCompaction != nil {
					h.OnContextCompaction(s.ctx, turnRC, stats)
				}
			})
			if s.agent.tracingEnabled {
				s.state.recordCompaction(stats)
			}
		}
	}

	messages := s.state.messages
	for _, proc := range s.agent.historyProcessors {
		beforeCount := len(messages)
		var compactions []ContextCompactionStats
		procCtx := ContextWithCompactionCallback(s.ctx, func(stats ContextCompactionStats) {
			compactions = append(compactions, stats)
		})
		processed, procErr := proc(procCtx, messages)
		if procErr != nil {
			hpErr := fmt.Errorf("history processor failed: %w", procErr)
			s.emitStreamTurnCompleted(nil, hpErr)
			return hpErr
		}
		if len(compactions) == 0 && len(processed) < beforeCount {
			compactions = append(compactions, ContextCompactionStats{
				Strategy:       CompactionStrategyHistoryProcessor,
				MessagesBefore: beforeCount,
				MessagesAfter:  len(processed),
			})
		}
		for _, stats := range compactions {
			s.agent.fireHook(func(h Hook) {
				if h.OnContextCompaction != nil {
					h.OnContextCompaction(s.ctx, turnRC, stats)
				}
			})
			if s.agent.tracingEnabled {
				s.state.recordCompaction(stats)
			}
		}
		messages = processed
	}

	if len(s.agent.messageInterceptors) > 0 {
		var dropped bool
		messages, dropped = runMessageInterceptors(s.ctx, s.agent.messageInterceptors, messages)
		if dropped {
			miErr := errors.New("message interceptor dropped the request")
			s.emitStreamTurnCompleted(nil, miErr)
			return miErr
		}
	}

	turnGuardRC := s.agent.buildRunContext(s.state, s.deps, s.prompt)
	turnGuardRC.Messages = messages
	for _, g := range s.agent.turnGuardrails {
		gErr := g.fn(s.ctx, turnGuardRC, messages)
		passed := gErr == nil
		s.agent.fireHook(func(h Hook) {
			if h.OnGuardrailEvaluated != nil {
				h.OnGuardrailEvaluated(s.ctx, turnGuardRC, g.name, passed, gErr)
			}
		})
		if gErr != nil {
			grErr := &GuardrailError{
				GuardrailName: g.name,
				Message:       gErr.Error(),
			}
			s.emitStreamTurnCompleted(nil, grErr)
			return grErr
		}
	}

	modelRC := s.agent.buildRunContext(s.state, s.deps, s.prompt)
	modelRC.Messages = messages
	s.agent.fireHook(func(h Hook) {
		if h.OnModelRequest != nil {
			h.OnModelRequest(s.ctx, modelRC, messages)
		}
	})

	modelReqStart := time.Now()
	if s.agent.eventBus != nil {
		Publish(s.agent.eventBus, ModelRequestStartedEvent{
			RunID:        s.state.runID,
			ParentRunID:  s.state.parentRunID,
			TurnNumber:   s.state.runStep,
			MessageCount: len(messages),
			StartedAt:    modelReqStart,
		})
	}
	if s.agent.tracingEnabled {
		s.state.traceSteps = append(s.state.traceSteps, TraceStep{
			Kind:      TraceModelRequest,
			Timestamp: modelReqStart,
			Data:      map[string]any{"message_count": len(messages)},
		})
	}

	var (
		stream StreamedResponse
		err    error
	)
	middleware := s.agent.streamMiddleware
	if s.agent.tracingEnabled {
		middleware = append(append([]AgentStreamMiddleware(nil), middleware...), newRequestTraceMiddleware(s.state, s.agent.model.ModelName()).Stream)
	}
	if len(middleware) > 0 {
		mwCtx := ContextWithCompactionCallback(s.ctx, func(stats ContextCompactionStats) {
			s.agent.fireHook(func(h Hook) {
				if h.OnContextCompaction != nil {
					h.OnContextCompaction(s.ctx, turnRC, stats)
				}
			})
			if s.agent.tracingEnabled {
				s.state.recordCompaction(stats)
			}
		})
		chain := buildStreamMiddlewareChain(middleware, s.agent.model)
		stream, err = chain(mwCtx, messages, settings, params)
	} else {
		stream, err = s.agent.model.RequestStream(s.ctx, messages, settings, params)
	}
	if err != nil {
		streamErr := fmt.Errorf("model stream request failed: %w", err)
		s.emitStreamTurnCompleted(nil, streamErr)
		return streamErr
	}

	s.settings = settings
	s.current = stream
	s.currentTurnRC = turnRC
	s.currentModelRC = modelRC
	s.currentReqStart = modelReqStart
	return nil
}

func (s *agentStream[T]) completeTurn() error {
	resp := s.current.Response()
	if resp == nil {
		_ = s.current.Close()
		s.current = nil
		return errors.New("stream completed without a response")
	}
	s.finalResponse = resp

	usage := resp.Usage
	if usage.InputTokens == 0 && usage.OutputTokens == 0 &&
		usage.CacheWriteTokens == 0 && usage.CacheReadTokens == 0 && len(usage.Details) == 0 {
		usage = s.current.Usage()
		resp.Usage = usage
	}

	_ = s.current.Close()
	s.current = nil

	s.state.usage.IncrRequest(usage)
	if usage.InputTokens > 0 {
		s.state.lastInputTokens = usage.InputTokens
	}
	if s.agent.costTracker != nil {
		singleUsage := RunUsage{}
		singleUsage.Incr(usage)
		s.agent.costTracker.Record(s.agent.model.ModelName(), singleUsage)
	}

	if len(s.agent.responseInterceptors) > 0 {
		if runResponseInterceptors(s.ctx, s.agent.responseInterceptors, resp) {
			s.completeActiveTurn(resp, nil)
			return nil
		}
	}

	if s.agent.toolChoiceAutoReset && len(resp.ToolCalls()) > 0 && s.settings != nil && s.settings.ToolChoice != nil {
		nextSettings := *s.settings
		nextSettings.ToolChoice = ToolChoiceAuto()
		s.settings = &nextSettings
	}

	s.state.messages = append(s.state.messages, *resp)
	s.agent.fireHook(func(h Hook) {
		if h.OnModelResponse != nil {
			h.OnModelResponse(s.ctx, s.currentModelRC, resp)
		}
	})

	if s.agent.eventBus != nil {
		Publish(s.agent.eventBus, ModelResponseCompletedEvent{
			RunID:        s.state.runID,
			ParentRunID:  s.state.parentRunID,
			TurnNumber:   s.state.runStep,
			FinishReason: string(resp.FinishReason),
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			HasToolCalls: len(resp.ToolCalls()) > 0,
			HasText:      resp.TextContent() != "",
			DurationMs:   time.Since(s.currentReqStart).Milliseconds(),
			CompletedAt:  time.Now(),
		})
	}
	if s.agent.tracingEnabled {
		s.state.traceSteps = append(s.state.traceSteps, TraceStep{
			Kind:      TraceModelResponse,
			Timestamp: time.Now(),
			Duration:  time.Since(s.currentReqStart),
			Data:      map[string]any{"text": resp.TextContent(), "tool_calls": len(resp.ToolCalls())},
		})
	}

	if s.limits.HasTokenLimits() {
		if err := s.limits.CheckTokens(s.state.usage); err != nil {
			s.completeActiveTurn(resp, err)
			return err
		}
	}

	if len(s.agent.runConditions) > 0 {
		condRC := s.agent.buildRunContext(s.state, s.deps, s.prompt)
		for _, cond := range s.agent.runConditions {
			if stop, reason := cond(s.ctx, condRC, resp); stop {
				s.agent.fireHook(func(h Hook) {
					if h.OnRunConditionChecked != nil {
						h.OnRunConditionChecked(s.ctx, condRC, true, reason)
					}
				})
				if hasText := resp.TextContent() != ""; hasText && s.agent.outputSchema.AllowsText {
					text := resp.TextContent()
					output, parseErr := deserializeOutput[T](text, s.agent.outputSchema.OuterTypedDictKey)
					if parseErr != nil && s.agent.outputSchema.Mode == OutputModeText {
						if textOutput, ok := any(text).(T); ok {
							output = textOutput
							parseErr = nil
						}
					}
					if parseErr == nil {
						s.completeActiveTurn(resp, nil)
						s.finish(&RunResult[T]{
							Output:    output,
							Messages:  s.state.messages,
							Usage:     s.state.usage,
							RunID:     s.state.runID,
							ToolState: s.agent.exportToolState(),
						}, nil)
						return nil
					}
				}
				condErr := &RunConditionError{Reason: reason}
				s.completeActiveTurn(resp, condErr)
				return condErr
			}
		}
	}

	result, nextParts, deferredReqs, err := s.agent.processResponse(s.ctx, s.state, resp, s.toolMap, s.outNames, s.deps, s.prompt)

	s.agent.fireHook(func(h Hook) {
		if h.OnTurnEnd != nil {
			h.OnTurnEnd(s.ctx, s.currentTurnRC, s.state.runStep, resp)
		}
	})

	if err != nil {
		s.completeActiveTurn(resp, err)
		return err
	}
	if len(deferredReqs) > 0 {
		s.completeActiveTurn(resp, nil)
		if s.agent.eventBus != nil {
			for _, dr := range deferredReqs {
				Publish(s.agent.eventBus, DeferredRequestedEvent{
					RunID:       s.state.runID,
					ParentRunID: s.state.parentRunID,
					ToolCallID:  dr.ToolCallID,
					ToolName:    dr.ToolName,
					ArgsJSON:    dr.ArgsJSON,
					RequestedAt: time.Now(),
				})
			}
			Publish(s.agent.eventBus, RunWaitingEvent{
				RunID:       s.state.runID,
				ParentRunID: s.state.parentRunID,
				Reason:      "deferred",
				WaitingAt:   time.Now(),
			})
		}
		if len(nextParts) > 0 {
			s.state.messages = append(s.state.messages, ModelRequest{
				Parts:     nextParts,
				Timestamp: time.Now(),
			})
		}
		return &ErrDeferred[T]{
			Result: RunResultDeferred[T]{
				DeferredRequests: deferredReqs,
				Messages:         s.state.messages,
				Usage:            s.state.usage,
			},
		}
	}
	if result != nil {
		if s.agent.kbAutoStore && s.agent.knowledgeBase != nil {
			responseText := resp.TextContent()
			if responseText != "" {
				if storeErr := s.agent.knowledgeBase.Store(s.ctx, responseText); storeErr != nil {
					kbErr := fmt.Errorf("knowledge base store failed: %w", storeErr)
					s.completeActiveTurn(resp, kbErr)
					return kbErr
				}
			}
		}
		s.completeActiveTurn(resp, nil)
		s.finish(&RunResult[T]{
			Output:    result.output,
			Messages:  s.state.messages,
			Usage:     s.state.usage,
			RunID:     s.state.runID,
			ToolState: s.agent.exportToolState(),
		}, nil)
		return nil
	}

	// No result — tool calls processed, continue to next turn.
	s.completeActiveTurn(resp, nil)

	if len(nextParts) > 0 {
		s.state.messages = append(s.state.messages, ModelRequest{
			Parts:     nextParts,
			Timestamp: time.Now(),
		})
	}

	return nil
}

func (s *agentStream[T]) finish(result *RunResult[T], runErr error) {
	if s.current != nil {
		_ = s.current.Close()
		s.current = nil
	}

	// If a turn is active (startTurn was called but completeTurn hasn't
	// cleared currentTurnRC), emit TurnCompleted so the turn lifecycle
	// is always balanced. This handles mid-stream errors and early Close.
	if s.currentTurnRC != nil {
		s.emitStreamTurnCompleted(s.finalResponse, runErr)
		s.currentTurnRC = nil
		s.currentModelRC = nil
	}

	s.done = true
	s.result = result
	s.err = runErr

	s.endOnce.Do(func() {
		s.agent.endRun(s.ctx, s.state, s.deps, s.prompt, runErr)

		if s.agent.tracingEnabled {
			trace := buildRunTrace(s.state, s.prompt, runErr)
			if s.result != nil {
				s.result.Trace = trace
			}
			for _, exporter := range s.agent.traceExporters {
				_ = exporter.Export(s.ctx, trace)
			}
		}

		if s.result != nil && s.agent.costTracker != nil {
			s.result.Cost = s.agent.costTracker.buildRunCost()
		}
	})
}
