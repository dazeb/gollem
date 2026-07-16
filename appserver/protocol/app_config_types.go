package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AppConfig is exact standalone configuration data for one app.
// Policy interpretation, persistence, and app-tool execution remain deferred.
type AppConfig struct {
	Enabled                  bool               `json:"enabled"`
	ApprovalsReviewer        *ApprovalsReviewer `json:"approvals_reviewer,omitempty"`
	DestructiveEnabled       *bool              `json:"destructive_enabled,omitempty"`
	OpenWorldEnabled         *bool              `json:"open_world_enabled,omitempty"`
	DefaultToolsApprovalMode *AppToolApproval   `json:"default_tools_approval_mode,omitempty"`
	DefaultToolsEnabled      *bool              `json:"default_tools_enabled,omitempty"`
	Tools                    *AppToolsConfig    `json:"tools,omitempty"`
}

func (c AppConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"enabled":                     c.Enabled,
		"approvals_reviewer":          c.ApprovalsReviewer,
		"destructive_enabled":         c.DestructiveEnabled,
		"open_world_enabled":          c.OpenWorldEnabled,
		"default_tools_approval_mode": c.DefaultToolsApprovalMode,
		"default_tools_enabled":       c.DefaultToolsEnabled,
		"tools":                       c.Tools,
	})
}

func (c *AppConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode app config into nil receiver")
	}
	const objectName = "app config"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"enabled",
		"approvals_reviewer",
		"destructive_enabled",
		"open_world_enabled",
		"default_tools_approval_mode",
		"default_tools_enabled",
		"tools",
	)
	if err != nil {
		return err
	}
	enabled, err := decodeDefaultEnabledAppConfigBool(payload)
	if err != nil {
		return err
	}
	approvalsReviewer, err := decodeOptionalNullableConfigValue[ApprovalsReviewer](
		payload, objectName, "approvals_reviewer",
	)
	if err != nil {
		return err
	}
	destructiveEnabled, err := decodeOptionalNullableConfigValue[bool](
		payload, objectName, "destructive_enabled",
	)
	if err != nil {
		return err
	}
	openWorldEnabled, err := decodeOptionalNullableConfigValue[bool](
		payload, objectName, "open_world_enabled",
	)
	if err != nil {
		return err
	}
	defaultToolsApprovalMode, err := decodeOptionalNullableConfigValue[AppToolApproval](
		payload, objectName, "default_tools_approval_mode",
	)
	if err != nil {
		return err
	}
	defaultToolsEnabled, err := decodeOptionalNullableConfigValue[bool](
		payload, objectName, "default_tools_enabled",
	)
	if err != nil {
		return err
	}
	tools, err := decodeOptionalNullableConfigValue[AppToolsConfig](payload, objectName, "tools")
	if err != nil {
		return err
	}
	*c = AppConfig{
		Enabled: enabled, ApprovalsReviewer: approvalsReviewer,
		DestructiveEnabled: destructiveEnabled, OpenWorldEnabled: openWorldEnabled,
		DefaultToolsApprovalMode: defaultToolsApprovalMode,
		DefaultToolsEnabled:      defaultToolsEnabled, Tools: tools,
	}
	return nil
}

func decodeDefaultEnabledAppConfigBool(payload map[string]json.RawMessage) (bool, error) {
	raw, ok := payload["enabled"]
	if !ok {
		return true, nil
	}
	if isJSONNull(raw) {
		return false, errors.New("decode app config enabled: value cannot be null")
	}
	var enabled bool
	if err := json.Unmarshal(raw, &enabled); err != nil {
		return false, fmt.Errorf("decode app config enabled: %w", err)
	}
	return enabled, nil
}

var (
	_ json.Marshaler   = AppConfig{}
	_ json.Unmarshaler = (*AppConfig)(nil)
)
