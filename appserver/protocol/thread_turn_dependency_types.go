package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

type CodexErrorInfo struct {
	raw json.RawMessage
}

func (i CodexErrorInfo) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnDependency(i.raw, "Codex error info", validateCodexErrorInfoJSON)
}

func (i *CodexErrorInfo) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode Codex error info into nil receiver")
	}
	canonical, err := validateCodexErrorInfoJSON(data)
	if err != nil {
		return err
	}
	i.raw = canonical
	return nil
}

type TurnError struct {
	Message           string          `json:"message"`
	CodexErrorInfo    *CodexErrorInfo `json:"codexErrorInfo"`
	AdditionalDetails *string         `json:"additionalDetails"`
}

func (e TurnError) MarshalJSON() ([]byte, error) {
	type wire TurnError
	return json.Marshal(wire(e))
}

func (e *TurnError) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode turn error into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"turn error",
		"message",
		"codexErrorInfo",
		"additionalDetails",
	)
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, "turn error", "message")
	if err != nil {
		return err
	}
	codexErrorInfo, err := decodeRequiredNullableThreadItemValue[CodexErrorInfo](payload, "turn error", "codexErrorInfo")
	if err != nil {
		return err
	}
	additionalDetails, err := decodeRequiredNullableThreadItemValue[string](payload, "turn error", "additionalDetails")
	if err != nil {
		return err
	}
	*e = TurnError{
		Message:           message,
		CodexErrorInfo:    codexErrorInfo,
		AdditionalDetails: additionalDetails,
	}
	return nil
}

type ThreadStatus struct {
	raw json.RawMessage
}

func (s ThreadStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnDependency(s.raw, "thread status", validateThreadStatusJSON)
}

func (s *ThreadStatus) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode thread status into nil receiver")
	}
	canonical, err := validateThreadStatusJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

type SubAgentSource struct {
	raw json.RawMessage
}

func (s SubAgentSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnDependency(s.raw, "subagent source", validateSubAgentSourceJSON)
}

func (s *SubAgentSource) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode subagent source into nil receiver")
	}
	canonical, err := validateSubAgentSourceJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

type SessionSource struct {
	raw json.RawMessage
}

func (s SessionSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnDependency(s.raw, "session source", validateSessionSourceJSON)
}

func (s *SessionSource) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode session source into nil receiver")
	}
	canonical, err := validateSessionSourceJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

var codexErrorInfoStrings = []string{
	"contextWindowExceeded",
	"sessionBudgetExceeded",
	"usageLimitExceeded",
	"serverOverloaded",
	"cyberPolicy",
	"internalServerError",
	"unauthorized",
	"badRequest",
	"threadRollbackFailed",
	"sandboxError",
	"other",
}

var codexErrorHTTPVariants = []string{
	"httpConnectionFailed",
	"responseStreamConnectionFailed",
	"responseStreamDisconnected",
	"responseTooManyFailedAttempts",
}

func validateCodexErrorInfoJSON(data []byte) (json.RawMessage, error) {
	if scalar, ok, err := decodeThreadTurnDependencyString(data, "Codex error info"); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if !slices.Contains(codexErrorInfoStrings, scalar) {
			return nil, fmt.Errorf("unknown Codex error info %q", scalar)
		}
		return json.Marshal(scalar)
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"Codex error info",
		"httpConnectionFailed",
		"responseStreamConnectionFailed",
		"responseStreamDisconnected",
		"responseTooManyFailedAttempts",
		"activeTurnNotSteerable",
	)
	if err != nil {
		return nil, err
	}
	if len(payload) != 1 {
		return nil, errors.New("codex error info requires exactly one variant")
	}
	for _, name := range codexErrorHTTPVariants {
		if raw, ok := payload[name]; ok {
			return canonicalCodexHTTPError(name, raw)
		}
	}
	raw := payload["activeTurnNotSteerable"]
	nested, err := decodeExactThreadItemObject(raw, "active-turn-not-steerable error", "turnKind")
	if err != nil {
		return nil, err
	}
	turnKind, err := decodeRequiredThreadItemValue[NonSteerableTurnKind](nested, "active-turn-not-steerable error", "turnKind")
	if err != nil {
		return nil, err
	}
	value := struct {
		TurnKind NonSteerableTurnKind `json:"turnKind"`
	}{TurnKind: turnKind}
	return json.Marshal(map[string]any{"activeTurnNotSteerable": value})
}

func canonicalCodexHTTPError(name string, data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(data, name+" error", "httpStatusCode")
	if err != nil {
		return nil, err
	}
	status, err := decodeRequiredNullableThreadItemValue[uint16](payload, name+" error", "httpStatusCode")
	if err != nil {
		return nil, err
	}
	nested := struct {
		HTTPStatusCode *uint16 `json:"httpStatusCode"`
	}{HTTPStatusCode: status}
	return json.Marshal(map[string]any{name: nested})
}

func validateThreadStatusJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(data, "thread status", "type", "activeFlags")
	if err != nil {
		return nil, err
	}
	statusType, err := decodeRequiredThreadItemValue[string](payload, "thread status", "type")
	if err != nil {
		return nil, err
	}
	switch statusType {
	case "notLoaded", "idle", "systemError":
		if err := rejectThreadItemFields(payload, statusType+" thread status", "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: statusType})
	case "active":
		flags, err := decodeRequiredThreadItemArray[ThreadActiveFlag](payload, "active thread status", "activeFlags")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type        string             `json:"type"`
			ActiveFlags []ThreadActiveFlag `json:"activeFlags"`
		}{Type: statusType, ActiveFlags: flags})
	default:
		return nil, fmt.Errorf("unknown thread status type %q", statusType)
	}
}

func validateSubAgentSourceJSON(data []byte) (json.RawMessage, error) {
	if scalar, ok, err := decodeThreadTurnDependencyString(data, "subagent source"); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if !slices.Contains([]string{"review", "compact", "memory_consolidation"}, scalar) {
			return nil, fmt.Errorf("unknown subagent source %q", scalar)
		}
		return json.Marshal(scalar)
	}
	payload, err := decodeExactThreadItemObject(data, "subagent source", "thread_spawn", "other")
	if err != nil {
		return nil, err
	}
	if len(payload) != 1 {
		return nil, errors.New("subagent source requires exactly one variant")
	}
	if _, ok := payload["other"]; ok {
		other, err := decodeRequiredThreadItemValue[string](payload, "other subagent source", "other")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Other string `json:"other"`
		}{Other: other})
	}
	spawn, err := decodeExactThreadItemObject(
		payload["thread_spawn"],
		"thread-spawn subagent source",
		"parent_thread_id",
		"depth",
		"agent_path",
		"agent_nickname",
		"agent_role",
	)
	if err != nil {
		return nil, err
	}
	parentThreadID, err := decodeRequiredThreadItemValue[ThreadId](spawn, "thread-spawn subagent source", "parent_thread_id")
	if err != nil {
		return nil, err
	}
	depth, err := decodeRequiredThreadItemValue[int32](spawn, "thread-spawn subagent source", "depth")
	if err != nil {
		return nil, err
	}
	agentPath, err := decodeRequiredNullableThreadItemValue[AgentPath](spawn, "thread-spawn subagent source", "agent_path")
	if err != nil {
		return nil, err
	}
	agentNickname, err := decodeRequiredNullableThreadItemValue[string](spawn, "thread-spawn subagent source", "agent_nickname")
	if err != nil {
		return nil, err
	}
	agentRole, err := decodeRequiredNullableThreadItemValue[string](spawn, "thread-spawn subagent source", "agent_role")
	if err != nil {
		return nil, err
	}
	nested := struct {
		ParentThreadID ThreadId   `json:"parent_thread_id"`
		Depth          int32      `json:"depth"`
		AgentPath      *AgentPath `json:"agent_path"`
		AgentNickname  *string    `json:"agent_nickname"`
		AgentRole      *string    `json:"agent_role"`
	}{
		ParentThreadID: parentThreadID,
		Depth:          depth,
		AgentPath:      agentPath,
		AgentNickname:  agentNickname,
		AgentRole:      agentRole,
	}
	return json.Marshal(map[string]any{"thread_spawn": nested})
}

func validateSessionSourceJSON(data []byte) (json.RawMessage, error) {
	if scalar, ok, err := decodeThreadTurnDependencyString(data, "session source"); ok || err != nil {
		if err != nil {
			return nil, err
		}
		if !slices.Contains([]string{"cli", "vscode", "exec", "appServer", "unknown"}, scalar) {
			return nil, fmt.Errorf("unknown session source %q", scalar)
		}
		return json.Marshal(scalar)
	}
	payload, err := decodeExactThreadItemObject(data, "session source", "custom", "subAgent")
	if err != nil {
		return nil, err
	}
	if len(payload) != 1 {
		return nil, errors.New("session source requires exactly one variant")
	}
	if _, ok := payload["custom"]; ok {
		custom, err := decodeRequiredThreadItemValue[string](payload, "custom session source", "custom")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Custom string `json:"custom"`
		}{Custom: custom})
	}
	subAgent, err := decodeRequiredThreadItemValue[SubAgentSource](payload, "subagent session source", "subAgent")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		SubAgent SubAgentSource `json:"subAgent"`
	}{SubAgent: subAgent})
}

func decodeThreadTurnDependencyString(data []byte, name string) (string, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '"' {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return "", true, fmt.Errorf("decode %s: %w", name, err)
	}
	return value, true, nil
}

func marshalThreadTurnDependency(
	raw json.RawMessage,
	name string,
	validate func([]byte) (json.RawMessage, error),
) ([]byte, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s has no value", name)
	}
	return validate(raw)
}
