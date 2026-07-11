package appserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

func TestServerThreadGoalHandlersUseStructuredProtocol(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Structured goal"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))

	getResp := server.HandleRequest(ctx, request("thread/goal/get", map[string]any{"threadId": thread.ID}))
	if getResp.Error != nil {
		t.Fatalf("thread/goal/get error: %v", getResp.Error)
	}
	var initial protocol.ThreadGoalGetResponse
	decodeResult(t, getResp, &initial)
	if initial.Goal != nil || initial.Set == nil || *initial.Set {
		t.Fatalf("initial goal = %#v", initial)
	}

	setResp := server.HandleRequest(ctx, request("thread/goal/set", map[string]any{
		"threadId":    thread.ID,
		"objective":   "Ship the structured goal contract",
		"status":      "active",
		"tokenBudget": 1000,
	}))
	if setResp.Error != nil {
		t.Fatalf("thread/goal/set error: %v", setResp.Error)
	}
	var created protocol.ThreadGoalSetResponse
	decodeResult(t, setResp, &created)
	if created.Goal.ThreadID != thread.ID || created.Goal.Objective != "Ship the structured goal contract" ||
		created.Goal.Status != protocol.ThreadGoalActive || created.Goal.TokenBudget == nil || *created.Goal.TokenBudget != 1000 ||
		created.Goal.TokensUsed != 0 || created.Goal.TimeUsedSeconds != 0 || created.Goal.CreatedAt == 0 || created.Goal.UpdatedAt < created.Goal.CreatedAt ||
		created.Set == nil || !*created.Set || created.Thread == nil {
		t.Fatalf("created goal = %#v", created)
	}
	createdAt := created.Goal.CreatedAt
	events := server.DrainNotifications()
	assertNotificationMethods(t, events, "thread/settings/updated", "thread/goal/updated")
	var updated protocol.ThreadGoalUpdatedNotification
	if err := json.Unmarshal(events[1].Params, &updated); err != nil {
		t.Fatalf("decode goal update: %v", err)
	}
	if updated.ThreadID != thread.ID || updated.TurnID != nil || !reflect.DeepEqual(updated.Goal, created.Goal) || updated.Thread == nil || updated.At == nil {
		t.Fatalf("goal update = %#v", updated)
	}

	turn, err := st.CreateTurn(ctx, store.CreateTurnRequest{ThreadID: thread.ID})
	if err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}
	if _, err := st.StartTurn(ctx, turn.ID); err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if _, err := st.CompleteTurn(ctx, store.CompleteTurnRequest{
		ID:     turn.ID,
		Status: store.TurnCompleted,
		Usage: map[string]any{"usage": map[string]any{
			"inputTokens":  600,
			"outputTokens": 500,
		}},
	}); err != nil {
		t.Fatalf("CompleteTurn: %v", err)
	}

	getResp = server.HandleRequest(ctx, request("thread/goal/get", map[string]any{"threadId": thread.ID}))
	if getResp.Error != nil {
		t.Fatalf("thread/goal/get after usage error: %v", getResp.Error)
	}
	var accounted protocol.ThreadGoalGetResponse
	decodeResult(t, getResp, &accounted)
	if accounted.Goal == nil || accounted.Goal.TokensUsed != 1100 || accounted.Goal.Status != protocol.ThreadGoalBudgetLimited || accounted.Goal.CreatedAt != createdAt {
		t.Fatalf("accounted goal = %#v", accounted)
	}

	updateResp := server.HandleRequest(ctx, request("thread/goal/set", map[string]any{
		"threadId":    thread.ID,
		"status":      "blocked",
		"tokenBudget": nil,
	}))
	if updateResp.Error != nil {
		t.Fatalf("thread/goal/set update error: %v", updateResp.Error)
	}
	var changed protocol.ThreadGoalSetResponse
	decodeResult(t, updateResp, &changed)
	if changed.Goal.Objective != created.Goal.Objective || changed.Goal.Status != protocol.ThreadGoalBlocked ||
		changed.Goal.TokenBudget != nil || changed.Goal.TokensUsed != 1100 || changed.Goal.CreatedAt != createdAt {
		t.Fatalf("changed goal = %#v", changed)
	}
	_ = server.DrainNotifications()

	clearResp := server.HandleRequest(ctx, request("thread/goal/clear", map[string]any{"threadId": thread.ID}))
	if clearResp.Error != nil {
		t.Fatalf("thread/goal/clear error: %v", clearResp.Error)
	}
	var cleared protocol.ThreadGoalClearResponse
	decodeResult(t, clearResp, &cleared)
	if !cleared.Cleared || cleared.ThreadID != thread.ID || cleared.Thread == nil {
		t.Fatalf("cleared goal = %#v", cleared)
	}
	events = server.DrainNotifications()
	assertNotificationMethods(t, events, "thread/settings/updated", "thread/goal/cleared")
	var clearedNotice protocol.ThreadGoalClearedNotification
	if err := json.Unmarshal(events[1].Params, &clearedNotice); err != nil {
		t.Fatalf("decode goal cleared: %v", err)
	}
	if clearedNotice.ThreadID != thread.ID || clearedNotice.Thread == nil || clearedNotice.At == nil {
		t.Fatalf("goal cleared notification = %#v", clearedNotice)
	}

	clearResp = server.HandleRequest(ctx, request("thread/goal/clear", map[string]any{"threadId": thread.ID}))
	if clearResp.Error != nil {
		t.Fatalf("thread/goal/clear no-op error: %v", clearResp.Error)
	}
	cleared = protocol.ThreadGoalClearResponse{}
	decodeResult(t, clearResp, &cleared)
	if cleared.Cleared {
		t.Fatalf("clear no-op = %#v", cleared)
	}
	if events := server.DrainNotifications(); len(events) != 0 {
		t.Fatalf("clear no-op emitted notifications: %#v", events)
	}
}

func TestServerThreadGoalCompatibilityAndValidation(t *testing.T) {
	ctx := context.Background()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "threads.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	legacy, err := st.CreateThread(ctx, store.CreateThreadRequest{
		Title:    "Legacy goal",
		Settings: map[string]any{threadGoalSettingKey: "preserve this legacy objective"},
	})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	longLegacy, err := st.CreateThread(ctx, store.CreateThreadRequest{
		Title:    "Long legacy goal",
		Settings: map[string]any{threadGoalSettingKey: strings.Repeat("x", maxThreadGoalObjectiveChars+1)},
	})
	if err != nil {
		t.Fatalf("CreateThread long legacy: %v", err)
	}
	empty, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Validation"})
	if err != nil {
		t.Fatalf("CreateThread validation: %v", err)
	}
	server := readyServer(WithStore(st))

	legacyResp := server.HandleRequest(ctx, request("thread/goal/get", map[string]any{"threadId": legacy.ID}))
	if legacyResp.Error != nil {
		t.Fatalf("legacy thread/goal/get error: %v", legacyResp.Error)
	}
	var migrated protocol.ThreadGoalGetResponse
	decodeResult(t, legacyResp, &migrated)
	if migrated.Goal == nil || migrated.Goal.ThreadID != legacy.ID || migrated.Goal.Objective != "preserve this legacy objective" ||
		migrated.Goal.Status != protocol.ThreadGoalActive || migrated.Goal.CreatedAt != legacy.CreatedAt.Unix() {
		t.Fatalf("migrated legacy goal = %#v", migrated)
	}
	longLegacyResp := server.HandleRequest(ctx, request("thread/goal/get", map[string]any{"threadId": longLegacy.ID}))
	if longLegacyResp.Error != nil {
		t.Fatalf("long legacy thread/goal/get error: %v", longLegacyResp.Error)
	}
	var bounded protocol.ThreadGoalGetResponse
	decodeResult(t, longLegacyResp, &bounded)
	if bounded.Goal == nil || utf8.RuneCountInString(bounded.Goal.Objective) != maxThreadGoalObjectiveChars {
		t.Fatalf("bounded legacy goal = %#v", bounded.Goal)
	}

	for name, params := range map[string]map[string]any{
		"missing objective": {"threadId": empty.ID, "status": "active"},
		"blank objective":   {"threadId": empty.ID, "objective": "   "},
		"long objective":    {"threadId": empty.ID, "objective": strings.Repeat("x", 4001)},
		"invalid status":    {"threadId": empty.ID, "objective": "valid", "status": "unknown"},
		"invalid budget":    {"threadId": empty.ID, "objective": "valid", "tokenBudget": 0},
	} {
		t.Run(name, func(t *testing.T) {
			resp := server.HandleRequest(ctx, request("thread/goal/set", params))
			if resp.Error == nil || resp.Error.Code != protocol.CodeInvalidParams {
				t.Fatalf("error = %#v, want invalid params", resp.Error)
			}
		})
	}
}

func TestServerThreadGoalPersistsAcrossRestart(t *testing.T) {
	ctx := context.Background()
	databasePath := filepath.Join(t.TempDir(), "threads.db")
	st, err := store.NewSQLiteStore(databasePath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	thread, err := st.CreateThread(ctx, store.CreateThreadRequest{Title: "Persistent goal"})
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	server := readyServer(WithStore(st))
	setResp := server.HandleRequest(ctx, request("thread/goal/set", map[string]any{
		"threadId":    thread.ID,
		"objective":   "Survive a daemon restart",
		"tokenBudget": 500,
	}))
	if setResp.Error != nil {
		t.Fatalf("thread/goal/set error: %v", setResp.Error)
	}
	var created protocol.ThreadGoalSetResponse
	decodeResult(t, setResp, &created)
	updateResp := server.HandleRequest(ctx, request("thread/goal/set", map[string]any{
		"threadId":    thread.ID,
		"status":      "paused",
		"tokenBudget": nil,
	}))
	if updateResp.Error != nil {
		t.Fatalf("thread/goal/set update error: %v", updateResp.Error)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := store.NewSQLiteStore(databasePath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := readyServer(WithStore(reopened))
	getResp := restarted.HandleRequest(ctx, request("thread/goal/get", map[string]any{"threadId": thread.ID}))
	if getResp.Error != nil {
		t.Fatalf("thread/goal/get after restart error: %v", getResp.Error)
	}
	var got protocol.ThreadGoalGetResponse
	decodeResult(t, getResp, &got)
	if got.Goal == nil || got.Goal.Objective != "Survive a daemon restart" || got.Goal.Status != protocol.ThreadGoalPaused ||
		got.Goal.TokenBudget != nil || got.Goal.CreatedAt != created.Goal.CreatedAt || got.Goal.UpdatedAt < got.Goal.CreatedAt {
		t.Fatalf("restarted goal = %#v", got.Goal)
	}
}

func TestThreadGoalUsageFromTurns(t *testing.T) {
	goal := protocol.ThreadGoal{CreatedAt: time.Unix(100, 0).Unix()}
	turns := []*store.Turn{
		{CreatedAt: time.Unix(90, 0), StartedAt: time.Unix(90, 0), CompletedAt: time.Unix(99, 0), Usage: map[string]any{"usage": map[string]any{"inputTokens": 100}}},
		{CreatedAt: time.Unix(101, 0), StartedAt: time.Unix(102, 0), CompletedAt: time.Unix(107, 0), Usage: map[string]any{"usage": map[string]any{"inputTokens": 200, "outputTokens": 50}}},
		{CreatedAt: time.Unix(108, 0), StartedAt: time.Unix(109, 0), CompletedAt: time.Unix(112, 0), Usage: map[string]any{"usage": map[string]any{"inputTokens": 20}}},
	}
	tokens, elapsed := threadGoalUsageFromTurns(goal, turns)
	if tokens != 270 || elapsed != 8 {
		t.Fatalf("usage = %d tokens/%d seconds, want 270/8", tokens, elapsed)
	}
}

func TestThreadGoalLegacyProjectionVariants(t *testing.T) {
	createdAt := time.Unix(100, 0).UTC()
	thread := &store.Thread{ID: "thread-legacy", CreatedAt: createdAt, UpdatedAt: createdAt.Add(time.Minute)}
	if _, ok := threadGoalFromStored(nil); ok {
		t.Fatal("nil thread has a goal")
	}
	if _, ok := threadGoalFromStored(thread); ok {
		t.Fatal("thread without settings has a goal")
	}

	goal := threadGoalFromValue(thread, &protocol.ThreadGoal{
		Objective: "pointer goal",
		Status:    protocol.ThreadGoalComplete,
		CreatedAt: createdAt.Unix(),
		UpdatedAt: createdAt.Unix(),
	})
	if goal.ThreadID != thread.ID || goal.Objective != "pointer goal" || goal.Status != protocol.ThreadGoalComplete {
		t.Fatalf("pointer goal = %#v", goal)
	}
	goal = threadGoalFromValue(thread, json.RawMessage(`{
		"objective":"raw goal",
		"status":"usage_limited",
		"tokenBudget":-1,
		"tokensUsed":-2,
		"timeUsedSeconds":-3
	}`))
	if goal.Objective != "raw goal" || goal.Status != protocol.ThreadGoalUsageLimited || goal.TokenBudget != nil ||
		goal.TokensUsed != 0 || goal.TimeUsedSeconds != 0 || goal.CreatedAt != createdAt.Unix() {
		t.Fatalf("raw goal = %#v", goal)
	}
	goal = threadGoalFromValue(thread, []byte(`"byte goal"`))
	if goal.Objective != "byte goal" || goal.Status != protocol.ThreadGoalActive {
		t.Fatalf("byte goal = %#v", goal)
	}
	goal = threadGoalFromValue(thread, map[string]any{"custom": true})
	if goal.Objective != `{"custom":true}` {
		t.Fatalf("object fallback goal = %#v", goal)
	}
	if normalizeStoredThreadGoalStatus("budget_limited") != protocol.ThreadGoalBudgetLimited ||
		normalizeStoredThreadGoalStatus("invalid") != protocol.ThreadGoalActive {
		t.Fatal("legacy status normalization failed")
	}

	if params, rpcErr := decodeThreadGoalGetParams(json.RawMessage(`{"id":"legacy-id"}`)); rpcErr != nil || params.EffectiveThreadID() != "legacy-id" {
		t.Fatalf("legacy get params = %#v/%#v", params, rpcErr)
	}
	if _, rpcErr := decodeThreadGoalClearParams(json.RawMessage(`{"threadId":42}`)); rpcErr == nil || rpcErr.Code != protocol.CodeInvalidParams {
		t.Fatalf("malformed clear params error = %#v", rpcErr)
	}
	if _, rpcErr := decodeThreadGoalSetParams(json.RawMessage(`{}`)); rpcErr == nil || rpcErr.Code != protocol.CodeInvalidParams {
		t.Fatalf("missing set id error = %#v", rpcErr)
	}
	publishThreadGoalTurnUsage(nil, nil, nil)
}
