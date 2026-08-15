package protocol

import (
	"encoding/json"
	"errors"
)

// PluginShareListParams is the exact standalone source contract for
// plugin/share/list. It intentionally does not enumerate shared plugins.
type PluginShareListParams struct{}

func (p *PluginShareListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin share list params into nil receiver")
	}
	if _, err := decodeRustSerdeObject(data, "plugin share list params"); err != nil {
		return err
	}
	*p = PluginShareListParams{}
	return nil
}

func pluginShareListParamSchema() Schema {
	return Schema{"type": "object"}
}

var _ json.Unmarshaler = (*PluginShareListParams)(nil)
