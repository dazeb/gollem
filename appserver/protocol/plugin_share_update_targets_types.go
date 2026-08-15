package protocol

import (
	"encoding/json"
	"errors"
)

// PluginShareUpdateDiscoverability is the exact closed visibility choice for
// updating an existing shared plugin.
type PluginShareUpdateDiscoverability string

const (
	PluginShareUpdateDiscoverabilityUnlisted PluginShareUpdateDiscoverability = "UNLISTED"
	PluginShareUpdateDiscoverabilityPrivate  PluginShareUpdateDiscoverability = "PRIVATE"
	PluginShareUpdateDiscoverabilityListed   PluginShareUpdateDiscoverability = "LISTED"
)

func (d PluginShareUpdateDiscoverability) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(d, "plugin-share update discoverability", PluginShareUpdateDiscoverability.valid)
}

func (d *PluginShareUpdateDiscoverability) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, d, "plugin-share update discoverability", PluginShareUpdateDiscoverability.valid)
}

func (d PluginShareUpdateDiscoverability) valid() bool {
	switch d {
	case PluginShareUpdateDiscoverabilityUnlisted, PluginShareUpdateDiscoverabilityPrivate, PluginShareUpdateDiscoverabilityListed:
		return true
	default:
		return false
	}
}

// PluginShareUpdateTargetsParams is the exact standalone source contract for
// plugin/share/updateTargets. It intentionally does not mutate shared plugins.
type PluginShareUpdateTargetsParams struct {
	RemotePluginID  string                           `json:"remotePluginId"`
	Discoverability PluginShareUpdateDiscoverability `json:"discoverability"`
	ShareTargets    []PluginShareTarget              `json:"shareTargets"`
}

func (p PluginShareUpdateTargetsParams) MarshalJSON() ([]byte, error) {
	targets := p.ShareTargets
	if targets == nil {
		targets = []PluginShareTarget{}
	}
	return json.Marshal(struct {
		RemotePluginID  string                           `json:"remotePluginId"`
		Discoverability PluginShareUpdateDiscoverability `json:"discoverability"`
		ShareTargets    []PluginShareTarget              `json:"shareTargets"`
	}{RemotePluginID: p.RemotePluginID, Discoverability: p.Discoverability, ShareTargets: targets})
}

func (p *PluginShareUpdateTargetsParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin-share update-targets params into nil receiver")
	}
	const objectName = "plugin-share update-targets params"
	payload, err := decodeRustSerdeObject(data, objectName, "remotePluginId", "discoverability", "shareTargets")
	if err != nil {
		return err
	}
	remotePluginID, err := decodeRequiredThreadItemValue[string](payload, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	discoverability, err := decodeRequiredThreadItemValue[PluginShareUpdateDiscoverability](payload, objectName, "discoverability")
	if err != nil {
		return err
	}
	targets, err := decodeRequiredThreadItemArray[PluginShareTarget](payload, objectName, "shareTargets")
	if err != nil {
		return err
	}
	*p = PluginShareUpdateTargetsParams{RemotePluginID: remotePluginID, Discoverability: discoverability, ShareTargets: targets}
	return nil
}

func pluginShareUpdateDiscoverabilitySchema() Schema {
	return stringEnumSchema("UNLISTED", "PRIVATE", "LISTED")
}

func pluginShareUpdateTargetsParamsSchema() Schema {
	return Schema{"properties": Schema{
		"discoverability": Schema{"$ref": "#/$defs/PluginShareUpdateDiscoverability"},
		"remotePluginId":  Schema{"type": "string"},
		"shareTargets":    Schema{"items": Schema{"$ref": "#/$defs/PluginShareTarget"}, "type": "array"},
	}, "required": []string{"discoverability", "remotePluginId", "shareTargets"}, "type": "object"}
}

var (
	_ json.Marshaler   = PluginShareUpdateDiscoverability("")
	_ json.Unmarshaler = (*PluginShareUpdateDiscoverability)(nil)
	_ json.Marshaler   = PluginShareUpdateTargetsParams{}
	_ json.Unmarshaler = (*PluginShareUpdateTargetsParams)(nil)
)
