package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

const threadRollbackDeprecationSummary = "thread/rollback is deprecated and will be removed soon"

func (s *Server) handleThreadRollback(ctx context.Context, raw json.RawMessage) (any, *protocol.Error) {
	st, rpcErr := s.requireStore("thread/rollback")
	if rpcErr != nil {
		return nil, rpcErr
	}
	var params protocol.ThreadHistoryRollbackParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return nil, rpcErr
	}
	threadID := params.EffectiveThreadID()
	if threadID == "" {
		return nil, invalidParams("threadId is required", nil)
	}
	if params.NumTurns < 1 {
		return nil, invalidParams("numTurns must be >= 1", nil)
	}
	releaseMutation, err := s.acquireWorkspaceMutationLease()
	if err != nil {
		if errors.Is(err, ErrWorkspaceRevertInProgress) {
			return nil, rpcError(
				protocol.CodeInvalidRequest,
				"cannot roll back thread history while a workspace file-change revert is in progress",
				nil,
			)
		}
		return nil, mapError("thread/rollback", err)
	}
	defer releaseMutation()
	activeTurns, err := st.ListTurns(ctx, store.TurnFilter{
		ThreadID: threadID,
		Statuses: []store.TurnStatus{store.TurnQueued, store.TurnRunning},
	})
	if err != nil {
		return nil, mapError("thread/rollback", err)
	}
	if len(activeTurns) > 0 {
		return nil, rpcError(
			protocol.CodeInvalidRequest,
			"cannot roll back thread history while a turn is active",
			nil,
		)
	}
	s.publishThreadRollbackDeprecationNotice()
	result, err := st.RollbackThread(ctx, store.RollbackThreadRequest{
		ID:       threadID,
		NumTurns: params.NumTurns,
	})
	if err != nil {
		return nil, mapError("thread/rollback", err)
	}
	marker := protocolTimelineItem(result.Marker)
	if result.Thread == nil || marker == nil {
		return nil, rpcError(protocol.CodeInternalError, "rollback result is incomplete", nil)
	}
	s.markThreadLoaded(result.Thread)
	return protocol.ThreadHistoryRollbackResult{
		Thread:                   threadHistoryRollbackRecord(result.Thread, result.Turns),
		RemovedTurnIDs:           rollbackTurnIDs(result.RemovedTurns),
		Marker:                   *marker,
		WorkspaceEffectsReverted: false,
	}, nil
}

func (s *Server) publishThreadRollbackDeprecationNotice() {
	if s == nil {
		return
	}
	s.mu.Lock()
	clientName := s.clientInfo.Name
	s.mu.Unlock()
	if strings.EqualFold(clientName, "codex-tui") {
		return
	}
	s.PublishNotification("deprecationNotice", deprecationNoticeNotificationParams{
		Summary: threadRollbackDeprecationSummary,
		Details: nil,
	})
}

func threadHistoryRollbackRecord(
	thread *store.Thread,
	turns []*store.Turn,
) protocol.ThreadHistoryRollbackRecord {
	var name *string
	if thread != nil && thread.Title != "" {
		title := thread.Title
		name = &title
	}
	record := protocolThreadRecord(thread)
	return protocol.ThreadHistoryRollbackRecord{
		ID:                 record.ID,
		Title:              record.Title,
		Workspace:          record.Workspace,
		Status:             record.Status,
		ForkedFromThreadID: record.ForkedFromThreadID,
		Settings:           record.Settings,
		Metadata:           record.Metadata,
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
		ArchivedAt:         record.ArchivedAt,
		DeletedAt:          record.DeletedAt,
		Name:               name,
		Turns:              protocolTurnRecords(turns),
	}
}

func rollbackTurnIDs(turns []*store.Turn) []string {
	ids := make([]string, 0, len(turns))
	for _, turn := range turns {
		if turn != nil {
			ids = append(ids, turn.ID)
		}
	}
	return ids
}
