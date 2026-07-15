package protocol

import (
	"encoding/json"
	"errors"
)

// HookEventName is the exact closed public hook lifecycle event name. It does
// not imply that Gollem discovers or executes hooks.
type HookEventName string

const (
	HookEventNamePreToolUse        HookEventName = "preToolUse"
	HookEventNamePermissionRequest HookEventName = "permissionRequest"
	HookEventNamePostToolUse       HookEventName = "postToolUse"
	HookEventNamePreCompact        HookEventName = "preCompact"
	HookEventNamePostCompact       HookEventName = "postCompact"
	HookEventNameSessionStart      HookEventName = "sessionStart"
	HookEventNameUserPromptSubmit  HookEventName = "userPromptSubmit"
	HookEventNameSubagentStart     HookEventName = "subagentStart"
	HookEventNameSubagentStop      HookEventName = "subagentStop"
	HookEventNameStop              HookEventName = "stop"
)

func (n HookEventName) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(n, "hook event name", HookEventName.valid)
}

func (n *HookEventName) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, n, "hook event name", HookEventName.valid)
}

func (n HookEventName) valid() bool {
	switch n {
	case HookEventNamePreToolUse, HookEventNamePermissionRequest,
		HookEventNamePostToolUse, HookEventNamePreCompact, HookEventNamePostCompact,
		HookEventNameSessionStart, HookEventNameUserPromptSubmit,
		HookEventNameSubagentStart, HookEventNameSubagentStop, HookEventNameStop:
		return true
	default:
		return false
	}
}

// HookExecutionMode is the exact closed public hook execution mode.
type HookExecutionMode string

const (
	HookExecutionModeSync  HookExecutionMode = "sync"
	HookExecutionModeAsync HookExecutionMode = "async"
)

func (m HookExecutionMode) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "hook execution mode", HookExecutionMode.valid)
}

func (m *HookExecutionMode) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "hook execution mode", HookExecutionMode.valid)
}

func (m HookExecutionMode) valid() bool {
	return m == HookExecutionModeSync || m == HookExecutionModeAsync
}

// HookHandlerType is the exact closed public hook handler type.
type HookHandlerType string

const (
	HookHandlerTypeCommand HookHandlerType = "command"
	HookHandlerTypePrompt  HookHandlerType = "prompt"
	HookHandlerTypeAgent   HookHandlerType = "agent"
)

func (h HookHandlerType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(h, "hook handler type", HookHandlerType.valid)
}

func (h *HookHandlerType) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, h, "hook handler type", HookHandlerType.valid)
}

func (h HookHandlerType) valid() bool {
	return h == HookHandlerTypeCommand || h == HookHandlerTypePrompt || h == HookHandlerTypeAgent
}

// HookOutputEntryKind is the exact closed public hook output category.
type HookOutputEntryKind string

const (
	HookOutputEntryKindWarning  HookOutputEntryKind = "warning"
	HookOutputEntryKindStop     HookOutputEntryKind = "stop"
	HookOutputEntryKindFeedback HookOutputEntryKind = "feedback"
	HookOutputEntryKindContext  HookOutputEntryKind = "context"
	HookOutputEntryKindError    HookOutputEntryKind = "error"
)

func (k HookOutputEntryKind) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(k, "hook output entry kind", HookOutputEntryKind.valid)
}

func (k *HookOutputEntryKind) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, k, "hook output entry kind", HookOutputEntryKind.valid)
}

func (k HookOutputEntryKind) valid() bool {
	switch k {
	case HookOutputEntryKindWarning, HookOutputEntryKindStop, HookOutputEntryKindFeedback,
		HookOutputEntryKindContext, HookOutputEntryKindError:
		return true
	default:
		return false
	}
}

// HookRunStatus is the exact closed public hook run status.
type HookRunStatus string

const (
	HookRunStatusRunning   HookRunStatus = "running"
	HookRunStatusCompleted HookRunStatus = "completed"
	HookRunStatusFailed    HookRunStatus = "failed"
	HookRunStatusBlocked   HookRunStatus = "blocked"
	HookRunStatusStopped   HookRunStatus = "stopped"
)

func (s HookRunStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "hook run status", HookRunStatus.valid)
}

func (s *HookRunStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "hook run status", HookRunStatus.valid)
}

func (s HookRunStatus) valid() bool {
	switch s {
	case HookRunStatusRunning, HookRunStatusCompleted, HookRunStatusFailed,
		HookRunStatusBlocked, HookRunStatusStopped:
		return true
	default:
		return false
	}
}

// HookScope is the exact closed public hook run ownership scope.
type HookScope string

const (
	HookScopeThread HookScope = "thread"
	HookScopeTurn   HookScope = "turn"
)

func (s HookScope) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "hook scope", HookScope.valid)
}

func (s *HookScope) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "hook scope", HookScope.valid)
}

func (s HookScope) valid() bool {
	return s == HookScopeThread || s == HookScopeTurn
}

// HookSource is the exact closed public source classification for hook data.
// A value carries no path, trust, loading, or execution authority.
type HookSource string

const (
	HookSourceSystem                  HookSource = "system"
	HookSourceUser                    HookSource = "user"
	HookSourceProject                 HookSource = "project"
	HookSourceMDM                     HookSource = "mdm"
	HookSourceSessionFlags            HookSource = "sessionFlags"
	HookSourcePlugin                  HookSource = "plugin"
	HookSourceCloudRequirements       HookSource = "cloudRequirements"
	HookSourceCloudManagedConfig      HookSource = "cloudManagedConfig"
	HookSourceLegacyManagedConfigFile HookSource = "legacyManagedConfigFile"
	HookSourceLegacyManagedConfigMDM  HookSource = "legacyManagedConfigMdm"
	HookSourceUnknown                 HookSource = "unknown"
)

func (s HookSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "hook source", HookSource.valid)
}

func (s *HookSource) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "hook source", HookSource.valid)
}

func (s HookSource) valid() bool {
	switch s {
	case HookSourceSystem, HookSourceUser, HookSourceProject, HookSourceMDM,
		HookSourceSessionFlags, HookSourcePlugin, HookSourceCloudRequirements,
		HookSourceCloudManagedConfig, HookSourceLegacyManagedConfigFile,
		HookSourceLegacyManagedConfigMDM, HookSourceUnknown:
		return true
	default:
		return false
	}
}

// HookTrustStatus is the exact closed public hook trust classification. It is
// descriptive only and does not grant execution authority.
type HookTrustStatus string

const (
	HookTrustStatusManaged   HookTrustStatus = "managed"
	HookTrustStatusUntrusted HookTrustStatus = "untrusted"
	HookTrustStatusTrusted   HookTrustStatus = "trusted"
	HookTrustStatusModified  HookTrustStatus = "modified"
)

func (s HookTrustStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "hook trust status", HookTrustStatus.valid)
}

func (s *HookTrustStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "hook trust status", HookTrustStatus.valid)
}

func (s HookTrustStatus) valid() bool {
	switch s {
	case HookTrustStatusManaged, HookTrustStatusUntrusted,
		HookTrustStatusTrusted, HookTrustStatusModified:
		return true
	default:
		return false
	}
}

// HookOutputEntry is one exact standalone descriptive hook output value.
type HookOutputEntry struct {
	Kind HookOutputEntryKind `json:"kind"`
	Text string              `json:"text"`
}

func (e HookOutputEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Kind HookOutputEntryKind `json:"kind"`
		Text string              `json:"text"`
	}{Kind: e.Kind, Text: e.Text})
}

func (e *HookOutputEntry) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode hook output entry into nil receiver")
	}
	const objectName = "hook output entry"
	payload, err := decodeRustSerdeObject(data, objectName, "kind", "text")
	if err != nil {
		return err
	}
	kind, err := decodeRequiredThreadItemValue[HookOutputEntryKind](payload, objectName, "kind")
	if err != nil {
		return err
	}
	text, err := decodeRequiredThreadItemValue[string](payload, objectName, "text")
	if err != nil {
		return err
	}
	*e = HookOutputEntry{Kind: kind, Text: text}
	return nil
}

var (
	_ json.Marshaler   = HookEventName("")
	_ json.Unmarshaler = (*HookEventName)(nil)
	_ json.Marshaler   = HookExecutionMode("")
	_ json.Unmarshaler = (*HookExecutionMode)(nil)
	_ json.Marshaler   = HookHandlerType("")
	_ json.Unmarshaler = (*HookHandlerType)(nil)
	_ json.Marshaler   = HookOutputEntryKind("")
	_ json.Unmarshaler = (*HookOutputEntryKind)(nil)
	_ json.Marshaler   = HookRunStatus("")
	_ json.Unmarshaler = (*HookRunStatus)(nil)
	_ json.Marshaler   = HookScope("")
	_ json.Unmarshaler = (*HookScope)(nil)
	_ json.Marshaler   = HookSource("")
	_ json.Unmarshaler = (*HookSource)(nil)
	_ json.Marshaler   = HookTrustStatus("")
	_ json.Unmarshaler = (*HookTrustStatus)(nil)
	_ json.Marshaler   = HookOutputEntry{}
	_ json.Unmarshaler = (*HookOutputEntry)(nil)
)
