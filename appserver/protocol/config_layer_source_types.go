package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ConfigLayerSource is the exact public origin of a configuration layer. It is
// standalone from Gollem's live in-memory configuration service.
type ConfigLayerSource struct {
	raw json.RawMessage
}

func (s ConfigLayerSource) MarshalJSON() ([]byte, error) {
	if len(s.raw) == 0 {
		return nil, errors.New("config layer source is empty")
	}
	return validateConfigLayerSourceJSON(s.raw)
}

func (s *ConfigLayerSource) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode config layer source into nil receiver")
	}
	canonical, err := validateConfigLayerSourceJSON(data)
	if err != nil {
		return err
	}
	s.raw = canonical
	return nil
}

func (s ConfigLayerSource) Type() string {
	return permissionUnionDiscriminant(s.raw, "type")
}

func validateConfigLayerSourceJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(
		data,
		"config layer source",
		"type",
		"domain",
		"key",
		"file",
		"id",
		"name",
		"profile",
		"dotCodexFolder",
	)
	if err != nil {
		return nil, err
	}
	typeName, err := decodeRequiredThreadItemValue[string](payload, "config layer source", "type")
	if err != nil {
		return nil, err
	}
	switch typeName {
	case "mdm":
		if err := requirePermissionFields(payload, []string{"type", "domain", "key"}, "type", "domain", "key"); err != nil {
			return nil, err
		}
		domain, err := decodeRequiredThreadItemValue[string](payload, "MDM config layer source", "domain")
		if err != nil {
			return nil, err
		}
		key, err := decodeRequiredThreadItemValue[string](payload, "MDM config layer source", "key")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type   string `json:"type"`
			Domain string `json:"domain"`
			Key    string `json:"key"`
		}{Type: typeName, Domain: domain, Key: key})
	case "system":
		return canonicalConfigLayerFileSource(payload, typeName, "system config layer source")
	case "enterpriseManaged":
		if err := requirePermissionFields(payload, []string{"type", "id", "name"}, "type", "id", "name"); err != nil {
			return nil, err
		}
		id, err := decodeRequiredThreadItemValue[string](payload, "enterprise-managed config layer source", "id")
		if err != nil {
			return nil, err
		}
		name, err := decodeRequiredThreadItemValue[string](payload, "enterprise-managed config layer source", "name")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}{Type: typeName, ID: id, Name: name})
	case "user":
		if err := requirePermissionFields(payload, []string{"type", "file", "profile"}, "type", "file"); err != nil {
			return nil, err
		}
		file, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, "user config layer source", "file")
		if err != nil {
			return nil, err
		}
		profile, err := decodeOptionalNullableConfigRequirementValue[string](
			payload,
			"user config layer source",
			"profile",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string          `json:"type"`
			File    AbsolutePathBuf `json:"file"`
			Profile *string         `json:"profile"`
		}{Type: typeName, File: file, Profile: profile})
	case "project":
		if err := requirePermissionFields(payload, []string{"type", "dotCodexFolder"}, "type", "dotCodexFolder"); err != nil {
			return nil, err
		}
		folder, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, "project config layer source", "dotCodexFolder")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type           string          `json:"type"`
			DotCodexFolder AbsolutePathBuf `json:"dotCodexFolder"`
		}{Type: typeName, DotCodexFolder: folder})
	case "sessionFlags", "legacyManagedConfigTomlFromMdm":
		if err := requirePermissionFields(payload, []string{"type"}, "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: typeName})
	case "legacyManagedConfigTomlFromFile":
		return canonicalConfigLayerFileSource(payload, typeName, "legacy managed-file config layer source")
	default:
		return nil, fmt.Errorf("unsupported config layer source type %q", typeName)
	}
}

func canonicalConfigLayerFileSource(
	payload map[string]json.RawMessage,
	typeName string,
	objectName string,
) (json.RawMessage, error) {
	if err := requirePermissionFields(payload, []string{"type", "file"}, "type", "file"); err != nil {
		return nil, err
	}
	file, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "file")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type string          `json:"type"`
		File AbsolutePathBuf `json:"file"`
	}{Type: typeName, File: file})
}

var (
	_ json.Marshaler   = ConfigLayerSource{}
	_ json.Unmarshaler = (*ConfigLayerSource)(nil)
)
