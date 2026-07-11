package protocol

import (
	"bytes"
	"encoding/json"
	"time"
)

type ThreadGoalStatus string

const (
	ThreadGoalActive        ThreadGoalStatus = "active"
	ThreadGoalPaused        ThreadGoalStatus = "paused"
	ThreadGoalBlocked       ThreadGoalStatus = "blocked"
	ThreadGoalUsageLimited  ThreadGoalStatus = "usageLimited"
	ThreadGoalBudgetLimited ThreadGoalStatus = "budgetLimited"
	ThreadGoalComplete      ThreadGoalStatus = "complete"
)

func (s ThreadGoalStatus) Valid() bool {
	switch s {
	case ThreadGoalActive, ThreadGoalPaused, ThreadGoalBlocked,
		ThreadGoalUsageLimited, ThreadGoalBudgetLimited, ThreadGoalComplete:
		return true
	default:
		return false
	}
}

type ThreadGoal struct {
	ThreadID        string           `json:"threadId"`
	Objective       string           `json:"objective"`
	Status          ThreadGoalStatus `json:"status"`
	TokenBudget     *int64           `json:"tokenBudget"`
	TokensUsed      int64            `json:"tokensUsed"`
	TimeUsedSeconds int64            `json:"timeUsedSeconds"`
	CreatedAt       int64            `json:"createdAt"`
	UpdatedAt       int64            `json:"updatedAt"`
}

type ThreadGoalSetParams struct {
	ThreadID    string            `json:"threadId"`
	Objective   *string           `json:"objective,omitempty"`
	Status      *ThreadGoalStatus `json:"status,omitempty"`
	TokenBudget *int64            `json:"tokenBudget,omitempty"`
	ID          string            `json:"id,omitempty"`
	Goal        json.RawMessage   `json:"goal,omitempty"`
	Text        json.RawMessage   `json:"text,omitempty"`
	Value       json.RawMessage   `json:"value,omitempty"`

	tokenBudgetPresent bool
}

func (p *ThreadGoalSetParams) UnmarshalJSON(data []byte) error {
	type wire ThreadGoalSetParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ThreadGoalSetParams(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, p.tokenBudgetPresent = fields["tokenBudget"]
	return nil
}

func (p ThreadGoalSetParams) MarshalJSON() ([]byte, error) {
	type wire ThreadGoalSetParams
	data, err := json.Marshal(wire(p))
	if err != nil || !p.tokenBudgetPresent || p.TokenBudget != nil {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	fields["tokenBudget"] = json.RawMessage("null")
	return json.Marshal(fields)
}

func (p ThreadGoalSetParams) EffectiveThreadID() string {
	if p.ThreadID != "" {
		return p.ThreadID
	}
	return p.ID
}

func (p ThreadGoalSetParams) HasTokenBudget() bool {
	return p.tokenBudgetPresent || p.TokenBudget != nil
}

func (p *ThreadGoalSetParams) SetTokenBudget(value *int64) {
	p.TokenBudget = value
	p.tokenBudgetPresent = true
}

func (p ThreadGoalSetParams) LegacyGoal() (json.RawMessage, bool) {
	for _, raw := range []json.RawMessage{p.Goal, p.Text, p.Value} {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
			return append(json.RawMessage(nil), trimmed...), true
		}
	}
	return nil, false
}

type ThreadGoalSetResponse struct {
	Goal     ThreadGoal    `json:"goal"`
	ThreadID string        `json:"threadId,omitempty"`
	Set      *bool         `json:"set,omitempty"`
	Thread   *ThreadRecord `json:"thread,omitempty"`
}

type ThreadGoalGetParams struct {
	ThreadID string `json:"threadId"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadGoalGetParams) EffectiveThreadID() string {
	if p.ThreadID != "" {
		return p.ThreadID
	}
	return p.ID
}

type ThreadGoalGetResponse struct {
	Goal     *ThreadGoal   `json:"goal"`
	ThreadID string        `json:"threadId,omitempty"`
	Set      *bool         `json:"set,omitempty"`
	Thread   *ThreadRecord `json:"thread,omitempty"`
}

type ThreadGoalClearParams struct {
	ThreadID string `json:"threadId"`
	ID       string `json:"id,omitempty"`
}

func (p ThreadGoalClearParams) EffectiveThreadID() string {
	if p.ThreadID != "" {
		return p.ThreadID
	}
	return p.ID
}

type ThreadGoalClearResponse struct {
	Cleared  bool          `json:"cleared"`
	ThreadID string        `json:"threadId,omitempty"`
	Thread   *ThreadRecord `json:"thread,omitempty"`
}

type ThreadGoalUpdatedNotification struct {
	ThreadID string        `json:"threadId"`
	TurnID   *string       `json:"turnId"`
	Goal     ThreadGoal    `json:"goal"`
	Thread   *ThreadRecord `json:"thread,omitempty"`
	At       *time.Time    `json:"at,omitempty"`
}

type ThreadGoalClearedNotification struct {
	ThreadID string        `json:"threadId"`
	Thread   *ThreadRecord `json:"thread,omitempty"`
	At       *time.Time    `json:"at,omitempty"`
}
