package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/appserver/store"
	"github.com/fugue-labs/gollem/core"
)

var (
	ErrRuntimeNotConfigured = errors.New("appserver/runtime: model factory is not configured")
	ErrRuntimeTurnActive    = errors.New("appserver/runtime: turn is already running")
	ErrRuntimeTurnNotActive = errors.New("appserver/runtime: turn is not running")
	ErrRuntimePromptEmpty   = errors.New("appserver/runtime: prompt is required")
)

type RuntimeModelSelection struct {
	ProviderID string `json:"providerId,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
}

type RuntimeModelInfo struct {
	ProviderID string `json:"providerId,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
}

type RuntimeModelFactory func(context.Context, RuntimeModelSelection) (core.Model, RuntimeModelInfo, error)

type RuntimeOption func(*RuntimeService)

func WithRuntimeModelFactory(factory RuntimeModelFactory) RuntimeOption {
	return func(s *RuntimeService) {
		s.modelFactory = factory
	}
}

func WithRuntimeModel(model core.Model, info RuntimeModelInfo) RuntimeOption {
	return WithRuntimeModelFactory(func(context.Context, RuntimeModelSelection) (core.Model, RuntimeModelInfo, error) {
		if model == nil {
			return nil, RuntimeModelInfo{}, ErrRuntimeNotConfigured
		}
		if info.Model == "" {
			info.Model = model.ModelName()
		}
		return model, info, nil
	})
}

type RuntimeService struct {
	mu           sync.Mutex
	modelFactory RuntimeModelFactory
	active       map[string]*activeRuntimeTurn
}

func NewRuntimeService(opts ...RuntimeOption) *RuntimeService {
	s := &RuntimeService{
		active: make(map[string]*activeRuntimeTurn),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

type RuntimeStartRequest struct {
	ThreadID      string
	Prompt        string
	Input         json.RawMessage
	Metadata      map[string]any
	Selection     RuntimeModelSelection
	ModelSettings core.ModelSettings
	History       []core.ModelMessage
}

type RuntimeStartResult struct {
	Turn *store.Turn `json:"turn"`
}

type RuntimeInterruptResult struct {
	OK     bool        `json:"ok"`
	TurnID string      `json:"turnId"`
	Turn   *store.Turn `json:"turn,omitempty"`
}

type runtimeNotifier interface {
	PublishNotification(method string, params any)
}

type activeRuntimeTurn struct {
	cancel context.CancelFunc
}

func (s *RuntimeService) Start(ctx context.Context, st store.Store, notifier runtimeNotifier, req RuntimeStartRequest) (*RuntimeStartResult, error) {
	if s == nil || s.modelFactory == nil {
		return nil, ErrRuntimeNotConfigured
	}
	if st == nil {
		return nil, ErrRuntimeNotConfigured
	}
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.Prompt == "" {
		return nil, ErrRuntimePromptEmpty
	}
	if len(req.Input) == 0 {
		input, err := json.Marshal(runtimeTurnInput{
			Prompt:      req.Prompt,
			ProviderID:  req.Selection.ProviderID,
			Provider:    req.Selection.Provider,
			Model:       req.Selection.Model,
			Metadata:    cloneRuntimeMap(req.Metadata),
			SubmittedAt: time.Now().UTC(),
		})
		if err != nil {
			return nil, fmt.Errorf("marshal runtime input: %w", err)
		}
		req.Input = input
	}
	turn, err := st.CreateTurn(ctx, store.CreateTurnRequest{
		ThreadID: req.ThreadID,
		Input:    cloneRuntimeRaw(req.Input),
		Metadata: cloneRuntimeMap(req.Metadata),
	})
	if err != nil {
		return nil, err
	}
	if _, err := st.AppendItem(ctx, store.AppendItemRequest{
		ThreadID: turn.ThreadID,
		TurnID:   turn.ID,
		Kind:     "message",
		Status:   "completed",
		Payload:  mustRuntimeJSON(runtimeMessagePayload{Role: "user", Text: req.Prompt, CreatedAt: time.Now().UTC()}),
	}); err != nil {
		return nil, err
	}
	started, err := st.StartTurn(ctx, turn.ID)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	if _, exists := s.active[started.ID]; exists {
		s.mu.Unlock()
		cancel()
		return nil, ErrRuntimeTurnActive
	}
	s.active[started.ID] = &activeRuntimeTurn{cancel: cancel}
	s.mu.Unlock()

	publishTurnStarted(notifier, started)
	go s.run(runCtx, st, notifier, started, req)
	return &RuntimeStartResult{Turn: started}, nil
}

func (s *RuntimeService) Interrupt(ctx context.Context, st store.Store, turnID string) (*RuntimeInterruptResult, error) {
	if s == nil {
		return nil, ErrRuntimeNotConfigured
	}
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return nil, store.ErrTurnNotFound
	}
	s.mu.Lock()
	active := s.active[turnID]
	s.mu.Unlock()
	if active == nil {
		turn, err := st.GetTurn(ctx, turnID)
		if err != nil {
			return nil, err
		}
		return &RuntimeInterruptResult{OK: false, TurnID: turnID, Turn: turn}, ErrRuntimeTurnNotActive
	}
	active.cancel()
	turn, _ := st.GetTurn(ctx, turnID)
	return &RuntimeInterruptResult{OK: true, TurnID: turnID, Turn: turn}, nil
}

func (s *RuntimeService) IsActive(turnID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.active[turnID]
	return ok
}

func (s *RuntimeService) run(ctx context.Context, st store.Store, notifier runtimeNotifier, turn *store.Turn, req RuntimeStartRequest) {
	defer func() {
		s.mu.Lock()
		delete(s.active, turn.ID)
		s.mu.Unlock()
	}()

	model, info, err := s.modelFactory(ctx, req.Selection)
	if err != nil {
		s.complete(st, notifier, turn, store.TurnFailed, nil, err, info)
		return
	}
	defer closeRuntimeModel(model)
	if info.Model == "" && model != nil {
		info.Model = model.ModelName()
	}

	bus := core.NewEventBus()
	defer bus.Close()
	unsubscribeDelta := core.Subscribe(bus, func(event core.ModelDeltaEvent) {
		publishModelDelta(notifier, turn, event)
	})
	defer unsubscribeDelta()
	unsubscribeError := core.Subscribe(bus, func(event core.ErrorRaisedEvent) {
		publishRuntimeError(notifier, turn, event.Error)
	})
	defer unsubscribeError()

	agent := core.NewAgent[string](model, core.WithEventBus[string](bus))
	runOpts := make([]core.RunOption, 0, 2)
	if len(req.History) > 0 {
		runOpts = append(runOpts, core.WithMessages(req.History...))
	}
	if hasRuntimeModelSettings(req.ModelSettings) {
		runOpts = append(runOpts, core.WithRunModelSettings(req.ModelSettings))
	}
	stream, err := agent.RunStream(ctx, req.Prompt, runOpts...)
	if err != nil {
		s.complete(st, notifier, turn, statusFromRuntimeError(err), nil, err, info)
		return
	}
	defer stream.Close()

	var streamErr error
	for _, err := range stream.StreamEvents() {
		if err != nil {
			streamErr = err
			break
		}
	}
	result, err := stream.Result()
	if err == nil {
		err = streamErr
	}
	s.complete(st, notifier, turn, statusFromRuntimeError(err), result, err, info)
}

func (s *RuntimeService) complete(st store.Store, notifier runtimeNotifier, turn *store.Turn, status store.TurnStatus, result *core.RunResult[string], runErr error, info RuntimeModelInfo) {
	resultPayload := runtimeResultPayload{
		ProviderID:  info.ProviderID,
		Provider:    info.Provider,
		Model:       info.Model,
		CompletedAt: time.Now().UTC(),
	}
	var usage map[string]any
	if result != nil {
		resultPayload.Output = result.Output
		resultPayload.RunID = result.RunID
		resultPayload.Text = lastRuntimeAssistantText(result.Messages)
		resultPayload.ToolState = result.ToolState
		usage = runtimeUsageMap(result.Usage)
	}
	if resultPayload.Text != "" {
		item, err := st.AppendItem(context.Background(), store.AppendItemRequest{
			ThreadID: turn.ThreadID,
			TurnID:   turn.ID,
			Kind:     "message",
			Status:   "completed",
			Payload: mustRuntimeJSON(runtimeMessagePayload{
				Role:      "assistant",
				Text:      resultPayload.Text,
				Model:     info.Model,
				Provider:  firstRuntimeNonEmpty(info.ProviderID, info.Provider),
				CreatedAt: time.Now().UTC(),
			}),
		})
		if err == nil {
			publishItemCompleted(notifier, turn, item)
		}
	}
	var rawResult json.RawMessage
	if payload, err := json.Marshal(resultPayload); err == nil {
		rawResult = payload
	}
	errorText := ""
	if runErr != nil {
		errorText = runErr.Error()
	}
	completed, err := st.CompleteTurn(context.Background(), store.CompleteTurnRequest{
		ID:     turn.ID,
		Status: status,
		Result: rawResult,
		Error:  errorText,
		Usage:  usage,
	})
	if err == nil {
		publishTurnCompleted(notifier, completed)
	}
}

func statusFromRuntimeError(err error) store.TurnStatus {
	if err == nil {
		return store.TurnCompleted
	}
	if errors.Is(err, context.Canceled) {
		return store.TurnInterrupted
	}
	return store.TurnFailed
}

type runtimeTurnInput struct {
	Prompt      string         `json:"prompt"`
	ProviderID  string         `json:"providerId,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	Model       string         `json:"model,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	SubmittedAt time.Time      `json:"submittedAt"`
}

type runtimeResultPayload struct {
	Output      string         `json:"output,omitempty"`
	Text        string         `json:"text,omitempty"`
	RunID       string         `json:"runId,omitempty"`
	ProviderID  string         `json:"providerId,omitempty"`
	Provider    string         `json:"provider,omitempty"`
	Model       string         `json:"model,omitempty"`
	ToolState   map[string]any `json:"toolState,omitempty"`
	CompletedAt time.Time      `json:"completedAt"`
}

type runtimeMessagePayload struct {
	Role      string    `json:"role"`
	Text      string    `json:"text,omitempty"`
	Model     string    `json:"model,omitempty"`
	Provider  string    `json:"provider,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type turnNotificationParams struct {
	ThreadID string           `json:"threadId"`
	TurnID   string           `json:"turnId"`
	Status   store.TurnStatus `json:"status,omitempty"`
	Turn     *store.Turn      `json:"turn,omitempty"`
	At       time.Time        `json:"at"`
}

type runtimeItemNotificationParams struct {
	ThreadID string      `json:"threadId"`
	TurnID   string      `json:"turnId,omitempty"`
	ItemID   string      `json:"itemId,omitempty"`
	Item     *store.Item `json:"item,omitempty"`
	At       time.Time   `json:"at"`
}

type runtimeDeltaNotificationParams struct {
	ThreadID string    `json:"threadId"`
	TurnID   string    `json:"turnId"`
	Delta    string    `json:"delta"`
	Index    int       `json:"index,omitempty"`
	At       time.Time `json:"at"`
}

type runtimeErrorNotificationParams struct {
	ThreadID string    `json:"threadId,omitempty"`
	TurnID   string    `json:"turnId,omitempty"`
	Error    string    `json:"error"`
	At       time.Time `json:"at"`
}

func publishTurnStarted(notifier runtimeNotifier, turn *store.Turn) {
	if notifier == nil || turn == nil {
		return
	}
	notifier.PublishNotification("turn/started", turnNotificationParams{
		ThreadID: turn.ThreadID,
		TurnID:   turn.ID,
		Status:   turn.Status,
		Turn:     turn,
		At:       time.Now().UTC(),
	})
}

func publishTurnCompleted(notifier runtimeNotifier, turn *store.Turn) {
	if notifier == nil || turn == nil {
		return
	}
	notifier.PublishNotification("turn/completed", turnNotificationParams{
		ThreadID: turn.ThreadID,
		TurnID:   turn.ID,
		Status:   turn.Status,
		Turn:     turn,
		At:       time.Now().UTC(),
	})
}

func publishItemCompleted(notifier runtimeNotifier, turn *store.Turn, item *store.Item) {
	if notifier == nil || turn == nil || item == nil {
		return
	}
	notifier.PublishNotification("item/completed", runtimeItemNotificationParams{
		ThreadID: turn.ThreadID,
		TurnID:   turn.ID,
		ItemID:   item.ID,
		Item:     item,
		At:       time.Now().UTC(),
	})
}

func publishModelDelta(notifier runtimeNotifier, turn *store.Turn, event core.ModelDeltaEvent) {
	if notifier == nil || turn == nil || event.ContentDelta == "" {
		return
	}
	method := "item/agentMessage/delta"
	if event.DeltaKind == "thinking" {
		method = "item/reasoning/textDelta"
	}
	notifier.PublishNotification(method, runtimeDeltaNotificationParams{
		ThreadID: turn.ThreadID,
		TurnID:   turn.ID,
		Delta:    event.ContentDelta,
		Index:    event.PartIndex,
		At:       time.Now().UTC(),
	})
}

func publishRuntimeError(notifier runtimeNotifier, turn *store.Turn, text string) {
	if notifier == nil || text == "" {
		return
	}
	params := runtimeErrorNotificationParams{Error: text, At: time.Now().UTC()}
	if turn != nil {
		params.ThreadID = turn.ThreadID
		params.TurnID = turn.ID
	}
	notifier.PublishNotification("error", params)
}

func runtimeUsageMap(usage core.RunUsage) map[string]any {
	return map[string]any{
		"requests": usage.Requests,
		"usage": map[string]any{
			"inputTokens":      usage.InputTokens,
			"outputTokens":     usage.OutputTokens,
			"cacheWriteTokens": usage.CacheWriteTokens,
			"cacheReadTokens":  usage.CacheReadTokens,
			"totalTokens":      usage.TotalTokens(),
		},
	}
}

func lastRuntimeAssistantText(messages []core.ModelMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if resp, ok := messages[i].(core.ModelResponse); ok {
			return resp.TextContent()
		}
	}
	return ""
}

func runtimeMessagesFromItems(items []*store.Item) []core.ModelMessage {
	messages := make([]core.ModelMessage, 0, len(items))
	for _, item := range items {
		if item == nil || len(item.Payload) == 0 {
			continue
		}
		if item.Kind == threadInjectedResponseItemKind {
			if message, ok := runtimeMessageFromInjectedResponseItem(item.Payload); ok {
				messages = append(messages, message)
			}
			continue
		}
		if item.Kind != "message" {
			continue
		}
		var payload runtimeMessagePayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			continue
		}
		switch payload.Role {
		case "user":
			messages = append(messages, core.ModelRequest{
				Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: payload.Text, Timestamp: payload.CreatedAt}},
				Timestamp: payload.CreatedAt,
			})
		case "assistant":
			messages = append(messages, core.ModelResponse{
				Parts:        []core.ModelResponsePart{core.TextPart{Content: payload.Text}},
				ModelName:    payload.Model,
				FinishReason: core.FinishReasonStop,
				Timestamp:    payload.CreatedAt,
			})
		}
	}
	return messages
}

func runtimePromptFromInput(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	var obj struct {
		Prompt  string `json:"prompt"`
		Message string `json:"message"`
		Text    string `json:"text"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(firstRuntimeNonEmpty(obj.Prompt, obj.Message, obj.Text))
}

func mustRuntimeJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}

func cloneRuntimeRaw(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out
}

func cloneRuntimeMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func firstRuntimeNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func closeRuntimeModel(model core.Model) {
	closer, ok := model.(interface{ Close() error })
	if !ok {
		return
	}
	_ = closer.Close()
}

func hasRuntimeModelSettings(settings core.ModelSettings) bool {
	return settings.MaxTokens != nil ||
		settings.Temperature != nil ||
		settings.TopP != nil ||
		settings.ToolChoice != nil ||
		settings.ThinkingBudget != nil ||
		settings.AdaptiveThinking != nil ||
		settings.ReasoningEffort != nil ||
		len(settings.StopSequences) > 0
}
