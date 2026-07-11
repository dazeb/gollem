package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ThreadId matches the generated public identifier contract. Generation uses
// UUIDv7 values, but the standalone wire type is intentionally any string.
type ThreadId string

func (id ThreadId) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(id))
}

func (id *ThreadId) UnmarshalJSON(data []byte) error {
	if id == nil {
		return errors.New("decode thread id into nil receiver")
	}
	value, err := decodeOpenThreadTurnString(data, "thread id")
	if err != nil {
		return err
	}
	*id = ThreadId(value)
	return nil
}

type NonSteerableTurnKind string

const (
	NonSteerableTurnKindReview  NonSteerableTurnKind = "review"
	NonSteerableTurnKindCompact NonSteerableTurnKind = "compact"
)

func (k NonSteerableTurnKind) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(k, "non-steerable turn kind", NonSteerableTurnKind.valid)
}

func (k *NonSteerableTurnKind) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, k, "non-steerable turn kind", NonSteerableTurnKind.valid)
}

func (k NonSteerableTurnKind) valid() bool {
	return k == NonSteerableTurnKindReview || k == NonSteerableTurnKindCompact
}

type TurnItemsView string

const (
	TurnItemsViewNotLoaded TurnItemsView = "notLoaded"
	TurnItemsViewSummary   TurnItemsView = "summary"
	TurnItemsViewFull      TurnItemsView = "full"
)

func (v TurnItemsView) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(v, "turn items view", TurnItemsView.valid)
}

func (v *TurnItemsView) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, v, "turn items view", TurnItemsView.valid)
}

func (v TurnItemsView) valid() bool {
	switch v {
	case TurnItemsViewNotLoaded, TurnItemsViewSummary, TurnItemsViewFull:
		return true
	default:
		return false
	}
}

type TurnStatus string

const (
	TurnStatusCompleted   TurnStatus = "completed"
	TurnStatusInterrupted TurnStatus = "interrupted"
	TurnStatusFailed      TurnStatus = "failed"
	TurnStatusInProgress  TurnStatus = "inProgress"
)

func (s TurnStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "turn status", TurnStatus.valid)
}

func (s *TurnStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "turn status", TurnStatus.valid)
}

func (s TurnStatus) valid() bool {
	switch s {
	case TurnStatusCompleted, TurnStatusInterrupted, TurnStatusFailed, TurnStatusInProgress:
		return true
	default:
		return false
	}
}

type ThreadActiveFlag string

const (
	ThreadActiveFlagWaitingOnApproval  ThreadActiveFlag = "waitingOnApproval"
	ThreadActiveFlagWaitingOnUserInput ThreadActiveFlag = "waitingOnUserInput"
)

func (f ThreadActiveFlag) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(f, "thread active flag", ThreadActiveFlag.valid)
}

func (f *ThreadActiveFlag) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, f, "thread active flag", ThreadActiveFlag.valid)
}

func (f ThreadActiveFlag) valid() bool {
	return f == ThreadActiveFlagWaitingOnApproval || f == ThreadActiveFlagWaitingOnUserInput
}

// ThreadSource is the public thread-provenance string. Gollem's filter-facing
// ThreadSourceKind remains a separate closed enum.
type ThreadSource string

func (s ThreadSource) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

func (s *ThreadSource) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode thread source into nil receiver")
	}
	value, err := decodeOpenThreadTurnString(data, "thread source")
	if err != nil {
		return err
	}
	*s = ThreadSource(value)
	return nil
}

type GitInfo struct {
	SHA       *string `json:"sha"`
	Branch    *string `json:"branch"`
	OriginURL *string `json:"originUrl"`
}

func (i GitInfo) MarshalJSON() ([]byte, error) {
	type wire GitInfo
	return json.Marshal(wire(i))
}

func (i *GitInfo) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode git info into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("git info cannot be null")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("decode git info: %w", err)
	}
	if err := rejectThreadItemFields(payload, "git info", "sha", "branch", "originUrl"); err != nil {
		return err
	}
	sha, err := decodeRequiredNullableThreadItemString(payload, "git info", "sha")
	if err != nil {
		return err
	}
	branch, err := decodeRequiredNullableThreadItemString(payload, "git info", "branch")
	if err != nil {
		return err
	}
	originURL, err := decodeRequiredNullableThreadItemString(payload, "git info", "originUrl")
	if err != nil {
		return err
	}
	*i = GitInfo{SHA: sha, Branch: branch, OriginURL: originURL}
	return nil
}

func decodeOpenThreadTurnString(data []byte, name string) (string, error) {
	if isJSONNull(data) {
		return "", fmt.Errorf("%s cannot be null", name)
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("decode %s: %w", name, err)
	}
	return value, nil
}

func marshalThreadTurnLeafEnum[T ~string](value T, name string, valid func(T) bool) ([]byte, error) {
	if !valid(value) {
		return nil, fmt.Errorf("invalid %s %q", name, value)
	}
	return json.Marshal(string(value))
}

func unmarshalThreadTurnLeafEnum[T ~string](data []byte, value *T, name string, valid func(T) bool) error {
	if value == nil {
		return fmt.Errorf("decode %s into nil receiver", name)
	}
	raw, err := decodeOpenThreadTurnString(data, name)
	if err != nil {
		return err
	}
	decoded := T(raw)
	if !valid(decoded) {
		return fmt.Errorf("invalid %s %q", name, raw)
	}
	*value = decoded
	return nil
}
