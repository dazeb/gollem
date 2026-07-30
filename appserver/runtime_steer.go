package appserver

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	"github.com/fugue-labs/gollem/core"
	"github.com/google/uuid"
)

const (
	runtimeSteerItemKind       = "steer"
	runtimeSteerMessageMaxSize = 64 << 10
	runtimeSteerIDMaxSize      = 256
	runtimeSteerStatusQueued   = "queued"
	runtimeSteerStatusComplete = "completed"
	runtimeSteerStatusFailed   = "failed"
)

var (
	ErrRuntimeSteerIdempotencyConflict = errors.New("appserver/runtime: steer idempotency conflict")
	ErrRuntimeSteerMessageTooLarge     = errors.New("appserver/runtime: steer message is too large")
	ErrRuntimeSteerIDTooLarge          = errors.New("appserver/runtime: steer client message ID is too large")
)

type RuntimeSteerRequest struct {
	ThreadID            string
	TurnID              string
	ClientUserMessageID string
	Message             string
}

type RuntimeSteerResult struct {
	TurnID              string
	ClientUserMessageID string
	Item                *store.Item
	Reused              bool
}

type runtimeSteerPayload struct {
	ClientUserMessageID string     `json:"clientUserMessageId"`
	Message             string     `json:"message"`
	Status              string     `json:"status"`
	QueuedAt            time.Time  `json:"queuedAt"`
	ConsumedAfterSeq    int64      `json:"consumedAfterSeq,omitempty"`
	ConsumedAt          *time.Time `json:"consumedAt,omitempty"`
	FailedAt            *time.Time `json:"failedAt,omitempty"`
	Error               string     `json:"error,omitempty"`
}

type runtimeSteerItemState struct {
	item             *store.Item
	payload          runtimeSteerPayload
	preparedAfterSeq int64
	acceptErr        error
}

type runtimeSteerState struct {
	mu       sync.Mutex
	store    store.Store
	notifier runtimeNotifier
	turn     *store.Turn
	queue    *core.SteerQueue
	byClient map[string]*runtimeSteerItemState
	byItem   map[string]*runtimeSteerItemState
}

func newActiveRuntimeTurn(
	cancel context.CancelFunc,
	st store.Store,
	notifier runtimeNotifier,
	turn *store.Turn,
) *activeRuntimeTurn {
	state := &runtimeSteerState{
		store:    st,
		notifier: notifier,
		turn:     turn,
		byClient: make(map[string]*runtimeSteerItemState),
		byItem:   make(map[string]*runtimeSteerItemState),
	}
	state.queue = core.NewSteerQueue(core.SteerQueueHooks{
		OnPrepared: state.prepare,
		OnConsumed: state.consume,
		OnRejected: state.reject,
	})
	return &activeRuntimeTurn{cancel: cancel, runtimeSteerState: state}
}

func (s *RuntimeService) Steer(
	ctx context.Context,
	req RuntimeSteerRequest,
) (*RuntimeSteerResult, error) {
	if s == nil {
		return nil, ErrRuntimeNotConfigured
	}
	req.ThreadID = strings.TrimSpace(req.ThreadID)
	req.TurnID = strings.TrimSpace(req.TurnID)
	req.ClientUserMessageID = strings.TrimSpace(req.ClientUserMessageID)
	req.Message = strings.TrimSpace(req.Message)
	if req.ThreadID == "" || req.TurnID == "" {
		return nil, store.ErrTurnNotFound
	}
	if req.Message == "" {
		return nil, core.ErrSteerMessageEmpty
	}
	if len(req.Message) > runtimeSteerMessageMaxSize {
		return nil, fmt.Errorf("%w: maximum is %d bytes", ErrRuntimeSteerMessageTooLarge, runtimeSteerMessageMaxSize)
	}
	if req.ClientUserMessageID == "" {
		req.ClientUserMessageID = uuid.NewString()
	}
	if len(req.ClientUserMessageID) > runtimeSteerIDMaxSize {
		return nil, fmt.Errorf("%w: maximum is %d bytes", ErrRuntimeSteerIDTooLarge, runtimeSteerIDMaxSize)
	}

	s.mu.Lock()
	active := s.active[req.TurnID]
	s.mu.Unlock()
	if active == nil || active.runtimeSteerState == nil {
		return nil, ErrRuntimeTurnNotActive
	}
	return active.enqueue(ctx, req)
}

func (s *activeRuntimeTurn) enqueue(
	ctx context.Context,
	req RuntimeSteerRequest,
) (*RuntimeSteerResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.turn == nil || s.turn.ID != req.TurnID || s.turn.ThreadID != req.ThreadID {
		return nil, store.ErrTurnNotFound
	}
	if existing := s.byClient[req.ClientUserMessageID]; existing != nil {
		if existing.payload.Message != req.Message {
			return nil, ErrRuntimeSteerIdempotencyConflict
		}
		if existing.acceptErr != nil {
			return nil, existing.acceptErr
		}
		item := existing.item
		if refreshed, err := s.store.GetItem(ctx, item.ID); err == nil {
			item = refreshed
		}
		return &RuntimeSteerResult{
			TurnID:              req.TurnID,
			ClientUserMessageID: req.ClientUserMessageID,
			Item:                item,
			Reused:              true,
		}, nil
	}

	queuedAt := time.Now().UTC()
	payload := runtimeSteerPayload{
		ClientUserMessageID: req.ClientUserMessageID,
		Message:             req.Message,
		Status:              runtimeSteerStatusQueued,
		QueuedAt:            queuedAt,
	}
	item, err := s.store.AppendItem(ctx, store.AppendItemRequest{
		ThreadID: req.ThreadID,
		TurnID:   req.TurnID,
		Kind:     runtimeSteerItemKind,
		Status:   runtimeSteerStatusQueued,
		Payload:  mustRuntimeJSON(payload),
	})
	if err != nil {
		return nil, err
	}
	state := &runtimeSteerItemState{item: item, payload: payload}
	s.byClient[req.ClientUserMessageID] = state
	s.byItem[item.ID] = state
	publishItemStarted(s.notifier, s.turn, item)
	if err := s.queue.Enqueue(core.SteerMessage{
		ID:       item.ID,
		Text:     req.Message,
		QueuedAt: queuedAt,
	}); err != nil {
		state.acceptErr = ErrRuntimeTurnNotActive
		if errors.Is(err, core.ErrSteerQueueFull) {
			state.acceptErr = err
		}
		s.failLocked(state, err)
		return nil, state.acceptErr
	}
	return &RuntimeSteerResult{
		TurnID:              req.TurnID,
		ClientUserMessageID: req.ClientUserMessageID,
		Item:                item,
	}, nil
}

func (s *runtimeSteerState) prepare(messages []core.SteerMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items, err := s.store.ListItems(context.Background(), store.ItemFilter{
		ThreadID: s.turn.ThreadID,
		TurnID:   s.turn.ID,
	})
	if err != nil {
		return fmt.Errorf("list steer consumption boundary: %w", err)
	}
	var consumedAfterSeq int64
	for _, item := range items {
		if item != nil && item.Seq > consumedAfterSeq {
			consumedAfterSeq = item.Seq
		}
	}
	if consumedAfterSeq == 0 {
		return errors.New("steer consumption boundary is unavailable")
	}
	for _, message := range messages {
		state := s.byItem[message.ID]
		if state == nil || state.item == nil {
			return fmt.Errorf("steer item %q is unavailable", message.ID)
		}
		if consumedAfterSeq < state.item.Seq {
			return fmt.Errorf("steer item %q is beyond consumption boundary", message.ID)
		}
		state.preparedAfterSeq = consumedAfterSeq
	}
	return nil
}

func (s *runtimeSteerState) consume(message core.SteerMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.byItem[message.ID]
	if state == nil {
		return fmt.Errorf("steer item %q is unavailable", message.ID)
	}
	if state.preparedAfterSeq == 0 {
		return fmt.Errorf("steer item %q has no prepared consumption boundary", message.ID)
	}
	now := time.Now().UTC()
	state.payload.Status = runtimeSteerStatusComplete
	state.payload.ConsumedAfterSeq = state.preparedAfterSeq
	state.payload.ConsumedAt = &now
	state.payload.FailedAt = nil
	state.payload.Error = ""
	item, err := s.store.UpdateItem(context.Background(), store.UpdateItemRequest{
		ID:      state.item.ID,
		Status:  runtimeSteerStatusComplete,
		Payload: mustRuntimeJSON(state.payload),
	})
	if err != nil {
		return err
	}
	state.item = item
	publishItemCompleted(s.notifier, s.turn, item)
	return nil
}

func (s *runtimeSteerState) reject(message core.SteerMessage, reason error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.byItem[message.ID]; state != nil {
		s.failLocked(state, reason)
	}
}

func (s *runtimeSteerState) failLocked(state *runtimeSteerItemState, reason error) {
	if state == nil || state.item == nil {
		return
	}
	now := time.Now().UTC()
	state.payload.Status = runtimeSteerStatusFailed
	state.preparedAfterSeq = 0
	state.payload.ConsumedAfterSeq = 0
	state.payload.ConsumedAt = nil
	state.payload.FailedAt = &now
	if reason != nil {
		state.payload.Error = reason.Error()
	}
	item, err := s.store.UpdateItem(context.Background(), store.UpdateItemRequest{
		ID:      state.item.ID,
		Status:  runtimeSteerStatusFailed,
		Payload: mustRuntimeJSON(state.payload),
	})
	if err != nil {
		return
	}
	state.item = item
	publishItemCompleted(s.notifier, s.turn, item)
}

func runtimeSteerText(input []protocol.UserInput) (string, error) {
	if len(input) == 0 {
		return "", errors.New("turn steer input must contain at least one text item")
	}
	parts := make([]string, 0, len(input))
	for _, item := range input {
		if item.Type != "text" {
			return "", fmt.Errorf("turn steer input type %q is not supported by the live runtime", item.Type)
		}
		text := strings.TrimSpace(item.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}
	message := strings.TrimSpace(strings.Join(parts, "\n\n"))
	if message == "" {
		return "", core.ErrSteerMessageEmpty
	}
	return message, nil
}
