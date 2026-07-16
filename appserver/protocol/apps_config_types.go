package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

// AppsDefaultConfig is exact standalone default policy data for apps.
// Default inheritance and policy evaluation remain deferred.
type AppsDefaultConfig struct {
	Enabled                  bool               `json:"enabled"`
	ApprovalsReviewer        *ApprovalsReviewer `json:"approvals_reviewer,omitempty"`
	DestructiveEnabled       bool               `json:"destructive_enabled"`
	OpenWorldEnabled         bool               `json:"open_world_enabled"`
	DefaultToolsApprovalMode *AppToolApproval   `json:"default_tools_approval_mode,omitempty"`
}

func (c AppsDefaultConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"enabled":                     c.Enabled,
		"approvals_reviewer":          c.ApprovalsReviewer,
		"destructive_enabled":         c.DestructiveEnabled,
		"open_world_enabled":          c.OpenWorldEnabled,
		"default_tools_approval_mode": c.DefaultToolsApprovalMode,
	})
}

func (c *AppsDefaultConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode apps default config into nil receiver")
	}
	const objectName = "apps default config"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"enabled",
		"approvals_reviewer",
		"destructive_enabled",
		"open_world_enabled",
		"default_tools_approval_mode",
	)
	if err != nil {
		return err
	}
	enabled, err := decodeDefaultTrueConfigBool(payload, objectName, "enabled")
	if err != nil {
		return err
	}
	approvalsReviewer, err := decodeOptionalNullableConfigValue[ApprovalsReviewer](
		payload, objectName, "approvals_reviewer",
	)
	if err != nil {
		return err
	}
	destructiveEnabled, err := decodeDefaultTrueConfigBool(
		payload, objectName, "destructive_enabled",
	)
	if err != nil {
		return err
	}
	openWorldEnabled, err := decodeDefaultTrueConfigBool(payload, objectName, "open_world_enabled")
	if err != nil {
		return err
	}
	defaultToolsApprovalMode, err := decodeOptionalNullableConfigValue[AppToolApproval](
		payload, objectName, "default_tools_approval_mode",
	)
	if err != nil {
		return err
	}
	*c = AppsDefaultConfig{
		Enabled:                  enabled,
		ApprovalsReviewer:        approvalsReviewer,
		DestructiveEnabled:       destructiveEnabled,
		OpenWorldEnabled:         openWorldEnabled,
		DefaultToolsApprovalMode: defaultToolsApprovalMode,
	}
	return nil
}

// AppsConfig is exact standalone default and per-app configuration data.
type AppsConfig struct {
	Default *AppsDefaultConfig   `json:"_default,omitempty"`
	Apps    map[string]AppConfig `json:"-"`
}

func (c AppsConfig) MarshalJSON() ([]byte, error) {
	defaultJSON, err := json.Marshal(c.Default)
	if err != nil {
		return nil, fmt.Errorf("encode apps config default: %w", err)
	}
	names := make([]string, 0, len(c.Apps))
	for name := range c.Apps {
		names = append(names, name)
	}
	sort.Strings(names)

	var encoded bytes.Buffer
	encoded.WriteString(`{"_default":`)
	encoded.Write(defaultJSON)
	for _, name := range names {
		nameJSON, _ := json.Marshal(name)
		appJSON, err := json.Marshal(c.Apps[name])
		if err != nil {
			return nil, fmt.Errorf("encode apps config app %q: %w", name, err)
		}
		encoded.WriteByte(',')
		encoded.Write(nameJSON)
		encoded.WriteByte(':')
		encoded.Write(appJSON)
	}
	encoded.WriteByte('}')
	return encoded.Bytes(), nil
}

func (c *AppsConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode apps config into nil receiver")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode apps config: %w", err)
	}
	if opening != json.Delim('{') {
		return errors.New("apps config must be an object")
	}

	var defaultConfig *AppsDefaultConfig
	seenDefault := false
	apps := make(map[string]AppConfig)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode apps config key: %w", err)
		}
		name := token.(string)
		if name == "_default" {
			if seenDefault {
				return errors.New("decode apps config: duplicate field \"_default\"")
			}
			seenDefault = true
			if err := decoder.Decode(&defaultConfig); err != nil {
				return fmt.Errorf("decode apps config default: %w", err)
			}
			continue
		}
		var app AppConfig
		if err := decoder.Decode(&app); err != nil {
			return fmt.Errorf("decode apps config app %q: %w", name, err)
		}
		// Serde's flattened HashMap lets a later duplicate replace an earlier app.
		apps[name] = app
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("decode apps config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("apps config must contain one JSON value")
		}
		return fmt.Errorf("decode apps config trailing value: %w", err)
	}
	*c = AppsConfig{Default: defaultConfig, Apps: apps}
	return nil
}

func decodeDefaultTrueConfigBool(
	payload map[string]json.RawMessage,
	objectName string,
	field string,
) (bool, error) {
	raw, ok := payload[field]
	if !ok {
		return true, nil
	}
	if isJSONNull(raw) {
		return false, fmt.Errorf("decode %s %s: value cannot be null", objectName, field)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("decode %s %s: %w", objectName, field, err)
	}
	return value, nil
}

var (
	_ json.Marshaler   = AppsDefaultConfig{}
	_ json.Unmarshaler = (*AppsDefaultConfig)(nil)
	_ json.Marshaler   = AppsConfig{}
	_ json.Unmarshaler = (*AppsConfig)(nil)
)
