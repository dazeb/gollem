package protocol

import (
	"encoding/json"
	"errors"
)

// PluginUninstallParams is the exact standalone source contract for
// plugin/uninstall. It intentionally does not uninstall configured plugins.
type PluginUninstallParams struct {
	PluginID string `json:"pluginId"`
}

func (p *PluginUninstallParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin uninstall params into nil receiver")
	}
	const objectName = "plugin uninstall params"
	payload, err := decodeRustSerdeObject(data, objectName, "pluginId")
	if err != nil {
		return err
	}
	pluginID, err := decodeRequiredThreadItemValue[string](payload, objectName, "pluginId")
	if err != nil {
		return err
	}
	*p = PluginUninstallParams{PluginID: pluginID}
	return nil
}

func pluginUninstallParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"pluginId": Schema{"type": "string"},
		},
		"required": []string{"pluginId"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*PluginUninstallParams)(nil)
