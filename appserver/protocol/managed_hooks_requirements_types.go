package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ConfiguredHookHandler retains one exact public managed-hook handler without
// executing or binding it to Gollem runtime hook behavior.
type ConfiguredHookHandler struct {
	raw json.RawMessage
}

func (h ConfiguredHookHandler) MarshalJSON() ([]byte, error) {
	if len(h.raw) == 0 {
		return nil, errors.New("configured hook handler has no value")
	}
	return validateConfiguredHookHandlerJSON(h.raw)
}

func (h *ConfiguredHookHandler) UnmarshalJSON(data []byte) error {
	if h == nil {
		return errors.New("decode configured hook handler into nil receiver")
	}
	canonical, err := validateConfiguredHookHandlerJSON(data)
	if err != nil {
		return err
	}
	h.raw = canonical
	return nil
}

func validateConfiguredHookHandlerJSON(data []byte) (json.RawMessage, error) {
	const objectName = "configured hook handler"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"type",
		"command",
		"commandWindows",
		"timeoutSec",
		"async",
		"statusMessage",
	)
	if err != nil {
		return nil, err
	}
	hookType, err := decodeRequiredThreadItemValue[string](payload, objectName, "type")
	if err != nil {
		return nil, err
	}
	switch hookType {
	case "command":
		command, err := decodeRequiredThreadItemValue[string](payload, "command hook handler", "command")
		if err != nil {
			return nil, err
		}
		commandWindows, err := decodeOptionalNullableConfigRequirementValue[string](
			payload, "command hook handler", "commandWindows",
		)
		if err != nil {
			return nil, err
		}
		timeoutSec, err := decodeOptionalNullableConfigRequirementValue[uint64](
			payload, "command hook handler", "timeoutSec",
		)
		if err != nil {
			return nil, err
		}
		async, err := decodeRequiredThreadItemValue[bool](payload, "command hook handler", "async")
		if err != nil {
			return nil, err
		}
		statusMessage, err := decodeOptionalNullableConfigRequirementValue[string](
			payload, "command hook handler", "statusMessage",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type           string  `json:"type"`
			Command        string  `json:"command"`
			CommandWindows *string `json:"commandWindows"`
			TimeoutSec     *uint64 `json:"timeoutSec"`
			Async          bool    `json:"async"`
			StatusMessage  *string `json:"statusMessage"`
		}{
			Type: hookType, Command: command, CommandWindows: commandWindows,
			TimeoutSec: timeoutSec, Async: async, StatusMessage: statusMessage,
		})
	case "prompt", "agent":
		if err := rejectThreadItemFields(payload, hookType+" hook handler", "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: hookType})
	default:
		return nil, fmt.Errorf("unknown configured hook handler type %q", hookType)
	}
}

// ConfiguredHookMatcherGroup is the exact public matcher and non-null handler
// list used by managed hook requirements.
type ConfiguredHookMatcherGroup struct {
	Matcher *string                 `json:"matcher"`
	Hooks   []ConfiguredHookHandler `json:"hooks" jsonschema:"nonnullable=true"`
}

func (g ConfiguredHookMatcherGroup) MarshalJSON() ([]byte, error) {
	if g.Hooks == nil {
		return nil, errors.New("configured hook matcher group hooks cannot be null")
	}
	type wire ConfiguredHookMatcherGroup
	return json.Marshal(wire(g))
}

func (g *ConfiguredHookMatcherGroup) UnmarshalJSON(data []byte) error {
	if g == nil {
		return errors.New("decode configured hook matcher group into nil receiver")
	}
	const objectName = "configured hook matcher group"
	payload, err := decodeExactThreadItemObject(data, objectName, "matcher", "hooks")
	if err != nil {
		return err
	}
	matcher, err := decodeOptionalNullableConfigRequirementValue[string](payload, objectName, "matcher")
	if err != nil {
		return err
	}
	hooks, err := decodeRequiredManagedHookArray[ConfiguredHookHandler](payload, objectName, "hooks")
	if err != nil {
		return err
	}
	*g = ConfiguredHookMatcherGroup{Matcher: matcher, Hooks: hooks}
	return nil
}

// ManagedHooksRequirements is the exact standalone public managed-hook
// requirements value. It does not imply runtime hook execution or enforcement.
type ManagedHooksRequirements struct {
	ManagedDir        *string                      `json:"managedDir"`
	WindowsManagedDir *string                      `json:"windowsManagedDir"`
	PreToolUse        []ConfiguredHookMatcherGroup `json:"PreToolUse" jsonschema:"nonnullable=true"`
	PermissionRequest []ConfiguredHookMatcherGroup `json:"PermissionRequest" jsonschema:"nonnullable=true"`
	PostToolUse       []ConfiguredHookMatcherGroup `json:"PostToolUse" jsonschema:"nonnullable=true"`
	PreCompact        []ConfiguredHookMatcherGroup `json:"PreCompact" jsonschema:"nonnullable=true"`
	PostCompact       []ConfiguredHookMatcherGroup `json:"PostCompact" jsonschema:"nonnullable=true"`
	SessionStart      []ConfiguredHookMatcherGroup `json:"SessionStart" jsonschema:"nonnullable=true"`
	UserPromptSubmit  []ConfiguredHookMatcherGroup `json:"UserPromptSubmit" jsonschema:"nonnullable=true"`
	SubagentStart     []ConfiguredHookMatcherGroup `json:"SubagentStart" jsonschema:"nonnullable=true"`
	SubagentStop      []ConfiguredHookMatcherGroup `json:"SubagentStop" jsonschema:"nonnullable=true"`
	Stop              []ConfiguredHookMatcherGroup `json:"Stop" jsonschema:"nonnullable=true"`
}

func (r ManagedHooksRequirements) MarshalJSON() ([]byte, error) {
	arrays := []struct {
		name   string
		values []ConfiguredHookMatcherGroup
	}{
		{name: "PreToolUse", values: r.PreToolUse},
		{name: "PermissionRequest", values: r.PermissionRequest},
		{name: "PostToolUse", values: r.PostToolUse},
		{name: "PreCompact", values: r.PreCompact},
		{name: "PostCompact", values: r.PostCompact},
		{name: "SessionStart", values: r.SessionStart},
		{name: "UserPromptSubmit", values: r.UserPromptSubmit},
		{name: "SubagentStart", values: r.SubagentStart},
		{name: "SubagentStop", values: r.SubagentStop},
		{name: "Stop", values: r.Stop},
	}
	for _, array := range arrays {
		if array.values == nil {
			return nil, fmt.Errorf("managed hook requirements %s cannot be null", array.name)
		}
	}
	type wire ManagedHooksRequirements
	return json.Marshal(wire(r))
}

func (r *ManagedHooksRequirements) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode managed hook requirements into nil receiver")
	}
	const objectName = "managed hook requirements"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"managedDir",
		"windowsManagedDir",
		"PreToolUse",
		"PermissionRequest",
		"PostToolUse",
		"PreCompact",
		"PostCompact",
		"SessionStart",
		"UserPromptSubmit",
		"SubagentStart",
		"SubagentStop",
		"Stop",
	)
	if err != nil {
		return err
	}
	managedDir, err := decodeOptionalNullableConfigRequirementValue[string](payload, objectName, "managedDir")
	if err != nil {
		return err
	}
	windowsManagedDir, err := decodeOptionalNullableConfigRequirementValue[string](
		payload, objectName, "windowsManagedDir",
	)
	if err != nil {
		return err
	}
	preToolUse, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "PreToolUse")
	if err != nil {
		return err
	}
	permissionRequest, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "PermissionRequest")
	if err != nil {
		return err
	}
	postToolUse, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "PostToolUse")
	if err != nil {
		return err
	}
	preCompact, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "PreCompact")
	if err != nil {
		return err
	}
	postCompact, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "PostCompact")
	if err != nil {
		return err
	}
	sessionStart, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "SessionStart")
	if err != nil {
		return err
	}
	userPromptSubmit, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "UserPromptSubmit")
	if err != nil {
		return err
	}
	subagentStart, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "SubagentStart")
	if err != nil {
		return err
	}
	subagentStop, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "SubagentStop")
	if err != nil {
		return err
	}
	stop, err := decodeRequiredManagedHookArray[ConfiguredHookMatcherGroup](payload, objectName, "Stop")
	if err != nil {
		return err
	}
	*r = ManagedHooksRequirements{
		ManagedDir: managedDir, WindowsManagedDir: windowsManagedDir,
		PreToolUse: preToolUse, PermissionRequest: permissionRequest,
		PostToolUse: postToolUse, PreCompact: preCompact, PostCompact: postCompact,
		SessionStart: sessionStart, UserPromptSubmit: userPromptSubmit,
		SubagentStart: subagentStart, SubagentStop: subagentStop, Stop: stop,
	}
	return nil
}

func decodeRequiredManagedHookArray[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) ([]T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s %s cannot be null", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return values, nil
}

var (
	_ json.Marshaler   = ConfiguredHookHandler{}
	_ json.Unmarshaler = (*ConfiguredHookHandler)(nil)
	_ json.Marshaler   = ConfiguredHookMatcherGroup{}
	_ json.Unmarshaler = (*ConfiguredHookMatcherGroup)(nil)
	_ json.Marshaler   = ManagedHooksRequirements{}
	_ json.Unmarshaler = (*ManagedHooksRequirements)(nil)
)
