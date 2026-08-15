package protocol

import (
	"encoding/json"
	"errors"
)

// PluginSkillReadParams is the exact standalone source contract for
// plugin/skill/read. It intentionally does not read configured plugin skills.
type PluginSkillReadParams struct {
	RemoteMarketplaceName string `json:"remoteMarketplaceName"`
	RemotePluginID        string `json:"remotePluginId"`
	SkillName             string `json:"skillName"`
}

func (p *PluginSkillReadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin skill read params into nil receiver")
	}
	const objectName = "plugin skill read params"
	payload, err := decodeRustSerdeObject(data, objectName, "remoteMarketplaceName", "remotePluginId", "skillName")
	if err != nil {
		return err
	}
	remoteMarketplaceName, err := decodeRequiredThreadItemValue[string](payload, objectName, "remoteMarketplaceName")
	if err != nil {
		return err
	}
	remotePluginID, err := decodeRequiredThreadItemValue[string](payload, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	skillName, err := decodeRequiredThreadItemValue[string](payload, objectName, "skillName")
	if err != nil {
		return err
	}
	*p = PluginSkillReadParams{
		RemoteMarketplaceName: remoteMarketplaceName,
		RemotePluginID:        remotePluginID,
		SkillName:             skillName,
	}
	return nil
}

func pluginSkillReadParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"remoteMarketplaceName": Schema{"type": "string"},
			"remotePluginId":        Schema{"type": "string"},
			"skillName":             Schema{"type": "string"},
		},
		"required": []string{"remoteMarketplaceName", "remotePluginId", "skillName"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*PluginSkillReadParams)(nil)
