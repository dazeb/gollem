package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PluginShareDiscoverability is the exact closed visibility choice for a
// shared plugin.
type PluginShareDiscoverability string

const (
	PluginShareDiscoverabilityListed   PluginShareDiscoverability = "LISTED"
	PluginShareDiscoverabilityUnlisted PluginShareDiscoverability = "UNLISTED"
	PluginShareDiscoverabilityPrivate  PluginShareDiscoverability = "PRIVATE"
)

func (d PluginShareDiscoverability) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(d, "plugin-share discoverability", PluginShareDiscoverability.valid)
}

func (d *PluginShareDiscoverability) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, d, "plugin-share discoverability", PluginShareDiscoverability.valid)
}

func (d PluginShareDiscoverability) valid() bool {
	switch d {
	case PluginShareDiscoverabilityListed, PluginShareDiscoverabilityUnlisted, PluginShareDiscoverabilityPrivate:
		return true
	default:
		return false
	}
}

// PluginSharePrincipalType is the exact closed subject type of a share target.
type PluginSharePrincipalType string

const (
	PluginSharePrincipalTypeUser      PluginSharePrincipalType = "user"
	PluginSharePrincipalTypeGroup     PluginSharePrincipalType = "group"
	PluginSharePrincipalTypeWorkspace PluginSharePrincipalType = "workspace"
)

func (t PluginSharePrincipalType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(t, "plugin-share principal type", PluginSharePrincipalType.valid)
}

func (t *PluginSharePrincipalType) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, t, "plugin-share principal type", PluginSharePrincipalType.valid)
}

func (t PluginSharePrincipalType) valid() bool {
	switch t {
	case PluginSharePrincipalTypeUser, PluginSharePrincipalTypeGroup, PluginSharePrincipalTypeWorkspace:
		return true
	default:
		return false
	}
}

// PluginShareTargetRole is the exact closed access level for a share target.
type PluginShareTargetRole string

const (
	PluginShareTargetRoleReader PluginShareTargetRole = "reader"
	PluginShareTargetRoleEditor PluginShareTargetRole = "editor"
)

func (r PluginShareTargetRole) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "plugin-share target role", PluginShareTargetRole.valid)
}

func (r *PluginShareTargetRole) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "plugin-share target role", PluginShareTargetRole.valid)
}

func (r PluginShareTargetRole) valid() bool {
	return r == PluginShareTargetRoleReader || r == PluginShareTargetRoleEditor
}

// PluginShareTarget is the exact standalone principal and access-level pair.
type PluginShareTarget struct {
	PrincipalType PluginSharePrincipalType `json:"principalType"`
	PrincipalID   string                   `json:"principalId"`
	Role          PluginShareTargetRole    `json:"role"`
}

func (t PluginShareTarget) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PrincipalType PluginSharePrincipalType `json:"principalType"`
		PrincipalID   string                   `json:"principalId"`
		Role          PluginShareTargetRole    `json:"role"`
	}{PrincipalType: t.PrincipalType, PrincipalID: t.PrincipalID, Role: t.Role})
}

func (t *PluginShareTarget) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode plugin-share target into nil receiver")
	}
	const objectName = "plugin-share target"
	payload, err := decodeRustSerdeObject(data, objectName, "principalType", "principalId", "role")
	if err != nil {
		return err
	}
	principalType, err := decodeRequiredThreadItemValue[PluginSharePrincipalType](payload, objectName, "principalType")
	if err != nil {
		return err
	}
	principalID, err := decodeRequiredThreadItemValue[string](payload, objectName, "principalId")
	if err != nil {
		return err
	}
	role, err := decodeRequiredThreadItemValue[PluginShareTargetRole](payload, objectName, "role")
	if err != nil {
		return err
	}
	*t = PluginShareTarget{PrincipalType: principalType, PrincipalID: principalID, Role: role}
	return nil
}

// PluginShareSaveParams is the exact standalone source contract for
// plugin/share/save. It intentionally does not publish or mutate plugins.
type PluginShareSaveParams struct {
	PluginPath      AbsolutePathBuf             `json:"pluginPath"`
	RemotePluginID  *string                     `json:"remotePluginId"`
	Discoverability *PluginShareDiscoverability `json:"discoverability"`
	ShareTargets    *[]PluginShareTarget        `json:"shareTargets"`
}

func (p PluginShareSaveParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PluginPath      AbsolutePathBuf             `json:"pluginPath"`
		RemotePluginID  *string                     `json:"remotePluginId"`
		Discoverability *PluginShareDiscoverability `json:"discoverability"`
		ShareTargets    *[]PluginShareTarget        `json:"shareTargets"`
	}{
		PluginPath: p.PluginPath, RemotePluginID: p.RemotePluginID,
		Discoverability: p.Discoverability, ShareTargets: p.ShareTargets,
	})
}

func (p *PluginShareSaveParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin-share save params into nil receiver")
	}
	const objectName = "plugin-share save params"
	payload, err := decodeRustSerdeObject(data, objectName, "pluginPath", "remotePluginId", "discoverability", "shareTargets")
	if err != nil {
		return err
	}
	pluginPath, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "pluginPath")
	if err != nil {
		return err
	}
	remotePluginID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "remotePluginId")
	if err != nil {
		return err
	}
	discoverability, err := decodeOptionalNullableConfigValue[PluginShareDiscoverability](payload, objectName, "discoverability")
	if err != nil {
		return err
	}
	shareTargets, err := decodeOptionalPluginShareTargetArray(payload, objectName, "shareTargets")
	if err != nil {
		return err
	}
	*p = PluginShareSaveParams{
		PluginPath: pluginPath, RemotePluginID: remotePluginID,
		Discoverability: discoverability, ShareTargets: shareTargets,
	}
	return nil
}

func decodeOptionalPluginShareTargetArray(payload map[string]json.RawMessage, objectName, fieldName string) (*[]PluginShareTarget, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]PluginShareTarget, len(entries))
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

func pluginShareDiscoverabilitySchema() Schema {
	return stringEnumSchema("LISTED", "UNLISTED", "PRIVATE")
}

func pluginSharePrincipalTypeSchema() Schema {
	return stringEnumSchema("user", "group", "workspace")
}

func pluginShareTargetRoleSchema() Schema {
	return stringEnumSchema("reader", "editor")
}

func pluginShareTargetSchema() Schema {
	return Schema{"properties": Schema{
		"principalId":   Schema{"type": "string"},
		"principalType": Schema{"$ref": "#/$defs/PluginSharePrincipalType"},
		"role":          Schema{"$ref": "#/$defs/PluginShareTargetRole"},
	}, "required": []string{"principalId", "principalType", "role"}, "type": "object"}
}

func pluginShareSaveParamsSchema() Schema {
	return Schema{"properties": Schema{
		"discoverability": Schema{"anyOf": []any{Schema{"$ref": "#/$defs/PluginShareDiscoverability"}, Schema{"type": "null"}}},
		"pluginPath":      Schema{"$ref": "#/$defs/AbsolutePathBuf"},
		"remotePluginId":  Schema{"type": []any{"string", "null"}},
		"shareTargets": Schema{
			"items": Schema{"$ref": "#/$defs/PluginShareTarget"},
			"type":  []any{"array", "null"},
		},
	}, "required": []string{"pluginPath"}, "type": "object"}
}

var (
	_ json.Marshaler   = PluginShareDiscoverability("")
	_ json.Unmarshaler = (*PluginShareDiscoverability)(nil)
	_ json.Marshaler   = PluginSharePrincipalType("")
	_ json.Unmarshaler = (*PluginSharePrincipalType)(nil)
	_ json.Marshaler   = PluginShareTargetRole("")
	_ json.Unmarshaler = (*PluginShareTargetRole)(nil)
	_ json.Marshaler   = PluginShareTarget{}
	_ json.Unmarshaler = (*PluginShareTarget)(nil)
	_ json.Marshaler   = PluginShareSaveParams{}
	_ json.Unmarshaler = (*PluginShareSaveParams)(nil)
)
