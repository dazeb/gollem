package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
)

var ErrInteractionRequestFailed = errors.New("appserver: interaction request failed")

const (
	InteractionRequestUserInput = "item/tool/requestUserInput"
	InteractionToolCall         = "item/tool/call"
	InteractionMCPElicitation   = "mcpServer/elicitation/request"
)

// InteractionService publishes runtime server-to-client interaction requests
// and resolves them from JSON-RPC responses sent by the client.
type InteractionService struct {
	mu       sync.Mutex
	counter  int64
	pending  map[string]pendingInteraction
	requests *RequestQueue
}

type pendingInteraction struct {
	meta InteractionRequestMeta
	ch   chan InteractionResponse
}

type InteractionRequest struct {
	Method    string         `json:"method"`
	RequestID string         `json:"requestId,omitempty"`
	ThreadID  string         `json:"threadId,omitempty"`
	TurnID    string         `json:"turnId,omitempty"`
	ItemID    string         `json:"itemId,omitempty"`
	Reason    string         `json:"reason,omitempty"`
	Params    map[string]any `json:"params,omitempty"`
}

type InteractionRequestMeta struct {
	RequestID string `json:"requestId"`
	Method    string `json:"method"`
	ThreadID  string `json:"threadId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	ItemID    string `json:"itemId,omitempty"`
}

type InteractionResponse struct {
	InteractionRequestMeta
	Result json.RawMessage `json:"result,omitempty"`
	Error  *protocol.Error `json:"error,omitempty"`
}

type UserInputRequest struct {
	ThreadID    string         `json:"threadId,omitempty"`
	TurnID      string         `json:"turnId,omitempty"`
	ItemID      string         `json:"itemId,omitempty"`
	Prompt      string         `json:"prompt"`
	Placeholder string         `json:"placeholder,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Options     []string       `json:"options,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type DynamicToolCallRequest struct {
	ThreadID  string          `json:"threadId,omitempty"`
	TurnID    string          `json:"turnId,omitempty"`
	ItemID    string          `json:"itemId,omitempty"`
	CallID    string          `json:"callId,omitempty"`
	Namespace *string         `json:"namespace"`
	ToolName  string          `json:"toolName"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Metadata  map[string]any  `json:"metadata,omitempty"`
}

type MCPElicitationRequest struct {
	ThreadID string         `json:"threadId,omitempty"`
	TurnID   string         `json:"turnId,omitempty"`
	ItemID   string         `json:"itemId,omitempty"`
	ServerID string         `json:"serverId,omitempty"`
	Message  string         `json:"message"`
	Schema   map[string]any `json:"schema,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func NewInteractionService() *InteractionService {
	return &InteractionService{
		pending:  make(map[string]pendingInteraction),
		requests: NewRequestQueue(),
	}
}

func (s *InteractionService) setRequestQueue(q *RequestQueue) {
	if s == nil || q == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = q
}

func (s *InteractionService) RequestUserInput(ctx context.Context, req UserInputRequest) (InteractionResponse, error) {
	params := map[string]any{
		"prompt":      req.Prompt,
		"placeholder": req.Placeholder,
		"required":    req.Required,
		"options":     append([]string(nil), req.Options...),
		"metadata":    cloneStringAnyMap(req.Metadata),
	}
	return s.Request(ctx, InteractionRequest{
		Method:   InteractionRequestUserInput,
		ThreadID: req.ThreadID,
		TurnID:   req.TurnID,
		ItemID:   req.ItemID,
		Reason:   req.Prompt,
		Params:   params,
	})
}

func (s *InteractionService) RequestToolCall(ctx context.Context, req DynamicToolCallRequest) (InteractionResponse, error) {
	params := map[string]any{
		"callId":    strings.TrimSpace(req.CallID),
		"namespace": req.Namespace,
		"tool":      req.ToolName,
		"toolName":  req.ToolName,
		"name":      req.ToolName,
		"arguments": json.RawMessage(append([]byte(nil), req.Arguments...)),
		"metadata":  cloneStringAnyMap(req.Metadata),
	}
	return s.Request(ctx, InteractionRequest{
		Method:   InteractionToolCall,
		ThreadID: req.ThreadID,
		TurnID:   req.TurnID,
		ItemID:   req.ItemID,
		Reason:   req.ToolName,
		Params:   params,
	})
}

func (s *InteractionService) RequestMCPElicitation(ctx context.Context, req MCPElicitationRequest) (InteractionResponse, error) {
	params := map[string]any{
		"serverId": req.ServerID,
		"message":  req.Message,
		"schema":   cloneStringAnyMap(req.Schema),
		"metadata": cloneStringAnyMap(req.Metadata),
	}
	return s.Request(ctx, InteractionRequest{
		Method:   InteractionMCPElicitation,
		ThreadID: req.ThreadID,
		TurnID:   req.TurnID,
		ItemID:   req.ItemID,
		Reason:   req.Message,
		Params:   params,
	})
}

func (s *InteractionService) Request(ctx context.Context, req InteractionRequest) (InteractionResponse, error) {
	if s == nil || s.requests == nil {
		return InteractionResponse{}, errors.New("interaction service is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	method := strings.TrimSpace(req.Method)
	if !isSupportedInteractionMethod(method) {
		return InteractionResponse{}, fmt.Errorf("unsupported interaction method %q", req.Method)
	}
	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = s.nextRequestID()
	}
	itemID := strings.TrimSpace(req.ItemID)
	if itemID == "" {
		itemID = requestID
	}
	meta := InteractionRequestMeta{
		RequestID: requestID,
		Method:    method,
		ThreadID:  strings.TrimSpace(req.ThreadID),
		TurnID:    strings.TrimSpace(req.TurnID),
		ItemID:    itemID,
	}
	payload := cloneStringAnyMap(req.Params)
	payload["requestId"] = requestID
	payload["threadId"] = meta.ThreadID
	payload["turnId"] = meta.TurnID
	payload["itemId"] = meta.ItemID
	payload["startedAtMs"] = time.Now().UnixMilli()
	if method == InteractionToolCall {
		callID, _ := payload["callId"].(string)
		if strings.TrimSpace(callID) == "" {
			payload["callId"] = firstNonEmpty(meta.ItemID, requestID)
		}
	}
	if strings.TrimSpace(req.Reason) != "" {
		payload["reason"] = strings.TrimSpace(req.Reason)
	}

	pending := pendingInteraction{meta: meta, ch: make(chan InteractionResponse, 1)}
	s.mu.Lock()
	if s.pending == nil {
		s.pending = make(map[string]pendingInteraction)
	}
	if _, exists := s.pending[requestID]; exists {
		s.mu.Unlock()
		return InteractionResponse{}, fmt.Errorf("interaction request %q is already pending", requestID)
	}
	s.pending[requestID] = pending
	s.mu.Unlock()

	s.requests.Publish(method, protocol.NewStringID(requestID), payload)
	select {
	case response := <-pending.ch:
		if response.Error != nil {
			return response, fmt.Errorf("%w: %s", ErrInteractionRequestFailed, response.Error.Message)
		}
		return response, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, requestID)
		s.mu.Unlock()
		return InteractionResponse{}, ctx.Err()
	}
}

func (s *InteractionService) Respond(resp protocol.Response) (InteractionResponse, bool, error) {
	if s == nil {
		return InteractionResponse{}, false, errors.New("interaction service is not configured")
	}
	requestID := requestIDString(resp.ID)
	if requestID == "" {
		return InteractionResponse{}, false, errors.New("response id is required")
	}
	s.mu.Lock()
	pending, ok := s.pending[requestID]
	if ok {
		delete(s.pending, requestID)
	}
	s.mu.Unlock()
	if !ok {
		return InteractionResponse{}, false, nil
	}
	response := InteractionResponse{
		InteractionRequestMeta: pending.meta,
		Result:                 append(json.RawMessage(nil), resp.Result...),
		Error:                  resp.Error,
	}
	var responseErr error
	if pending.meta.Method == InteractionToolCall && response.Error == nil {
		if len(response.Result) > runtimeInteractionPayloadMaxBytes {
			responseErr = fmt.Errorf("dynamic tool call response exceeds %d bytes", runtimeInteractionPayloadMaxBytes)
		} else {
			var result protocol.DynamicToolCallResponse
			if err := json.Unmarshal(response.Result, &result); err != nil {
				responseErr = fmt.Errorf("decode dynamic tool call response: %w", err)
			}
		}
		if responseErr != nil {
			response.Error = &protocol.Error{Code: protocol.CodeInvalidParams, Message: "invalid dynamic tool call response"}
			response.Result = nil
		}
	}
	pending.ch <- response
	return response, true, responseErr
}

func (s *InteractionService) nextRequestID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	return fmt.Sprintf("interaction-%d", s.counter)
}

func isSupportedInteractionMethod(method string) bool {
	switch method {
	case InteractionRequestUserInput, InteractionToolCall, InteractionMCPElicitation:
		return true
	default:
		return false
	}
}

func requestIDString(id protocol.RequestID) string {
	switch value := id.Value().(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		if value == nil {
			return ""
		}
		return fmt.Sprint(value)
	}
}

func cloneStringAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
