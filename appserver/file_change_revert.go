package appserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
	toolfs "github.com/fugue-labs/gollem/appserver/tools/fs"
)

const (
	fileChangeRevertMethod            = "item/fileChange/revert"
	fileChangeRevertIdempotencyMaxLen = 256
)

func (s *Server) handleFileChangeRevert(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore(fileChangeRevertMethod)
	if rpcErr != nil {
		return nil, rpcErr
	}
	fsService, rpcErr := s.requireFS(fileChangeRevertMethod)
	if rpcErr != nil {
		return nil, rpcErr
	}
	recoveryStore, ok := st.(store.FileChangeRecoveryStore)
	if !ok {
		return nil, protocol.MethodUnavailableErrorWithReason(fileChangeRevertMethod, "configured store does not support durable file-change recovery")
	}
	var params protocol.FileChangeRevertParams
	if err := decodeStrictJSON(raw, &params); err != nil {
		return nil, invalidParams("invalid params", err)
	}
	params.ThreadID = strings.TrimSpace(params.ThreadID)
	params.ItemID = strings.TrimSpace(params.ItemID)
	params.IdempotencyKey = strings.TrimSpace(params.IdempotencyKey)
	if params.ThreadID == "" || params.ItemID == "" || params.IdempotencyKey == "" {
		return nil, invalidParams("threadId, itemId, and idempotencyKey are required", nil)
	}
	if len(params.IdempotencyKey) > fileChangeRevertIdempotencyMaxLen {
		return nil, invalidParams("idempotencyKey is too long", nil)
	}

	thread, err := st.GetThread(ctx, params.ThreadID)
	if err != nil {
		return nil, mapError(fileChangeRevertMethod, err)
	}
	state, rpcErr := loadFileChangeRevertState(ctx, st, recoveryStore, thread, params.ItemID)
	if rpcErr != nil {
		return nil, rpcErr
	}
	item, recovery := state.item, state.recovery

	if recovery.Status == store.FileChangeRecoveryReverted {
		prepared, err := recoveryStore.PrepareFileChangeRevert(ctx, store.PrepareFileChangeRevertRequest{
			ItemID:         item.ID,
			IdempotencyKey: params.IdempotencyKey,
		})
		if err != nil {
			return nil, mapError(fileChangeRevertMethod, err)
		}
		return fileChangeRevertProtocolResult(prepared.Recovery, prepared.Marker, true)
	}
	if thread.Status == store.ThreadDeleted {
		return nil, mapError(fileChangeRevertMethod, store.ErrThreadDeleted)
	}
	if err := requireExactThreadWorkspace(thread, fsService.Root()); err != nil {
		return nil, invalidParams("thread workspace does not match the configured filesystem root", err)
	}
	if recovery.Status == store.FileChangeRecoveryPending && recovery.IdempotencyKey != params.IdempotencyKey {
		return nil, mapError(fileChangeRevertMethod, store.ErrFileChangeRevertIdempotencyConflict)
	}
	releaseWorkspace, err := s.reserveWorkspaceRevert(ctx, st, fsService.Root())
	if err != nil {
		if errors.Is(err, ErrWorkspaceTurnActive) {
			return nil, rpcError(protocol.CodeInvalidRequest, "cannot revert a file change while a workspace turn is active", nil)
		}
		if errors.Is(err, ErrWorkspaceRevertInProgress) {
			return nil, rpcError(protocol.CodeInvalidRequest, "another workspace file-change revert is in progress", nil)
		}
		return nil, mapError(fileChangeRevertMethod, err)
	}
	defer releaseWorkspace()

	state, rpcErr = loadFileChangeRevertState(ctx, st, recoveryStore, thread, params.ItemID)
	if rpcErr != nil {
		return nil, rpcErr
	}
	item, recovery = state.item, state.recovery
	turn := state.turn
	if recovery.Status == store.FileChangeRecoveryReverted {
		prepared, err := recoveryStore.PrepareFileChangeRevert(ctx, store.PrepareFileChangeRevertRequest{
			ItemID:         item.ID,
			IdempotencyKey: params.IdempotencyKey,
		})
		if err != nil {
			return nil, mapError(fileChangeRevertMethod, err)
		}
		return fileChangeRevertProtocolResult(prepared.Recovery, prepared.Marker, true)
	}
	if recovery.Status == store.FileChangeRecoveryPending && recovery.IdempotencyKey != params.IdempotencyKey {
		return nil, mapError(fileChangeRevertMethod, store.ErrFileChangeRevertIdempotencyConflict)
	}

	revertRequest := fileChangeRevertFilesystemRequest(recovery, params.IdempotencyKey)
	if err := fsService.RecoverPendingRevert(ctx, revertRequest); err != nil {
		return nil, mapError(fileChangeRevertMethod, err)
	}
	current, err := captureRuntimeArtifact(ctx, fsService, recovery.Path)
	if err != nil {
		return nil, mapError(fileChangeRevertMethod, err)
	}
	currentIsAfter := runtimeArtifactMatchesRecoveryState(current, recovery.AfterExists, recovery.AfterSHA256, recovery.AfterMode)
	currentIsBefore := runtimeArtifactMatchesRecoveryState(current, recovery.BeforeExists, recovery.BeforeSHA256, recovery.BeforeMode)
	if !currentIsAfter && (recovery.Status != store.FileChangeRecoveryPending || !currentIsBefore) {
		return nil, mapError(fileChangeRevertMethod, toolfs.ErrExactStateMismatch)
	}

	prepared, err := recoveryStore.PrepareFileChangeRevert(ctx, store.PrepareFileChangeRevertRequest{
		ItemID:         item.ID,
		IdempotencyKey: params.IdempotencyKey,
		PreparedAt:     time.Now().UTC(),
	})
	if err != nil {
		return nil, mapError(fileChangeRevertMethod, err)
	}
	if prepared.Recovery.Status == store.FileChangeRecoveryReverted {
		return fileChangeRevertProtocolResult(prepared.Recovery, prepared.Marker, true)
	}
	reused := prepared.Reused
	if currentIsAfter {
		approvalCtx := withRuntimeTurnContext(ctx, thread.ID, turn.ID)
		approvalCtx = withRuntimeApprovalItemID(approvalCtx, item.ID)
		if _, err := fsService.RevertFile(approvalCtx, revertRequest); err != nil {
			abortFileChangeRevertIfUnchanged(ctx, recoveryStore, fsService, recovery, params.IdempotencyKey)
			return nil, mapError(fileChangeRevertMethod, err)
		}
		s.publishFileChanged(string(toolfs.OperationRevertFileChange), recovery.Path, "")
	}
	completed, err := recoveryStore.CompleteFileChangeRevert(ctx, store.CompleteFileChangeRevertRequest{
		ItemID:         item.ID,
		IdempotencyKey: params.IdempotencyKey,
		RevertedAt:     time.Now().UTC(),
	})
	if err != nil {
		return nil, mapError(fileChangeRevertMethod, err)
	}
	if !completed.Reused {
		publishItemCompleted(s, turn, completed.Marker)
	}
	s.markThreadLoaded(thread)
	return fileChangeRevertProtocolResult(completed.Recovery, completed.Marker, reused || completed.Reused)
}

type loadedFileChangeRevertState struct {
	item     *store.Item
	turn     *store.Turn
	recovery *store.FileChangeRecovery
}

func loadFileChangeRevertState(
	ctx context.Context,
	st store.Store,
	recoveryStore store.FileChangeRecoveryStore,
	thread *store.Thread,
	itemID string,
) (loadedFileChangeRevertState, *protocol.Error) {
	item, err := st.GetItem(ctx, itemID)
	if err != nil {
		return loadedFileChangeRevertState{}, mapError(fileChangeRevertMethod, err)
	}
	if thread == nil || item.ThreadID != thread.ID || item.Kind != runtimeFileChangeItemKind || item.Status != runtimeFileChangeStatusCompleted {
		return loadedFileChangeRevertState{}, invalidParams("item is not a completed file change in the requested thread", nil)
	}
	turn, err := st.GetTurn(ctx, item.TurnID)
	if err != nil {
		return loadedFileChangeRevertState{}, mapError(fileChangeRevertMethod, err)
	}
	if turn.ThreadID != thread.ID || !fileChangeRevertTerminalTurnStatus(turn.Status) {
		return loadedFileChangeRevertState{}, invalidParams("file-change turn is not terminal in the requested thread", nil)
	}
	recovery, err := recoveryStore.GetFileChangeRecovery(ctx, item.ID)
	if err != nil {
		return loadedFileChangeRevertState{}, mapError(fileChangeRevertMethod, err)
	}
	if err := validateFileChangeRevertEvidence(item, recovery); err != nil {
		return loadedFileChangeRevertState{}, invalidParams("file-change recovery evidence is inconsistent", err)
	}
	return loadedFileChangeRevertState{item: item, turn: turn, recovery: recovery}, nil
}

func fileChangeRevertFilesystemRequest(
	recovery *store.FileChangeRecovery,
	idempotencyKey string,
) toolfs.RevertFileRequest {
	return toolfs.RevertFileRequest{
		Path:          recovery.Path,
		TransactionID: idempotencyKey,
		Before: toolfs.ExactFileState{
			Exists:    recovery.BeforeExists,
			SHA256:    recovery.BeforeSHA256,
			Content:   append([]byte(nil), recovery.BeforeContent...),
			Mode:      iofs.FileMode(recovery.BeforeMode),
			CheckMode: true,
		},
		After: toolfs.ExactFileState{
			Exists:    recovery.AfterExists,
			SHA256:    recovery.AfterSHA256,
			Mode:      iofs.FileMode(recovery.AfterMode),
			CheckMode: true,
		},
	}
}

func abortFileChangeRevertIfUnchanged(
	ctx context.Context,
	recoveryStore store.FileChangeRecoveryStore,
	fsService *toolfs.Service,
	recovery *store.FileChangeRecovery,
	idempotencyKey string,
) {
	if recoveryStore == nil || fsService == nil || recovery == nil {
		return
	}
	current, err := captureRuntimeArtifact(ctx, fsService, recovery.Path)
	if err != nil || !runtimeArtifactMatchesRecoveryState(current, recovery.AfterExists, recovery.AfterSHA256, recovery.AfterMode) {
		return
	}
	_, _ = recoveryStore.AbortFileChangeRevert(ctx, store.AbortFileChangeRevertRequest{
		ItemID:         recovery.ItemID,
		IdempotencyKey: idempotencyKey,
		AbortedAt:      time.Now().UTC(),
	})
}

func fileChangeRevertTerminalTurnStatus(status store.TurnStatus) bool {
	switch status {
	case store.TurnCompleted, store.TurnFailed, store.TurnInterrupted:
		return true
	default:
		return false
	}
}

func validateFileChangeRevertEvidence(item *store.Item, recovery *store.FileChangeRecovery) error {
	var payload protocol.FileChangeItem
	if err := decodeStrictJSON(item.Payload, &payload); err != nil {
		return fmt.Errorf("decode file-change item: %w", err)
	}
	if recovery == nil || recovery.ItemID != item.ID || recovery.ThreadID != item.ThreadID || recovery.TurnID != item.TurnID {
		return errors.New("recovery identity does not match item")
	}
	if payload.ID != item.ID || payload.Type != runtimeFileChangeItemKind || payload.Status != protocol.PatchApplyStatusCompleted ||
		len(payload.Changes) != 1 || len(payload.Evidence) != 1 {
		return errors.New("file-change item shape is not exact")
	}
	change := payload.Changes[0]
	evidence := payload.Evidence[0]
	expectedKind := runtimePatchChangeUpdate
	switch {
	case !recovery.BeforeExists && recovery.AfterExists:
		expectedKind = runtimePatchChangeAdd
	case recovery.BeforeExists && !recovery.AfterExists:
		expectedKind = runtimePatchChangeDelete
	case !recovery.BeforeExists && !recovery.AfterExists:
		return errors.New("recovery existence evidence is empty")
	}
	switch strings.ToLower(strings.TrimSpace(evidence.Operation)) {
	case "create", "update", "delete":
	default:
		return errors.New("file-change operation is unsupported")
	}
	if change.Path != recovery.Path || evidence.Path != recovery.Path ||
		change.Kind.Type != expectedKind || runtimeFileChangeKind(evidence.Operation) != expectedKind ||
		evidence.BeforeSHA256 != recovery.BeforeSHA256 || evidence.AfterSHA256 != recovery.AfterSHA256 ||
		!evidence.RevertSnapshotAvailable || evidence.RevertUnavailableReason != "" {
		return errors.New("public evidence does not match private recovery")
	}
	return nil
}

func runtimeArtifactMatchesRecoveryState(current runtimeArtifactCapture, exists bool, sha256 string, mode uint32) bool {
	if current.IsDir || current.IsSymlink || current.Exists != exists || (current.Exists && !current.IsRegular) {
		return false
	}
	if !exists {
		return true
	}
	return sha256 != "" && current.SHA256 == sha256 && current.Mode == mode
}

func requireExactThreadWorkspace(thread *store.Thread, filesystemRoot string) error {
	if thread == nil || strings.TrimSpace(thread.Workspace) == "" {
		return errors.New("thread workspace is empty")
	}
	threadWorkspace, err := canonicalExistingPath(thread.Workspace)
	if err != nil {
		return fmt.Errorf("resolve thread workspace: %w", err)
	}
	root, err := canonicalExistingPath(filesystemRoot)
	if err != nil {
		return fmt.Errorf("resolve filesystem root: %w", err)
	}
	if threadWorkspace != root {
		return fmt.Errorf("thread workspace %q differs from filesystem root %q", threadWorkspace, root)
	}
	return nil
}

func canonicalExistingPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	evaluated, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	return filepath.Clean(evaluated), nil
}

func fileChangeRevertProtocolResult(
	recovery *store.FileChangeRecovery,
	marker *store.Item,
	reused bool,
) (protocol.FileChangeRevertResult, *protocol.Error) {
	converted := protocolTimelineItem(marker)
	if recovery == nil || converted == nil || recovery.Status != store.FileChangeRecoveryReverted {
		return protocol.FileChangeRevertResult{}, rpcError(protocol.CodeInternalError, "file-change revert receipt is incomplete", nil)
	}
	return protocol.FileChangeRevertResult{
		ThreadID:       recovery.ThreadID,
		TurnID:         recovery.TurnID,
		ItemID:         recovery.ItemID,
		IdempotencyKey: recovery.IdempotencyKey,
		Path:           recovery.Path,
		BeforeExists:   recovery.BeforeExists,
		AfterExists:    recovery.AfterExists,
		BeforeSHA256:   recovery.BeforeSHA256,
		AfterSHA256:    recovery.AfterSHA256,
		RevertedAt:     recovery.RevertedAt,
		Marker:         *converted,
		Reused:         reused,
	}, nil
}

func decodeStrictJSON(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("payload must contain one JSON value")
		}
		return err
	}
	return nil
}
