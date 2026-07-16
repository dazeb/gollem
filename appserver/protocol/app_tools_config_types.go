package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// AppToolConfig is exact standalone configuration for one app tool.
// App-tool policy evaluation and execution remain deferred.
type AppToolConfig struct {
	Enabled      *bool            `json:"enabled,omitempty"`
	ApprovalMode *AppToolApproval `json:"approval_mode,omitempty"`
}

func (c AppToolConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"enabled":       c.Enabled,
		"approval_mode": c.ApprovalMode,
	})
}

func (c *AppToolConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode app tool config into nil receiver")
	}
	const objectName = "app tool config"
	payload, err := decodeRustSerdeObject(data, objectName, "enabled", "approval_mode")
	if err != nil {
		return err
	}
	enabled, err := decodeOptionalNullableConfigValue[bool](payload, objectName, "enabled")
	if err != nil {
		return err
	}
	approvalMode, err := decodeOptionalNullableConfigValue[AppToolApproval](
		payload,
		objectName,
		"approval_mode",
	)
	if err != nil {
		return err
	}
	*c = AppToolConfig{Enabled: enabled, ApprovalMode: approvalMode}
	return nil
}

// AppToolsConfig is the flattened per-tool map for one app.
type AppToolsConfig struct {
	Tools map[string]AppToolConfig `json:"-"`
}

func (c AppToolsConfig) MarshalJSON() ([]byte, error) {
	tools := c.Tools
	if tools == nil {
		tools = map[string]AppToolConfig{}
	}
	return json.Marshal(tools)
}

func (c *AppToolsConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode app tools config into nil receiver")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("decode app tools config: %w", err)
	}
	if opening != json.Delim('{') {
		return errors.New("app tools config must be an object")
	}

	tools := make(map[string]AppToolConfig)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("decode app tools config tool name: %w", err)
		}
		name := token.(string)
		var config AppToolConfig
		if err := decoder.Decode(&config); err != nil {
			return fmt.Errorf("decode app tools config tool %q: %w", name, err)
		}
		// Serde's flattened HashMap also lets a later duplicate replace the earlier entry.
		tools[name] = config
	}
	if _, err := decoder.Token(); err != nil {
		return fmt.Errorf("decode app tools config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("app tools config must contain one JSON value")
		}
		return fmt.Errorf("decode app tools config trailing value: %w", err)
	}
	*c = AppToolsConfig{Tools: tools}
	return nil
}

var (
	_ json.Marshaler   = AppToolConfig{}
	_ json.Unmarshaler = (*AppToolConfig)(nil)
	_ json.Marshaler   = AppToolsConfig{}
	_ json.Unmarshaler = (*AppToolsConfig)(nil)
)
