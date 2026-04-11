package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// RequestTrace captures one actual outbound model request after all middleware
// and request-shaping logic have been applied.
type RequestTrace struct {
	RequestID         string                   `json:"request_id"`
	TurnNumber        int                      `json:"turn_number"`
	Sequence          int                      `json:"sequence"`
	ModelName         string                   `json:"model_name,omitempty"`
	StartedAt         time.Time                `json:"started_at"`
	EndedAt           time.Time                `json:"ended_at,omitempty"`
	Duration          time.Duration            `json:"duration"`
	MessageCount      int                      `json:"message_count"`
	FunctionToolCount int                      `json:"function_tool_count,omitempty"`
	OutputToolCount   int                      `json:"output_tool_count,omitempty"`
	Messages          []SerializedMessage      `json:"messages,omitempty"`
	Settings          *ModelSettings           `json:"settings,omitempty"`
	Parameters        *RequestTraceParameters  `json:"parameters,omitempty"`
	Compactions       []ContextCompactionTrace `json:"compactions,omitempty"`
	Response          *RequestTraceResponse    `json:"response,omitempty"`
	Error             string                   `json:"error,omitempty"`
}

// RequestTraceParameters is the JSON-safe, trace-oriented snapshot of the
// effective request parameters sent to the model.
type RequestTraceParameters struct {
	FunctionTools   []ToolDefinition          `json:"function_tools,omitempty"`
	OutputMode      OutputMode                `json:"output_mode,omitempty"`
	OutputTools     []ToolDefinition          `json:"output_tools,omitempty"`
	OutputObject    *RequestTraceOutputObject `json:"output_object,omitempty"`
	AllowTextOutput bool                      `json:"allow_text_output,omitempty"`
}

// RequestTraceOutputObject captures the effective output object schema.
type RequestTraceOutputObject struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	JSONSchema  Schema `json:"json_schema,omitempty"`
	Strict      *bool  `json:"strict,omitempty"`
}

// RequestTraceResponse captures the provider response associated with a
// RequestTrace.
type RequestTraceResponse struct {
	ModelName    string             `json:"model_name,omitempty"`
	FinishReason FinishReason       `json:"finish_reason,omitempty"`
	Usage        Usage              `json:"usage,omitempty"`
	Message      *SerializedMessage `json:"message,omitempty"`
}

// ContextCompactionTrace records one compaction event that affected the next
// outbound request.
type ContextCompactionTrace struct {
	OccurredAt     time.Time `json:"occurred_at"`
	Strategy       string    `json:"strategy"`
	MessagesBefore int       `json:"messages_before"`
	MessagesAfter  int       `json:"messages_after"`
}

func newRequestTraceMiddleware(state *RunState, modelName string) AgentMiddleware {
	return DualMiddleware(
		func(
			ctx context.Context,
			messages []ModelMessage,
			settings *ModelSettings,
			params *ModelRequestParameters,
			next func(context.Context, []ModelMessage, *ModelSettings, *ModelRequestParameters) (*ModelResponse, error),
		) (*ModelResponse, error) {
			trace := state.beginRequestTrace(modelName, messages, settings, params)
			resp, err := next(ctx, messages, settings, params)
			state.completeRequestTrace(trace, resp, err)
			return resp, err
		},
		func(
			ctx context.Context,
			messages []ModelMessage,
			settings *ModelSettings,
			params *ModelRequestParameters,
			next AgentStreamFunc,
		) (StreamedResponse, error) {
			trace := state.beginRequestTrace(modelName, messages, settings, params)
			stream, err := next(ctx, messages, settings, params)
			if err != nil {
				state.completeRequestTrace(trace, nil, err)
				return nil, err
			}
			return &requestTraceStreamWrapper{
				inner: stream,
				onDone: func(resp *ModelResponse, streamErr error) {
					state.completeRequestTrace(trace, resp, streamErr)
				},
			}, nil
		},
	)
}

func buildRunTrace(state *RunState, prompt string, runErr error) *RunTrace {
	if state == nil {
		return nil
	}

	state.mu.Lock()
	runID := state.runID
	startTime := state.startTime
	usage := state.usage
	steps := append([]TraceStep(nil), state.traceSteps...)
	requests := cloneRequestTraces(state.requestTraces)
	state.mu.Unlock()

	endTime := time.Now()
	trace := &RunTrace{
		RunID:     runID,
		Prompt:    prompt,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime),
		Steps:     steps,
		Requests:  requests,
		Usage:     usage,
		Success:   runErr == nil,
	}
	if runErr != nil {
		trace.Error = runErr.Error()
	}
	return trace
}

func (s *RunState) recordCompaction(stats ContextCompactionStats) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingCompactions = append(s.pendingCompactions, ContextCompactionTrace{
		OccurredAt:     time.Now(),
		Strategy:       stats.Strategy,
		MessagesBefore: stats.MessagesBefore,
		MessagesAfter:  stats.MessagesAfter,
	})
}

func (s *RunState) beginRequestTrace(
	modelName string,
	messages []ModelMessage,
	settings *ModelSettings,
	params *ModelRequestParameters,
) *RequestTrace {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	sequence := len(s.requestTraces) + 1
	runID := s.runID
	turnNumber := s.runStep
	compactions := cloneCompactionTraces(s.pendingCompactions)
	s.pendingCompactions = nil
	s.mu.Unlock()

	trace := &RequestTrace{
		RequestID:    fmt.Sprintf("%s/request-%d", runID, sequence),
		TurnNumber:   turnNumber,
		Sequence:     sequence,
		ModelName:    modelName,
		StartedAt:    time.Now(),
		MessageCount: len(messages),
		Compactions:  compactions,
		Settings:     cloneModelSettings(settings),
		Parameters:   cloneRequestTraceParameters(params),
	}
	if trace.Parameters != nil {
		trace.FunctionToolCount = len(trace.Parameters.FunctionTools)
		trace.OutputToolCount = len(trace.Parameters.OutputTools)
	}

	if encoded, err := EncodeMessages(messages); err == nil {
		trace.Messages = encoded
	}

	return trace
}

func (s *RunState) completeRequestTrace(trace *RequestTrace, resp *ModelResponse, err error) {
	if s == nil || trace == nil {
		return
	}

	trace.EndedAt = time.Now()
	trace.Duration = trace.EndedAt.Sub(trace.StartedAt)
	if err != nil {
		trace.Error = err.Error()
	}
	if resp != nil {
		responseTrace := &RequestTraceResponse{
			ModelName:    resp.ModelName,
			FinishReason: resp.FinishReason,
			Usage:        cloneUsage(resp.Usage),
		}
		if encoded, encErr := EncodeModelResponse(resp); encErr == nil {
			responseTrace.Message = encoded
		}
		trace.Response = responseTrace
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.requestTraces = append(s.requestTraces, *cloneRequestTrace(trace))
}

func cloneRequestTraceParameters(params *ModelRequestParameters) *RequestTraceParameters {
	if params == nil {
		return nil
	}
	return &RequestTraceParameters{
		FunctionTools:   cloneToolDefinitionsForTrace(params.FunctionTools),
		OutputMode:      params.OutputMode,
		OutputTools:     cloneToolDefinitionsForTrace(params.OutputTools),
		OutputObject:    cloneTraceOutputObject(params.OutputObject),
		AllowTextOutput: params.AllowTextOutput,
	}
}

func cloneTraceOutputObject(src *OutputObjectDefinition) *RequestTraceOutputObject {
	if src == nil {
		return nil
	}
	cloned := &RequestTraceOutputObject{
		Name:        src.Name,
		Description: src.Description,
		JSONSchema:  cloneSchemaForTrace(src.JSONSchema),
	}
	if src.Strict != nil {
		strict := *src.Strict
		cloned.Strict = &strict
	}
	return cloned
}

func cloneUsage(src Usage) Usage {
	cloned := src
	if len(src.Details) > 0 {
		cloned.Details = make(map[string]int, len(src.Details))
		for key, value := range src.Details {
			cloned.Details[key] = value
		}
	}
	return cloned
}

func cloneRequestTraces(src []RequestTrace) []RequestTrace {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]RequestTrace, len(src))
	for i := range src {
		cloned[i] = *cloneRequestTrace(&src[i])
	}
	return cloned
}

func cloneRequestTrace(src *RequestTrace) *RequestTrace {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.Messages = cloneSerializedMessages(src.Messages)
	cloned.Settings = cloneModelSettings(src.Settings)
	cloned.Parameters = cloneRequestTraceParametersToCopy(src.Parameters)
	cloned.Compactions = cloneCompactionTraces(src.Compactions)
	cloned.Response = cloneRequestTraceResponse(src.Response)
	return &cloned
}

func cloneRequestTraceParametersToCopy(src *RequestTraceParameters) *RequestTraceParameters {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.FunctionTools = cloneToolDefinitionsForTrace(src.FunctionTools)
	cloned.OutputTools = cloneToolDefinitionsForTrace(src.OutputTools)
	cloned.OutputObject = cloneTraceOutputObjectToCopy(src.OutputObject)
	return &cloned
}

func cloneTraceOutputObjectToCopy(src *RequestTraceOutputObject) *RequestTraceOutputObject {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.JSONSchema = cloneSchemaForTrace(src.JSONSchema)
	if src.Strict != nil {
		strict := *src.Strict
		cloned.Strict = &strict
	}
	return &cloned
}

func cloneRequestTraceResponse(src *RequestTraceResponse) *RequestTraceResponse {
	if src == nil {
		return nil
	}
	cloned := *src
	cloned.Usage = cloneUsage(src.Usage)
	if src.Message != nil {
		message := *src.Message
		message.Data = append(json.RawMessage(nil), src.Message.Data...)
		cloned.Message = &message
	}
	return &cloned
}

func cloneCompactionTraces(src []ContextCompactionTrace) []ContextCompactionTrace {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]ContextCompactionTrace, len(src))
	copy(cloned, src)
	return cloned
}

func cloneSerializedMessages(src []SerializedMessage) []SerializedMessage {
	if len(src) == 0 {
		return nil
	}
	cloned := make([]SerializedMessage, len(src))
	for i := range src {
		cloned[i] = SerializedMessage{
			Kind: src[i].Kind,
			Data: append(json.RawMessage(nil), src[i].Data...),
		}
	}
	return cloned
}

func cloneToolDefinitionsForTrace(src []ToolDefinition) []ToolDefinition {
	if len(src) == 0 {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		cloned := append([]ToolDefinition(nil), src...)
		return cloned
	}
	var cloned []ToolDefinition
	if err := json.Unmarshal(data, &cloned); err != nil {
		return append([]ToolDefinition(nil), src...)
	}
	return cloned
}

func cloneSchemaForTrace(src Schema) Schema {
	if len(src) == 0 {
		return nil
	}
	data, err := json.Marshal(src)
	if err != nil {
		return nil
	}
	var cloned Schema
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

type requestTraceStreamWrapper struct {
	inner    StreamedResponse
	onDone   func(resp *ModelResponse, err error)
	doneOnce sync.Once
	mu       sync.Mutex
	done     bool
}

func (w *requestTraceStreamWrapper) Next() (ModelResponseStreamEvent, error) {
	event, err := w.inner.Next()
	if err != nil {
		w.finalize(err)
	}
	return event, err
}

func (w *requestTraceStreamWrapper) Response() *ModelResponse {
	return w.inner.Response()
}

func (w *requestTraceStreamWrapper) Usage() Usage {
	return w.inner.Usage()
}

func (w *requestTraceStreamWrapper) Close() error {
	err := w.inner.Close()
	if err != nil {
		w.finalize(err)
		return err
	}
	if w.isDone() {
		return nil
	}
	w.finalize(errStreamClosed)
	return errStreamClosed
}

func (w *requestTraceStreamWrapper) finalize(err error) {
	w.doneOnce.Do(func() {
		w.mu.Lock()
		w.done = true
		w.mu.Unlock()
		if errors.Is(err, io.EOF) {
			if w.inner.Response() != nil {
				err = nil
			} else {
				err = errors.New("stream completed without a response")
			}
		}
		if w.onDone != nil {
			w.onDone(w.inner.Response(), err)
		}
	})
}

func (w *requestTraceStreamWrapper) isDone() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.done
}
