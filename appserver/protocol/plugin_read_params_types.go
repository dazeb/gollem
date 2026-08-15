package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PluginReadParams is the exact standalone source contract for plugin/read.
// It intentionally does not read configured plugins.
type PluginReadParams struct {
	MarketplacePath       *AbsolutePathBuf `json:"marketplacePath"`
	RemoteMarketplaceName *string          `json:"remoteMarketplaceName"`
	PluginName            string           `json:"pluginName"`
}

func (p *PluginReadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin read params into nil receiver")
	}
	const objectName = "plugin read params"
	payload, err := decodeRustSerdeObject(data, objectName, "marketplacePath", "remoteMarketplaceName", "pluginName")
	if err != nil {
		return err
	}
	marketplacePath, err := decodeOptionalPluginReadValue[AbsolutePathBuf](payload, objectName, "marketplacePath")
	if err != nil {
		return err
	}
	remoteMarketplaceName, err := decodeOptionalPluginReadValue[string](payload, objectName, "remoteMarketplaceName")
	if err != nil {
		return err
	}
	pluginName, err := decodeRequiredThreadItemValue[string](payload, objectName, "pluginName")
	if err != nil {
		return err
	}
	*p = PluginReadParams{MarketplacePath: marketplacePath, RemoteMarketplaceName: remoteMarketplaceName, PluginName: pluginName}
	return nil
}

func decodeOptionalPluginReadValue[T any](payload map[string]json.RawMessage, objectName, fieldName string) (*T, error) {
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

func pluginReadParamSchema() Schema {
	return Schema{"properties": Schema{
		"marketplacePath":       Schema{"anyOf": []any{Schema{"$ref": "#/$defs/AbsolutePathBuf"}, Schema{"type": "null"}}},
		"pluginName":            Schema{"type": "string"},
		"remoteMarketplaceName": Schema{"type": []any{"string", "null"}},
	}, "required": []string{"pluginName"}, "type": "object"}
}

var _ json.Unmarshaler = (*PluginReadParams)(nil)
