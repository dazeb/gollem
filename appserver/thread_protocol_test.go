package appserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

type legacyThreadReadResult struct {
	Thread *store.Thread `json:"thread"`
	Turns  []*store.Turn `json:"turns,omitempty"`
	Items  []*store.Item `json:"items,omitempty"`
}

func TestThreadListUsesExportedDiscoveryContract(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	alpha, err := st.CreateThread(ctx, store.CreateThreadRequest{
		Title:     "Alpha thread",
		Workspace: "/workspace/a",
		Settings:  map[string]any{"provider": "openai"},
	})
	if err != nil {
		t.Fatalf("CreateThread alpha: %v", err)
	}
	beta, err := st.CreateThread(ctx, store.CreateThreadRequest{
		Title:     "Beta thread",
		Workspace: "/workspace/b",
		Settings:  map[string]any{"provider": "anthropic"},
	})
	if err != nil {
		t.Fatalf("CreateThread beta: %v", err)
	}
	archived, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Archived thread", Workspace: "/workspace/a"})
	if err != nil {
		t.Fatalf("CreateThread archived: %v", err)
	}
	if _, err := st.ArchiveThread(ctx, archived.ID); err != nil {
		t.Fatalf("ArchiveThread: %v", err)
	}

	server := readyServer(WithStore(st))
	defaultResponse := server.HandleRequest(ctx, request("thread/list", nil))
	if defaultResponse.Error != nil {
		t.Fatalf("thread/list default error: %v", defaultResponse.Error)
	}
	var defaultList protocol.ThreadListResult
	decodeResult(t, defaultResponse, &defaultList)
	if len(defaultList.Data) != 2 || len(defaultList.Threads) != 2 || containsThreadRecord(defaultList.Data, archived.ID) {
		t.Fatalf("thread/list default = %#v", defaultList)
	}
	emptyFiltersResponse := server.HandleRequest(ctx, request("thread/list", map[string]any{
		"modelProviders": []string{},
		"sourceKinds":    []string{},
	}))
	if emptyFiltersResponse.Error != nil {
		t.Fatalf("thread/list empty filters error: %v", emptyFiltersResponse.Error)
	}
	var emptyFilters protocol.ThreadListResult
	decodeResult(t, emptyFiltersResponse, &emptyFilters)
	if len(emptyFilters.Data) != 2 {
		t.Fatalf("thread/list empty filters = %#v", emptyFilters)
	}

	filteredResponse := server.HandleRequest(ctx, request("thread/list", map[string]any{
		"cwd":            "/workspace/a",
		"modelProviders": []string{"openai"},
		"sourceKinds":    []string{"appServer"},
		"searchTerm":     "ALPHA",
		"sortKey":        "updated_at",
		"sortDirection":  "asc",
		"useStateDbOnly": true,
	}))
	if filteredResponse.Error != nil {
		t.Fatalf("thread/list filtered error: %v", filteredResponse.Error)
	}
	var filtered protocol.ThreadListResult
	decodeResult(t, filteredResponse, &filtered)
	if len(filtered.Data) != 1 || filtered.Data[0].ID != alpha.ID || filtered.Data[0].Status != protocol.ThreadLifecycleActive {
		t.Fatalf("thread/list filtered = %#v", filtered)
	}

	archivedResponse := server.HandleRequest(ctx, request("thread/list", map[string]any{"archived": true}))
	if archivedResponse.Error != nil {
		t.Fatalf("thread/list archived error: %v", archivedResponse.Error)
	}
	var archivedList protocol.ThreadListResult
	decodeResult(t, archivedResponse, &archivedList)
	if len(archivedList.Data) != 1 || archivedList.Data[0].ID != archived.ID {
		t.Fatalf("thread/list archived = %#v", archivedList)
	}

	firstResponse := server.HandleRequest(ctx, request("thread/list", map[string]any{"limit": 1}))
	if firstResponse.Error != nil {
		t.Fatalf("thread/list first page error: %v", firstResponse.Error)
	}
	var first protocol.ThreadListResult
	decodeResult(t, firstResponse, &first)
	if len(first.Data) != 1 || first.NextCursor == nil || first.BackwardsCursor == nil {
		t.Fatalf("thread/list first page = %#v", first)
	}
	secondResponse := server.HandleRequest(ctx, request("thread/list", map[string]any{
		"cursor": *first.NextCursor,
		"limit":  1,
	}))
	if secondResponse.Error != nil {
		t.Fatalf("thread/list second page error: %v", secondResponse.Error)
	}
	var second protocol.ThreadListResult
	decodeResult(t, secondResponse, &second)
	if len(second.Data) != 1 || second.Data[0].ID == first.Data[0].ID || !containsThreadRecord(append(first.Data, second.Data...), beta.ID) {
		t.Fatalf("thread/list pages = first %#v second %#v", first, second)
	}
}

func TestThreadReadUsesExportedRecordContract(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Read me", Workspace: "/workspace"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	turn, err := st.CreateTurn(ctx, store.CreateTurnRequest{ThreadID: thread.ID, Input: json.RawMessage(`{"prompt":"hello"}`)})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	item, err := st.AppendItem(ctx, store.AppendItemRequest{
		ThreadID: thread.ID,
		TurnID:   turn.ID,
		Kind:     "message",
		Status:   "completed",
		Payload:  json.RawMessage(`{"role":"user","text":"hello"}`),
	})
	if err != nil {
		t.Fatalf("AppendItem: %v", err)
	}

	server := readyServer(WithStore(st))
	response := server.HandleRequest(ctx, request("thread/read", map[string]any{"threadId": thread.ID}))
	if response.Error != nil {
		t.Fatalf("thread/read error: %v", response.Error)
	}
	var read protocol.ThreadReadResult
	decodeResult(t, response, &read)
	if read.Thread.ID != thread.ID || len(read.Turns) != 1 || len(read.Items) != 1 || len(read.Thread.Turns) != 1 {
		t.Fatalf("thread/read = %#v", read)
	}
	if read.Thread.Turns[0].ID != turn.ID || len(read.Thread.Turns[0].Items) != 1 || read.Thread.Turns[0].Items[0].ID != item.ID {
		t.Fatalf("nested thread/read = %#v", read.Thread.Turns)
	}
}

func TestThreadListRejectsInvalidDiscoveryParams(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{name: "cursor", params: map[string]any{"cursor": "bad"}},
		{name: "sort key", params: map[string]any{"sortKey": "bad"}},
		{name: "sort direction", params: map[string]any{"sortDirection": "bad"}},
		{name: "status", params: map[string]any{"statuses": []string{"bad"}}},
		{name: "source kind", params: map[string]any{"sourceKinds": []string{"bad"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := readyServer(WithStore(newThreadProtocolStore(t)))
			response := server.HandleRequest(context.Background(), request("thread/list", test.params))
			if response.Error == nil || response.Error.Code != protocol.CodeInvalidParams {
				t.Fatalf("thread/list error = %#v", response.Error)
			}
		})
	}
}

func TestThreadListSourceKindsPreservePublicCategories(t *testing.T) {
	threads := []*store.Thread{
		{ID: "app-server"},
		{ID: "review", Metadata: map[string]any{"sourceKind": "subAgentReview"}},
		{ID: "unknown", Metadata: map[string]any{"sourceKind": "custom-source"}},
	}
	tests := []struct {
		name   string
		source protocol.ThreadSourceKind
		want   string
	}{
		{name: "default app server", source: protocol.ThreadSourceAppServer, want: "app-server"},
		{name: "generic subagent", source: protocol.ThreadSourceSubAgent, want: "review"},
		{name: "unknown provenance", source: protocol.ThreadSourceUnknown, want: "unknown"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			filtered := filterThreadListRecords(threads, protocol.ThreadListParams{SourceKinds: []protocol.ThreadSourceKind{test.source}})
			if len(filtered) != 1 || filtered[0].ID != test.want {
				t.Fatalf("filtered threads = %#v, want %s", filtered, test.want)
			}
		})
	}
}

func TestThreadListEmptyPageUsesArrays(t *testing.T) {
	server := readyServer(WithStore(newThreadProtocolStore(t)))
	response := server.HandleRequest(context.Background(), request("thread/list", nil))
	if response.Error != nil {
		t.Fatalf("thread/list error: %v", response.Error)
	}
	var raw map[string]json.RawMessage
	decodeResult(t, response, &raw)
	if string(raw["data"]) != "[]" || string(raw["threads"]) != "[]" {
		t.Fatalf("thread/list empty arrays = data:%s threads:%s", raw["data"], raw["threads"])
	}
}

func TestThreadReadCanOmitTurnsAndItems(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "No history"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))
	response := server.HandleRequest(ctx, request("thread/read", map[string]any{
		"threadId":     thread.ID,
		"includeTurns": false,
		"includeItems": false,
	}))
	if response.Error != nil {
		t.Fatalf("thread/read error: %v", response.Error)
	}
	var raw map[string]json.RawMessage
	decodeResult(t, response, &raw)
	if _, ok := raw["turns"]; ok {
		t.Fatalf("thread/read unexpectedly included turns: %s", raw["turns"])
	}
	if _, ok := raw["items"]; ok {
		t.Fatalf("thread/read unexpectedly included items: %s", raw["items"])
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw["thread"], &nested); err != nil {
		t.Fatalf("decode thread: %v", err)
	}
	if _, ok := nested["turns"]; ok {
		t.Fatalf("thread/read unexpectedly included nested turns: %s", nested["turns"])
	}
}

func containsThreadRecord(threads []protocol.ThreadRecord, id string) bool {
	for _, thread := range threads {
		if thread.ID == id {
			return true
		}
	}
	return false
}

func newThreadProtocolStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}
