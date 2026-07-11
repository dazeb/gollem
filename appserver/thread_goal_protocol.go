package appserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/fugue-labs/gollem/appserver/protocol"
	"github.com/fugue-labs/gollem/appserver/store"
)

const maxThreadGoalObjectiveChars = 4_000

func decodeThreadGoalSetParams(raw json.RawMessage) (protocol.ThreadGoalSetParams, *protocol.Error) {
	var params protocol.ThreadGoalSetParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return protocol.ThreadGoalSetParams{}, rpcErr
	}
	if params.EffectiveThreadID() == "" {
		return protocol.ThreadGoalSetParams{}, invalidParams("threadId is required", nil)
	}
	return params, nil
}

func decodeThreadGoalGetParams(raw json.RawMessage) (protocol.ThreadGoalGetParams, *protocol.Error) {
	var params protocol.ThreadGoalGetParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return protocol.ThreadGoalGetParams{}, rpcErr
	}
	if params.EffectiveThreadID() == "" {
		return protocol.ThreadGoalGetParams{}, invalidParams("threadId is required", nil)
	}
	return params, nil
}

func decodeThreadGoalClearParams(raw json.RawMessage) (protocol.ThreadGoalClearParams, *protocol.Error) {
	var params protocol.ThreadGoalClearParams
	if rpcErr := decodeParams(raw, &params); rpcErr != nil {
		return protocol.ThreadGoalClearParams{}, rpcErr
	}
	if params.EffectiveThreadID() == "" {
		return protocol.ThreadGoalClearParams{}, invalidParams("threadId is required", nil)
	}
	return params, nil
}

func applyThreadGoalSet(thread *store.Thread, params protocol.ThreadGoalSetParams, turns []*store.Turn, now time.Time) (protocol.ThreadGoal, *protocol.Error) {
	goal, exists := threadGoalFromStored(thread)
	if exists {
		goal.TokensUsed, goal.TimeUsedSeconds = threadGoalUsageFromTurns(goal, turns)
		goal = applyThreadGoalBudgetStatus(goal)
	}

	legacyGoal, hasLegacyGoal := params.LegacyGoal()
	if !exists && hasLegacyGoal {
		goal = threadGoalFromValue(thread, legacyGoal)
		goal.CreatedAt = now.Unix()
		goal.UpdatedAt = now.Unix()
		goal.TokensUsed = 0
		goal.TimeUsedSeconds = 0
		exists = true
	}
	if !exists {
		if params.Objective == nil {
			return protocol.ThreadGoal{}, invalidParams("objective is required when no goal exists", nil)
		}
		goal = protocol.ThreadGoal{
			ThreadID:  thread.ID,
			Status:    protocol.ThreadGoalActive,
			CreatedAt: now.Unix(),
		}
	}

	if params.Objective != nil {
		objective, rpcErr := validateThreadGoalObjective(*params.Objective)
		if rpcErr != nil {
			return protocol.ThreadGoal{}, rpcErr
		}
		goal.Objective = objective
	} else if hasLegacyGoal {
		legacy := threadGoalFromValue(thread, legacyGoal)
		goal.Objective = legacy.Objective
		if params.Status == nil {
			goal.Status = legacy.Status
		}
		if !params.HasTokenBudget() {
			goal.TokenBudget = cloneInt64Pointer(legacy.TokenBudget)
		}
	}
	if _, rpcErr := validateThreadGoalObjective(goal.Objective); rpcErr != nil {
		return protocol.ThreadGoal{}, rpcErr
	}
	if params.Status != nil {
		if !params.Status.Valid() {
			return protocol.ThreadGoal{}, invalidParams("status is not a recognized thread goal status", nil)
		}
		goal.Status = *params.Status
	}
	if !goal.Status.Valid() {
		goal.Status = protocol.ThreadGoalActive
	}
	if params.HasTokenBudget() {
		if params.TokenBudget != nil && *params.TokenBudget <= 0 {
			return protocol.ThreadGoal{}, invalidParams("tokenBudget must be positive when provided", nil)
		}
		goal.TokenBudget = cloneInt64Pointer(params.TokenBudget)
	}
	if params.Status == nil && params.HasTokenBudget() && goal.Status == protocol.ThreadGoalBudgetLimited &&
		(goal.TokenBudget == nil || goal.TokensUsed < *goal.TokenBudget) {
		goal.Status = protocol.ThreadGoalActive
	}
	goal.ThreadID = thread.ID
	if goal.CreatedAt <= 0 {
		goal.CreatedAt = now.Unix()
	}
	goal.UpdatedAt = now.Unix()
	goal = applyThreadGoalBudgetStatus(goal)
	return goal, nil
}

func validateThreadGoalObjective(value string) (string, *protocol.Error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalidParams("goal objective must not be empty", nil)
	}
	if utf8.RuneCountInString(value) > maxThreadGoalObjectiveChars {
		return "", invalidParams(fmt.Sprintf("goal objective must be at most %d characters", maxThreadGoalObjectiveChars), nil)
	}
	return value, nil
}

func threadGoalFromStored(thread *store.Thread) (protocol.ThreadGoal, bool) {
	if thread == nil || thread.Settings == nil {
		return protocol.ThreadGoal{}, false
	}
	value, ok := thread.Settings[threadGoalSettingKey]
	if !ok || value == nil {
		return protocol.ThreadGoal{}, false
	}
	return threadGoalFromValue(thread, value), true
}

func threadGoalFromValue(thread *store.Thread, value any) protocol.ThreadGoal {
	goal := protocol.ThreadGoal{}
	switch typed := value.(type) {
	case protocol.ThreadGoal:
		goal = typed
	case *protocol.ThreadGoal:
		if typed != nil {
			goal = *typed
		}
	case string:
		goal.Objective = strings.TrimSpace(typed)
	case json.RawMessage:
		decodeThreadGoalJSON(typed, &goal)
	case []byte:
		decodeThreadGoalJSON(typed, &goal)
	default:
		if data, err := json.Marshal(value); err == nil {
			decodeThreadGoalJSON(data, &goal)
			if goal.Objective == "" {
				goal.Objective = string(data)
			}
		}
	}
	if goal.Objective == "" {
		if data, err := json.Marshal(value); err == nil && string(data) != "null" {
			goal.Objective = strings.TrimSpace(string(data))
		}
	}
	goal.Objective = truncateThreadGoalObjective(strings.TrimSpace(goal.Objective))
	goal.ThreadID = thread.ID
	goal.Status = normalizeStoredThreadGoalStatus(goal.Status)
	if goal.TokenBudget != nil && *goal.TokenBudget <= 0 {
		goal.TokenBudget = nil
	}
	if goal.TokensUsed < 0 {
		goal.TokensUsed = 0
	}
	if goal.TimeUsedSeconds < 0 {
		goal.TimeUsedSeconds = 0
	}
	if goal.CreatedAt <= 0 {
		goal.CreatedAt = thread.CreatedAt.Unix()
	}
	if goal.UpdatedAt < goal.CreatedAt {
		goal.UpdatedAt = thread.UpdatedAt.Unix()
	}
	if goal.UpdatedAt < goal.CreatedAt {
		goal.UpdatedAt = goal.CreatedAt
	}
	return goal
}

func truncateThreadGoalObjective(value string) string {
	if utf8.RuneCountInString(value) <= maxThreadGoalObjectiveChars {
		return value
	}
	count := 0
	for index := range value {
		if count == maxThreadGoalObjectiveChars {
			return value[:index]
		}
		count++
	}
	return value
}

func decodeThreadGoalJSON(data []byte, goal *protocol.ThreadGoal) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		return
	}
	if err := json.Unmarshal(data, goal); err == nil && goal.Objective != "" {
		return
	}
	var objective string
	if err := json.Unmarshal(data, &objective); err == nil {
		goal.Objective = objective
	}
}

func normalizeStoredThreadGoalStatus(status protocol.ThreadGoalStatus) protocol.ThreadGoalStatus {
	switch status {
	case "usage_limited":
		return protocol.ThreadGoalUsageLimited
	case "budget_limited":
		return protocol.ThreadGoalBudgetLimited
	default:
		if status.Valid() {
			return status
		}
		return protocol.ThreadGoalActive
	}
}

func refreshThreadGoalUsage(ctx context.Context, st store.Store, goal protocol.ThreadGoal) protocol.ThreadGoal {
	turns, err := st.ListTurns(ctx, store.TurnFilter{ThreadID: goal.ThreadID})
	if err == nil {
		goal.TokensUsed, goal.TimeUsedSeconds, goal.UpdatedAt = threadGoalAccountingFromTurns(goal, turns)
	}
	return applyThreadGoalBudgetStatus(goal)
}

func publishThreadGoalTurnUsage(notifier runtimeNotifier, st store.Store, turn *store.Turn) {
	if notifier == nil || st == nil || turn == nil {
		return
	}
	ctx := context.Background()
	thread, err := st.GetThread(ctx, turn.ThreadID)
	if err != nil {
		return
	}
	goal, ok := threadGoalFromStored(thread)
	if !ok {
		return
	}
	goal = refreshThreadGoalUsage(ctx, st, goal)
	now := time.Now().UTC()
	turnID := turn.ID
	record := protocolThreadRecordWithGoal(thread, &goal)
	notifier.PublishNotification("thread/goal/updated", protocol.ThreadGoalUpdatedNotification{
		ThreadID: thread.ID,
		TurnID:   &turnID,
		Goal:     goal,
		Thread:   &record,
		At:       &now,
	})
}

func threadGoalUsageFromTurns(goal protocol.ThreadGoal, turns []*store.Turn) (int64, int64) {
	tokens, elapsed, _ := threadGoalAccountingFromTurns(goal, turns)
	return tokens, elapsed
}

func threadGoalAccountingFromTurns(goal protocol.ThreadGoal, turns []*store.Turn) (int64, int64, int64) {
	var tokens int64
	var elapsed time.Duration
	updatedAt := goal.UpdatedAt
	for _, turn := range turns {
		if turn == nil || turn.CreatedAt.Unix() < goal.CreatedAt {
			continue
		}
		usage := runtimeUsageFromStoredMap(turn.Usage)
		tokens += int64(usage.TotalTokens())
		if !turn.StartedAt.IsZero() && !turn.CompletedAt.IsZero() && turn.CompletedAt.After(turn.StartedAt) {
			elapsed += turn.CompletedAt.Sub(turn.StartedAt)
		}
		if !turn.CompletedAt.IsZero() && turn.CompletedAt.Unix() > updatedAt {
			updatedAt = turn.CompletedAt.Unix()
		} else if turn.UpdatedAt.Unix() > updatedAt {
			updatedAt = turn.UpdatedAt.Unix()
		}
	}
	if tokens < 0 {
		tokens = 0
	}
	if elapsed < 0 {
		elapsed = 0
	}
	return tokens, int64(elapsed / time.Second), updatedAt
}

func applyThreadGoalBudgetStatus(goal protocol.ThreadGoal) protocol.ThreadGoal {
	if goal.Status == protocol.ThreadGoalActive && goal.TokenBudget != nil && goal.TokensUsed >= *goal.TokenBudget {
		goal.Status = protocol.ThreadGoalBudgetLimited
	}
	return goal
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func threadGoalGetResponse(thread *store.Thread, goal *protocol.ThreadGoal) protocol.ThreadGoalGetResponse {
	set := goal != nil
	record := protocolThreadRecordWithGoal(thread, goal)
	return protocol.ThreadGoalGetResponse{
		Goal:     goal,
		ThreadID: thread.ID,
		Set:      &set,
		Thread:   &record,
	}
}

func protocolThreadRecordWithGoal(thread *store.Thread, goal *protocol.ThreadGoal) protocol.ThreadRecord {
	record := protocolThreadRecord(thread)
	if goal == nil {
		return record
	}
	if record.Settings == nil {
		record.Settings = make(map[string]any)
	}
	record.Settings[threadGoalSettingKey] = *goal
	return record
}

func threadGoalSetResponse(thread *store.Thread, goal protocol.ThreadGoal) protocol.ThreadGoalSetResponse {
	set := true
	record := protocolThreadRecord(thread)
	return protocol.ThreadGoalSetResponse{
		Goal:     goal,
		ThreadID: thread.ID,
		Set:      &set,
		Thread:   &record,
	}
}

func threadGoalClearResponse(thread *store.Thread, cleared bool) protocol.ThreadGoalClearResponse {
	record := protocolThreadRecord(thread)
	return protocol.ThreadGoalClearResponse{
		Cleared:  cleared,
		ThreadID: thread.ID,
		Thread:   &record,
	}
}
