package protocol

import (
	"encoding/json"
	"errors"
)

// PluginShareCheckoutParams is the exact standalone source contract for
// plugin/share/checkout. It intentionally does not check out remote plugins.
type PluginShareCheckoutParams struct {
	RemotePluginID string `json:"remotePluginId"`
}

func (p *PluginShareCheckoutParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin share checkout params into nil receiver")
	}
	const objectName = "plugin share checkout params"
	payload, err := decodeRustSerdeObject(data, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	remotePluginID, err := decodeRequiredThreadItemValue[string](payload, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	*p = PluginShareCheckoutParams{RemotePluginID: remotePluginID}
	return nil
}

func pluginShareCheckoutParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"remotePluginId": Schema{"type": "string"},
		},
		"required": []string{"remotePluginId"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*PluginShareCheckoutParams)(nil)
