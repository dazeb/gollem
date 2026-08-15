package protocol

import (
	"encoding/json"
	"errors"
)

// PluginShareDeleteParams is the exact standalone source contract for
// plugin/share/delete. It intentionally does not delete remote plugins.
type PluginShareDeleteParams struct {
	RemotePluginID string `json:"remotePluginId"`
}

func (p *PluginShareDeleteParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin share delete params into nil receiver")
	}
	const objectName = "plugin share delete params"
	payload, err := decodeRustSerdeObject(data, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	remotePluginID, err := decodeRequiredThreadItemValue[string](payload, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	*p = PluginShareDeleteParams{RemotePluginID: remotePluginID}
	return nil
}

func pluginShareDeleteParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"remotePluginId": Schema{"type": "string"},
		},
		"required": []string{"remotePluginId"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*PluginShareDeleteParams)(nil)
