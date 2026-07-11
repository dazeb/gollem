package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AgentPath is the public logical agent identifier path. It is not a local
// filesystem path and intentionally has no normalization or non-empty rule.
type AgentPath string

func (p AgentPath) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(p))
}

func (p *AgentPath) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode agent path into nil receiver")
	}
	value, err := decodeRequiredCollabString(data, "agent path")
	if err != nil {
		return err
	}
	*p = AgentPath(value)
	return nil
}

// ReasoningEffort is a non-empty provider-advertised reasoning effort value.
// The public TypeScript contract remains string because the value is open.
type ReasoningEffort string

func (e ReasoningEffort) MarshalJSON() ([]byte, error) {
	if e == "" {
		return nil, errors.New("reasoning effort cannot be empty")
	}
	return json.Marshal(string(e))
}

func (e *ReasoningEffort) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode reasoning effort into nil receiver")
	}
	value, err := decodeRequiredCollabString(data, "reasoning effort")
	if err != nil {
		return err
	}
	if value == "" {
		return errors.New("reasoning effort cannot be empty")
	}
	*e = ReasoningEffort(value)
	return nil
}

type CollabAgentStatus string

const (
	CollabAgentStatusPendingInit CollabAgentStatus = "pendingInit"
	CollabAgentStatusRunning     CollabAgentStatus = "running"
	CollabAgentStatusInterrupted CollabAgentStatus = "interrupted"
	CollabAgentStatusCompleted   CollabAgentStatus = "completed"
	CollabAgentStatusErrored     CollabAgentStatus = "errored"
	CollabAgentStatusShutdown    CollabAgentStatus = "shutdown"
	CollabAgentStatusNotFound    CollabAgentStatus = "notFound"
)

func (s CollabAgentStatus) MarshalJSON() ([]byte, error) {
	return marshalClosedCollabValue("collab agent status", s, collabAgentStatuses...)
}

func (s *CollabAgentStatus) UnmarshalJSON(data []byte) error {
	return unmarshalClosedCollabValue(data, "collab agent status", s, collabAgentStatuses...)
}

var collabAgentStatuses = []CollabAgentStatus{
	CollabAgentStatusPendingInit,
	CollabAgentStatusRunning,
	CollabAgentStatusInterrupted,
	CollabAgentStatusCompleted,
	CollabAgentStatusErrored,
	CollabAgentStatusShutdown,
	CollabAgentStatusNotFound,
}

type CollabAgentTool string

const (
	CollabAgentToolSpawnAgent  CollabAgentTool = "spawnAgent"
	CollabAgentToolSendInput   CollabAgentTool = "sendInput"
	CollabAgentToolResumeAgent CollabAgentTool = "resumeAgent"
	CollabAgentToolWait        CollabAgentTool = "wait"
	CollabAgentToolCloseAgent  CollabAgentTool = "closeAgent"
)

func (t CollabAgentTool) MarshalJSON() ([]byte, error) {
	return marshalClosedCollabValue("collab agent tool", t, collabAgentTools...)
}

func (t *CollabAgentTool) UnmarshalJSON(data []byte) error {
	return unmarshalClosedCollabValue(data, "collab agent tool", t, collabAgentTools...)
}

var collabAgentTools = []CollabAgentTool{
	CollabAgentToolSpawnAgent,
	CollabAgentToolSendInput,
	CollabAgentToolResumeAgent,
	CollabAgentToolWait,
	CollabAgentToolCloseAgent,
}

type CollabAgentToolCallStatus string

const (
	CollabAgentToolCallStatusInProgress CollabAgentToolCallStatus = "inProgress"
	CollabAgentToolCallStatusCompleted  CollabAgentToolCallStatus = "completed"
	CollabAgentToolCallStatusFailed     CollabAgentToolCallStatus = "failed"
)

func (s CollabAgentToolCallStatus) MarshalJSON() ([]byte, error) {
	return marshalClosedCollabValue("collab agent tool-call status", s, collabAgentToolCallStatuses...)
}

func (s *CollabAgentToolCallStatus) UnmarshalJSON(data []byte) error {
	return unmarshalClosedCollabValue(data, "collab agent tool-call status", s, collabAgentToolCallStatuses...)
}

var collabAgentToolCallStatuses = []CollabAgentToolCallStatus{
	CollabAgentToolCallStatusInProgress,
	CollabAgentToolCallStatusCompleted,
	CollabAgentToolCallStatusFailed,
}

type SubAgentActivityKind string

const (
	SubAgentActivityStarted     SubAgentActivityKind = "started"
	SubAgentActivityInteracted  SubAgentActivityKind = "interacted"
	SubAgentActivityInterrupted SubAgentActivityKind = "interrupted"
)

func (k SubAgentActivityKind) MarshalJSON() ([]byte, error) {
	return marshalClosedCollabValue("subagent activity kind", k, subAgentActivityKinds...)
}

func (k *SubAgentActivityKind) UnmarshalJSON(data []byte) error {
	return unmarshalClosedCollabValue(data, "subagent activity kind", k, subAgentActivityKinds...)
}

var subAgentActivityKinds = []SubAgentActivityKind{
	SubAgentActivityStarted,
	SubAgentActivityInteracted,
	SubAgentActivityInterrupted,
}

type CollabAgentState struct {
	Status  CollabAgentStatus `json:"status"`
	Message *string           `json:"message"`
}

func (s CollabAgentState) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Status  CollabAgentStatus `json:"status"`
		Message *string           `json:"message"`
	}{Status: s.Status, Message: s.Message})
}

func (s *CollabAgentState) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode collab agent state into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "collab agent state", "status", "message")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[CollabAgentStatus](payload, "collab agent state", "status")
	if err != nil {
		return err
	}
	message, err := decodeRequiredNullableThreadItemString(payload, "collab agent state", "message")
	if err != nil {
		return err
	}
	*s = CollabAgentState{Status: status, Message: message}
	return nil
}

func decodeRequiredCollabString(data []byte, name string) (string, error) {
	if isJSONNull(data) {
		return "", fmt.Errorf("%s cannot be null", name)
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("decode %s: %w", name, err)
	}
	return value, nil
}

func marshalClosedCollabValue[T ~string](name string, value T, allowed ...T) ([]byte, error) {
	if !isAllowedCollabValue(value, allowed) {
		return nil, fmt.Errorf("unknown %s %q", name, value)
	}
	return json.Marshal(string(value))
}

func unmarshalClosedCollabValue[T ~string](data []byte, name string, target *T, allowed ...T) error {
	if target == nil {
		return fmt.Errorf("decode %s into nil receiver", name)
	}
	value, err := decodeRequiredCollabString(data, name)
	if err != nil {
		return err
	}
	parsed := T(value)
	if !isAllowedCollabValue(parsed, allowed) {
		return fmt.Errorf("unknown %s %q", name, value)
	}
	*target = parsed
	return nil
}

func isAllowedCollabValue[T comparable](value T, allowed []T) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
