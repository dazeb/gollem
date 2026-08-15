package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PluginInstallParams is the exact standalone source contract for plugin/install.
// It intentionally does not install configured plugins.
type PluginInstallParams struct {
	MarketplacePath       *AbsolutePathBuf `json:"marketplacePath"`
	RemoteMarketplaceName *string          `json:"remoteMarketplaceName"`
	InstallAttemptID      *string          `json:"installAttemptId"`
	PluginName            string           `json:"pluginName"`
}

func (p *PluginInstallParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin install params into nil receiver")
	}
	const objectName = "plugin install params"
	payload, err := decodeRustSerdeObject(data, objectName, "marketplacePath", "remoteMarketplaceName", "installAttemptId", "pluginName")
	if err != nil {
		return err
	}
	marketplacePath, err := decodeOptionalPluginInstallValue[AbsolutePathBuf](payload, objectName, "marketplacePath")
	if err != nil {
		return err
	}
	remoteMarketplaceName, err := decodeOptionalPluginInstallValue[string](payload, objectName, "remoteMarketplaceName")
	if err != nil {
		return err
	}
	installAttemptID, err := decodeOptionalPluginInstallValue[string](payload, objectName, "installAttemptId")
	if err != nil {
		return err
	}
	pluginName, err := decodeRequiredThreadItemValue[string](payload, objectName, "pluginName")
	if err != nil {
		return err
	}
	*p = PluginInstallParams{MarketplacePath: marketplacePath, RemoteMarketplaceName: remoteMarketplaceName, InstallAttemptID: installAttemptID, PluginName: pluginName}
	return nil
}

func decodeOptionalPluginInstallValue[T any](payload map[string]json.RawMessage, objectName, fieldName string) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func pluginInstallParamSchema() Schema {
	return Schema{"properties": Schema{
		"installAttemptId":      Schema{"description": "Client-generated identifier used to correlate one installation attempt.", "type": []any{"string", "null"}},
		"marketplacePath":       Schema{"anyOf": []any{Schema{"$ref": "#/$defs/AbsolutePathBuf"}, Schema{"type": "null"}}},
		"pluginName":            Schema{"type": "string"},
		"remoteMarketplaceName": Schema{"type": []any{"string", "null"}},
	}, "required": []string{"pluginName"}, "type": "object"}
}

var _ json.Unmarshaler = (*PluginInstallParams)(nil)
