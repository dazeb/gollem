package appserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
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

type runtimeFileChangePayload = protocol.FileChangeItem
type runtimeFileUpdateChange = protocol.FileUpdateChange
type runtimePatchChangeKind = protocol.PatchChangeKind
type runtimeFileChangeArtifactEvidence = protocol.FileChangeArtifactEvidence
type runtimeFileChangeItemStartedNotificationParams = protocol.FileChangeItemStartedNotificationParams
type runtimeFileChangeItemCompletedNotificationParams = protocol.FileChangeItemCompletedNotificationParams
type runtimeFileChangePatchUpdatedNotificationParams = protocol.FileChangePatchUpdatedNotification

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
	recovery, recoveryUnavailable := runtimeFileChangeRecovery(event, t.turn)
	payload.Evidence[0].RevertUnavailableReason = recoveryUnavailable
	recoveryStore, recoveryStoreAvailable := t.store.(store.FileChangeRecoveryStore)
	if recoveryUnavailable == "" && !recoveryStoreAvailable {
		payload.Evidence[0].RevertUnavailableReason = "durable recovery storage is unavailable"
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
	if recoveryUnavailable == "" && recoveryStoreAvailable {
		recovery.ItemID = item.ID
		if _, err := recoveryStore.SaveFileChangeRecovery(context.Background(), store.SaveFileChangeRecoveryRequest{
			Recovery: recovery,
		}); err != nil {
			t.recordErrorLocked("save recovery snapshot", err)
			return
		}
		payload.Evidence[0].RevertSnapshotAvailable = true
		payload.Evidence[0].RevertUnavailableReason = ""
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
		Changes:  append([]runtimeFileUpdateChange{}, payload.Changes...),
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

func runtimeFileChangeRecovery(event core.ArtifactChangedEvent, turn *store.Turn) (store.FileChangeRecovery, string) {
	recovery := store.FileChangeRecovery{
		ThreadID:      turn.ThreadID,
		TurnID:        turn.ID,
		Path:          filepath.ToSlash(filepath.Clean(event.Path)),
		BeforeExists:  event.BeforeExists,
		AfterExists:   event.AfterExists,
		BeforeSHA256:  event.BeforeSHA256,
		AfterSHA256:   event.AfterSHA256,
		BeforeMode:    event.BeforeMode,
		AfterMode:     event.AfterMode,
		BeforeContent: append([]byte(nil), event.BeforeContentBytes...),
		Status:        store.FileChangeRecoveryAvailable,
		CreatedAt:     event.ChangedAt.UTC(),
	}
	if recovery.CreatedAt.IsZero() {
		recovery.CreatedAt = time.Now().UTC()
	}
	if recovery.Path == "." || recovery.Path == "" || filepath.IsAbs(filepath.FromSlash(recovery.Path)) ||
		recovery.Path == ".." || strings.HasPrefix(recovery.Path, "../") {
		return store.FileChangeRecovery{}, "workspace-relative path is unavailable"
	}
	if event.BeforeIsDir || event.AfterIsDir {
		return store.FileChangeRecovery{}, "directory changes cannot be reverted"
	}
	if event.BeforeIsSymlink || event.AfterIsSymlink {
		return store.FileChangeRecovery{}, "symlink changes cannot be reverted"
	}
	if !event.BeforeExists && !event.AfterExists {
		return store.FileChangeRecovery{}, "file existence evidence is incomplete"
	}
	switch runtimeFileChangeKind(event.Operation) {
	case runtimePatchChangeAdd:
		if event.BeforeExists || !event.AfterExists {
			return store.FileChangeRecovery{}, "create evidence does not match file existence"
		}
	case runtimePatchChangeDelete:
		if !event.BeforeExists || event.AfterExists {
			return store.FileChangeRecovery{}, "delete evidence does not match file existence"
		}
	case runtimePatchChangeUpdate:
		if !event.BeforeExists || !event.AfterExists {
			return store.FileChangeRecovery{}, "update evidence does not match file existence"
		}
	}
	if event.BeforeSize > runtimeFileChangeRecoveryMaxBytes ||
		event.AfterSize > runtimeFileChangeRecoveryMaxBytes ||
		len(event.BeforeContentBytes) > runtimeFileChangeRecoveryMaxBytes ||
		len(event.AfterContentBytes) > runtimeFileChangeRecoveryMaxBytes {
		return store.FileChangeRecovery{}, fmt.Sprintf("file exceeds %d byte revert limit", runtimeFileChangeRecoveryMaxBytes)
	}
	if event.BeforeExists && int64(len(event.BeforeContentBytes)) != event.BeforeSize {
		return store.FileChangeRecovery{}, "before-content size is inconsistent"
	}
	if event.AfterExists && int64(len(event.AfterContentBytes)) != event.AfterSize {
		return store.FileChangeRecovery{}, "after-content size is inconsistent"
	}
	if event.BeforeExists && runtimeSHA256(event.BeforeContentBytes) != event.BeforeSHA256 {
		return store.FileChangeRecovery{}, "before-content digest is inconsistent"
	}
	if event.AfterExists && runtimeSHA256(event.AfterContentBytes) != event.AfterSHA256 {
		return store.FileChangeRecovery{}, "after-content digest is inconsistent"
	}
	return recovery, ""
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
