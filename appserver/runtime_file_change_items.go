package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/appserver/store"
	"github.com/fugue-labs/gollem/core"
)

const (
	runtimeFileChangeItemKind         = "fileChange"
	runtimeFileChangeStatusInProgress = "inProgress"
	runtimeFileChangeStatusCompleted  = "completed"

	runtimePatchChangeAdd    = "add"
	runtimePatchChangeDelete = "delete"
	runtimePatchChangeUpdate = "update"
)

type runtimeFileChangePayload struct {
	Type     string                              `json:"type"`
	ID       string                              `json:"id,omitempty"`
	Changes  []runtimeFileUpdateChange           `json:"changes"`
	Status   string                              `json:"status"`
	Evidence []runtimeFileChangeArtifactEvidence `json:"evidence,omitempty"`
}

type runtimeFileUpdateChange struct {
	Path string                 `json:"path"`
	Kind runtimePatchChangeKind `json:"kind"`
	Diff string                 `json:"diff"`
}

type runtimePatchChangeKind struct {
	Type     string
	MovePath *string
}

func (k runtimePatchChangeKind) MarshalJSON() ([]byte, error) {
	payload := map[string]any{"type": k.Type}
	if k.Type == runtimePatchChangeUpdate {
		payload["movePath"] = k.MovePath
	}
	return json.Marshal(payload)
}

func (k *runtimePatchChangeKind) UnmarshalJSON(data []byte) error {
	var payload struct {
		Type     string  `json:"type"`
		MovePath *string `json:"movePath"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	k.Type = payload.Type
	k.MovePath = payload.MovePath
	return nil
}

type runtimeFileChangeArtifactEvidence struct {
	Path                 string `json:"path"`
	Operation            string `json:"operation"`
	Bytes                int64  `json:"bytes"`
	BeforeSHA256         string `json:"beforeSha256,omitempty"`
	AfterSHA256          string `json:"afterSha256,omitempty"`
	DiffTruncated        bool   `json:"diffTruncated,omitempty"`
	DiffOmittedReason    string `json:"diffOmittedReason,omitempty"`
	ContentEncoding      string `json:"contentEncoding,omitempty"`
	ContentTruncated     bool   `json:"contentTruncated,omitempty"`
	ContentOmittedReason string `json:"contentOmittedReason,omitempty"`
}

type runtimeFileChangeItemStartedNotificationParams struct {
	Item        runtimeFileChangePayload `json:"item"`
	ThreadID    string                   `json:"threadId"`
	TurnID      string                   `json:"turnId"`
	StartedAtMS int64                    `json:"startedAtMs"`
}

type runtimeFileChangeItemCompletedNotificationParams struct {
	Item          runtimeFileChangePayload `json:"item"`
	ThreadID      string                   `json:"threadId"`
	TurnID        string                   `json:"turnId"`
	CompletedAtMS int64                    `json:"completedAtMs"`
}

type runtimeFileChangePatchUpdatedNotificationParams struct {
	ThreadID string                    `json:"threadId"`
	TurnID   string                    `json:"turnId"`
	ItemID   string                    `json:"itemId"`
	Changes  []runtimeFileUpdateChange `json:"changes"`
}

type runtimeFileChangeTracker struct {
	mu        sync.Mutex
	store     store.Store
	notifier  runtimeNotifier
	turn      *store.Turn
	toolItems *runtimeToolItemTracker
	turnDiffs []string
	err       error
}

func newRuntimeFileChangeTracker(st store.Store, notifier runtimeNotifier, turn *store.Turn, toolItems *runtimeToolItemTracker) *runtimeFileChangeTracker {
	return &runtimeFileChangeTracker{
		store:     st,
		notifier:  notifier,
		turn:      turn,
		toolItems: toolItems,
	}
}

func (t *runtimeFileChangeTracker) artifactChanged(event core.ArtifactChangedEvent) {
	if t == nil || t.store == nil || t.turn == nil || strings.TrimSpace(event.Path) == "" {
		return
	}
	changedAt := event.ChangedAt
	if changedAt.IsZero() {
		changedAt = time.Now().UTC()
	}
	change := runtimeFileUpdateChange{
		Path: event.Path,
		Kind: runtimePatchChangeKind{Type: runtimeFileChangeKind(event.Operation)},
		Diff: event.Diff,
	}
	payload := runtimeFileChangePayload{
		Type:    runtimeFileChangeItemKind,
		Changes: []runtimeFileUpdateChange{change},
		Status:  runtimeFileChangeStatusInProgress,
		Evidence: []runtimeFileChangeArtifactEvidence{{
			Path:                 event.Path,
			Operation:            event.Operation,
			Bytes:                event.Bytes,
			BeforeSHA256:         event.BeforeSHA256,
			AfterSHA256:          event.AfterSHA256,
			DiffTruncated:        event.DiffTruncated,
			DiffOmittedReason:    event.DiffOmittedReason,
			ContentEncoding:      event.ContentEncoding,
			ContentTruncated:     event.ContentTruncated,
			ContentOmittedReason: event.ContentOmittedReason,
		}},
	}
	parentItemID := ""
	if t.toolItems != nil {
		parentItemID = t.toolItems.itemID(event.RunID, event.ToolCallID, event.ToolName)
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	item, err := t.store.AppendItem(context.Background(), store.AppendItemRequest{
		ThreadID:     t.turn.ThreadID,
		TurnID:       t.turn.ID,
		ParentItemID: parentItemID,
		Kind:         runtimeFileChangeItemKind,
		Status:       runtimeFileChangeStatusInProgress,
		Payload:      mustRuntimeJSON(payload),
	})
	if err != nil {
		t.recordErrorLocked("append", err)
		return
	}
	payload.ID = item.ID
	item, err = t.store.UpdateItem(context.Background(), store.UpdateItemRequest{
		ID:      item.ID,
		Status:  runtimeFileChangeStatusInProgress,
		Payload: mustRuntimeJSON(payload),
	})
	if err != nil {
		t.recordErrorLocked("set id", err)
		return
	}
	if t.notifier != nil {
		t.notifier.PublishNotification("item/started", runtimeFileChangeItemStartedNotificationParams{
			Item:        payload,
			ThreadID:    t.turn.ThreadID,
			TurnID:      t.turn.ID,
			StartedAtMS: changedAt.UnixMilli(),
		})
	}

	payload.Status = runtimeFileChangeStatusCompleted
	item, err = t.store.UpdateItem(context.Background(), store.UpdateItemRequest{
		ID:      item.ID,
		Status:  runtimeFileChangeStatusCompleted,
		Payload: mustRuntimeJSON(payload),
	})
	if err != nil {
		t.recordErrorLocked("complete", err)
		return
	}
	if event.Diff != "" {
		t.turnDiffs = append(t.turnDiffs, event.Diff)
	}
	if t.notifier == nil {
		return
	}
	t.notifier.PublishNotification("item/fileChange/patchUpdated", runtimeFileChangePatchUpdatedNotificationParams{
		ThreadID: t.turn.ThreadID,
		TurnID:   t.turn.ID,
		ItemID:   item.ID,
		Changes:  append([]runtimeFileUpdateChange(nil), payload.Changes...),
	})
	if len(t.turnDiffs) > 0 {
		t.notifier.PublishNotification("turn/diff/updated", turnDiffUpdatedNotificationParams{
			ThreadID: t.turn.ThreadID,
			TurnID:   t.turn.ID,
			Diff:     strings.Join(t.turnDiffs, "\n"),
		})
	}
	t.notifier.PublishNotification("item/completed", runtimeFileChangeItemCompletedNotificationParams{
		Item:          payload,
		ThreadID:      t.turn.ThreadID,
		TurnID:        t.turn.ID,
		CompletedAtMS: changedAt.UnixMilli(),
	})
}

func (t *runtimeFileChangeTracker) Err() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

func (t *runtimeFileChangeTracker) recordErrorLocked(operation string, err error) {
	if err == nil || t.err != nil {
		return
	}
	t.err = fmt.Errorf("persist runtime file change item (%s): %w", operation, err)
	publishRuntimeError(t.notifier, t.turn, t.err.Error())
}

func runtimeFileChangeKind(operation string) string {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "create", "add", "createdirectory":
		return runtimePatchChangeAdd
	case "delete", "remove":
		return runtimePatchChangeDelete
	default:
		return runtimePatchChangeUpdate
	}
}
