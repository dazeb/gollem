package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PluginInstalledParams is the exact standalone source contract for plugin/installed.
// It intentionally does not discover installed plugins.
type PluginInstalledParams struct {
	CWDs                         *[]AbsolutePathBuf `json:"cwds"`
	InstallSuggestionPluginNames *[]string          `json:"installSuggestionPluginNames"`
}

func (p *PluginInstalledParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin installed params into nil receiver")
	}
	const objectName = "plugin installed params"
	payload, err := decodeRustSerdeObject(data, objectName, "cwds", "installSuggestionPluginNames")
	if err != nil {
		return err
	}
	cwds, err := decodeOptionalPluginInstalledArray[AbsolutePathBuf](payload, objectName, "cwds")
	if err != nil {
		return err
	}
	names, err := decodeOptionalPluginInstalledArray[string](payload, objectName, "installSuggestionPluginNames")
	if err != nil {
		return err
	}
	*p = PluginInstalledParams{CWDs: cwds, InstallSuggestionPluginNames: names}
	return nil
}
func decodeOptionalPluginInstalledArray[T any](payload map[string]json.RawMessage, objectName, fieldName string) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(entries))
	for index, entry := range entries {
		if isJSONNull(entry) {
			return nil, fmt.Errorf("decode %s %s[%d]: cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(entry, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}
func pluginInstalledParamSchema() Schema {
	return Schema{"properties": Schema{
		"cwds":                         Schema{"description": "Optional working directories used to discover repo marketplaces.", "items": Schema{"$ref": "#/$defs/AbsolutePathBuf"}, "type": []any{"array", "null"}},
		"installSuggestionPluginNames": Schema{"description": "Additional uninstalled plugin names that should be returned when present locally. This is used by mention surfaces that intentionally expose install entrypoints.", "items": Schema{"type": "string"}, "type": []any{"array", "null"}},
	}, "type": "object"}
}

var _ json.Unmarshaler = (*PluginInstalledParams)(nil)
