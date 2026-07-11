package appserver

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

func listThreadRecords(ctx context.Context, st store.Store, params protocol.ThreadListParams) (protocol.ThreadListResponse, *protocol.Error) {
	if rpcErr := validateThreadListParams(params); rpcErr != nil {
		return protocol.ThreadListResponse{}, rpcErr
	}
	cursor := ""
	if params.Cursor != nil {
		cursor = *params.Cursor
	}
	offset, rpcErr := parseCursor(cursor, "thread/list")
	if rpcErr != nil {
		return protocol.ThreadListResponse{}, rpcErr
	}
	statuses, includeDeleted := threadListStatuses(params)
	threads, err := st.ListThreads(ctx, store.ThreadFilter{
		Statuses:       statuses,
		IncludeDeleted: includeDeleted,
	})
	if err != nil {
		return protocol.ThreadListResponse{}, mapError("thread/list", err)
	}
	threads = filterThreadListRecords(threads, params)
	sortKey := ""
	if params.SortKey != nil {
		sortKey = string(*params.SortKey)
	}
	sortDirection := ""
	if params.SortDirection != nil {
		sortDirection = string(*params.SortDirection)
	}
	sortThreadsForSearch(threads, sortKey, sortDirection)

	limit := defaultThreadSearchLimit
	if params.Limit != nil {
		limit = searchLimit(int(*params.Limit))
	}
	page, nextCursor := paginateThreadRecords(threads, offset, limit)
	backwardsCursor := (*string)(nil)
	if len(page) > 0 {
		value := strconv.Itoa(offset)
		backwardsCursor = &value
	}
	data := protocolThreadRecords(page)
	return protocol.ThreadListResponse{
		Data:            data,
		NextCursor:      nextCursor,
		BackwardsCursor: backwardsCursor,
		Threads:         append(make([]protocol.ThreadRecord, 0, len(data)), data...),
	}, nil
}

func validateThreadListParams(params protocol.ThreadListParams) *protocol.Error {
	if params.SortKey != nil {
		switch *params.SortKey {
		case protocol.ThreadSortCreatedAt, protocol.ThreadSortUpdatedAt, protocol.ThreadSortRecencyAt:
		default:
			return invalidParams("thread/list sortKey is invalid", nil)
		}
	}
	if params.SortDirection != nil {
		switch *params.SortDirection {
		case protocol.SortDirectionAsc, protocol.SortDirectionDesc:
		default:
			return invalidParams("thread/list sortDirection is invalid", nil)
		}
	}
	for _, status := range params.Statuses {
		switch status {
		case protocol.ThreadLifecycleActive, protocol.ThreadLifecycleArchived, protocol.ThreadLifecycleDeleted:
		default:
			return invalidParams("thread/list status is invalid", nil)
		}
	}
	for _, source := range params.SourceKinds {
		if !validThreadSourceKind(source) {
			return invalidParams("thread/list sourceKind is invalid", nil)
		}
	}
	return nil
}

func validThreadSourceKind(source protocol.ThreadSourceKind) bool {
	switch source {
	case protocol.ThreadSourceCLI, protocol.ThreadSourceVSCode, protocol.ThreadSourceExec,
		protocol.ThreadSourceAppServer, protocol.ThreadSourceSubAgent,
		protocol.ThreadSourceSubAgentReview, protocol.ThreadSourceSubAgentCompact,
		protocol.ThreadSourceSubAgentSpawn, protocol.ThreadSourceSubAgentOther,
		protocol.ThreadSourceUnknown:
		return true
	default:
		return false
	}
}

func threadListStatuses(params protocol.ThreadListParams) ([]store.ThreadStatus, bool) {
	if len(params.Statuses) > 0 {
		statuses := make([]store.ThreadStatus, 0, len(params.Statuses))
		includeDeleted := params.IncludeDeleted
		for _, status := range params.Statuses {
			converted := store.ThreadStatus(status)
			statuses = append(statuses, converted)
			includeDeleted = includeDeleted || converted == store.ThreadDeleted
		}
		return statuses, includeDeleted
	}
	if params.IncludeDeleted && params.Archived == nil {
		return nil, true
	}
	status := store.ThreadActive
	if params.Archived != nil && *params.Archived {
		status = store.ThreadArchived
	}
	statuses := []store.ThreadStatus{status}
	if params.IncludeDeleted {
		statuses = append(statuses, store.ThreadDeleted)
	}
	return statuses, params.IncludeDeleted
}

func filterThreadListRecords(threads []*store.Thread, params protocol.ThreadListParams) []*store.Thread {
	var providers map[string]struct{}
	if len(params.ModelProviders) > 0 {
		providers = stringSet(params.ModelProviders)
	}
	var sources map[protocol.ThreadSourceKind]struct{}
	if len(params.SourceKinds) > 0 {
		sources = sourceKindSet(params.SourceKinds)
	}
	searchTerm := ""
	if params.SearchTerm != nil {
		searchTerm = strings.ToLower(strings.TrimSpace(*params.SearchTerm))
	}
	var workspaces map[string]struct{}
	if params.CWD != nil {
		workspaces = stringSet(params.CWD.Paths())
	}
	out := make([]*store.Thread, 0, len(threads))
	for _, thread := range threads {
		if thread == nil {
			continue
		}
		if workspaces != nil {
			if _, ok := workspaces[thread.Workspace]; !ok {
				continue
			}
		}
		if providers != nil {
			provider := firstNonEmpty(stringMapValue(thread.Settings, "providerId"), stringMapValue(thread.Settings, "provider"))
			if _, ok := providers[provider]; !ok {
				continue
			}
		}
		if sources != nil {
			if !threadSourceKindMatches(threadSourceKind(thread), sources) {
				continue
			}
		}
		if searchTerm != "" && !strings.Contains(strings.ToLower(thread.Title), searchTerm) {
			continue
		}
		out = append(out, thread)
	}
	return out
}

func threadSourceKind(thread *store.Thread) protocol.ThreadSourceKind {
	source := protocol.ThreadSourceKind(stringMapValue(thread.Metadata, "sourceKind"))
	if source == "" {
		return protocol.ThreadSourceAppServer
	}
	if !validThreadSourceKind(source) {
		return protocol.ThreadSourceUnknown
	}
	return source
}

func threadSourceKindMatches(source protocol.ThreadSourceKind, filters map[protocol.ThreadSourceKind]struct{}) bool {
	if _, ok := filters[source]; ok {
		return true
	}
	if _, genericSubAgent := filters[protocol.ThreadSourceSubAgent]; !genericSubAgent {
		return false
	}
	switch source {
	case protocol.ThreadSourceSubAgentReview, protocol.ThreadSourceSubAgentCompact,
		protocol.ThreadSourceSubAgentSpawn, protocol.ThreadSourceSubAgentOther:
		return true
	default:
		return false
	}
}

func stringSet(values []string) map[string]struct{} {
	if values == nil {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func sourceKindSet(values []protocol.ThreadSourceKind) map[protocol.ThreadSourceKind]struct{} {
	if values == nil {
		return nil
	}
	out := make(map[protocol.ThreadSourceKind]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func paginateThreadRecords(values []*store.Thread, offset, limit int) ([]*store.Thread, *string) {
	if offset >= len(values) {
		return []*store.Thread{}, nil
	}
	end := min(len(values), offset+limit)
	page := append([]*store.Thread(nil), values[offset:end]...)
	if end >= len(values) {
		return page, nil
	}
	cursor := strconv.Itoa(end)
	return page, &cursor
}

func protocolThreadRecords(threads []*store.Thread) []protocol.ThreadRecord {
	out := make([]protocol.ThreadRecord, 0, len(threads))
	for _, thread := range threads {
		if thread != nil {
			out = append(out, protocolThreadRecord(thread))
		}
	}
	return out
}

func protocolThreadRecord(thread *store.Thread) protocol.ThreadRecord {
	if thread == nil {
		return protocol.ThreadRecord{}
	}
	return protocol.ThreadRecord{
		ID:                 thread.ID,
		Title:              thread.Title,
		Workspace:          thread.Workspace,
		Status:             protocol.ThreadLifecycleStatus(thread.Status),
		ForkedFromThreadID: thread.ForkedFromThreadID,
		Settings:           cloneSettings(thread.Settings),
		Metadata:           cloneSettings(thread.Metadata),
		CreatedAt:          thread.CreatedAt,
		UpdatedAt:          thread.UpdatedAt,
		ArchivedAt:         thread.ArchivedAt,
		DeletedAt:          thread.DeletedAt,
	}
}

func protocolTurnRecords(turns []*store.Turn) []protocol.TurnRecord {
	out := make([]protocol.TurnRecord, 0, len(turns))
	for _, turn := range turns {
		if turn == nil {
			continue
		}
		out = append(out, protocol.TurnRecord{
			ID:          turn.ID,
			ThreadID:    turn.ThreadID,
			Status:      protocol.TurnLifecycleStatus(turn.Status),
			Input:       append(json.RawMessage(nil), turn.Input...),
			Result:      append(json.RawMessage(nil), turn.Result...),
			Error:       turn.Error,
			Usage:       cloneSettings(turn.Usage),
			Metadata:    cloneSettings(turn.Metadata),
			CreatedAt:   turn.CreatedAt,
			UpdatedAt:   turn.UpdatedAt,
			StartedAt:   turn.StartedAt,
			CompletedAt: turn.CompletedAt,
		})
	}
	return out
}

func protocolTimelineItems(items []*store.Item) []protocol.TimelineItem {
	out := make([]protocol.TimelineItem, 0, len(items))
	for _, item := range items {
		if converted := protocolTimelineItem(item); converted != nil {
			out = append(out, *converted)
		}
	}
	return out
}

func nestThreadTurns(turns []protocol.TurnRecord, items []protocol.TimelineItem) []protocol.TurnRecord {
	if turns == nil {
		return nil
	}
	nested := append([]protocol.TurnRecord(nil), turns...)
	turnIndex := make(map[string]int, len(nested))
	for i := range nested {
		turnIndex[nested[i].ID] = i
	}
	for _, item := range items {
		if i, ok := turnIndex[item.TurnID]; ok {
			nested[i].Items = append(nested[i].Items, item)
		}
	}
	return nested
}
