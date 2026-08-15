package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// PluginListMarketplaceKind is the exact closed source filter for plugin/list.
type PluginListMarketplaceKind string

const (
	PluginListMarketplaceKindLocal              PluginListMarketplaceKind = "local"
	PluginListMarketplaceKindVertical           PluginListMarketplaceKind = "vertical"
	PluginListMarketplaceKindWorkspaceDirectory PluginListMarketplaceKind = "workspace-directory"
	PluginListMarketplaceKindSharedWithMe       PluginListMarketplaceKind = "shared-with-me"
	PluginListMarketplaceKindCreatedByMeRemote  PluginListMarketplaceKind = "created-by-me-remote"
)

func (k PluginListMarketplaceKind) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(k, "plugin-list marketplace kind", PluginListMarketplaceKind.valid)
}

func (k *PluginListMarketplaceKind) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, k, "plugin-list marketplace kind", PluginListMarketplaceKind.valid)
}

func (k PluginListMarketplaceKind) valid() bool {
	switch k {
	case PluginListMarketplaceKindLocal,
		PluginListMarketplaceKindVertical,
		PluginListMarketplaceKindWorkspaceDirectory,
		PluginListMarketplaceKindSharedWithMe,
		PluginListMarketplaceKindCreatedByMeRemote:
		return true
	default:
		return false
	}
}

// PluginListParams is the exact standalone source contract for plugin/list.
// It intentionally does not discover configured plugins or marketplaces.
type PluginListParams struct {
	CWDs             *[]AbsolutePathBuf           `json:"cwds"`
	MarketplaceKinds *[]PluginListMarketplaceKind `json:"marketplaceKinds"`
	ForceRefetch     bool                         `json:"forceRefetch,omitempty"`
}

func (p PluginListParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		CWDs             *[]AbsolutePathBuf           `json:"cwds"`
		MarketplaceKinds *[]PluginListMarketplaceKind `json:"marketplaceKinds"`
		ForceRefetch     bool                         `json:"forceRefetch,omitempty"`
	}
	return json.Marshal(wire(p))
}

func (p *PluginListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode plugin list params into nil receiver")
	}
	const objectName = "plugin list params"
	payload, err := decodeRustSerdeObject(data, objectName, "cwds", "marketplaceKinds", "forceRefetch")
	if err != nil {
		return err
	}
	cwds, err := decodeOptionalPluginListArray[AbsolutePathBuf](payload, objectName, "cwds")
	if err != nil {
		return err
	}
	marketplaceKinds, err := decodeOptionalPluginListArray[PluginListMarketplaceKind](payload, objectName, "marketplaceKinds")
	if err != nil {
		return err
	}
	forceRefetch, err := decodeOptionalConfigBool(payload, objectName, "forceRefetch")
	if err != nil {
		return err
	}
	*p = PluginListParams{
		CWDs: cwds, MarketplaceKinds: marketplaceKinds, ForceRefetch: forceRefetch,
	}
	return nil
}

func decodeOptionalPluginListArray[T any](payload map[string]json.RawMessage, objectName, fieldName string) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(entries))
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

func pluginListMarketplaceKindSchema() Schema {
	return stringEnumSchema(
		string(PluginListMarketplaceKindLocal),
		string(PluginListMarketplaceKindVertical),
		string(PluginListMarketplaceKindWorkspaceDirectory),
		string(PluginListMarketplaceKindSharedWithMe),
		string(PluginListMarketplaceKindCreatedByMeRemote),
	)
}

func pluginListParamSchema() Schema {
	return Schema{"properties": Schema{
		"cwds": Schema{
			"description": "Optional working directories used to discover repo marketplaces. When omitted, only home-scoped marketplaces and the official curated marketplace are considered.",
			"items":       Schema{"$ref": "#/$defs/AbsolutePathBuf"},
			"type":        []any{"array", "null"},
		},
		"marketplaceKinds": Schema{
			"description": "Optional marketplace kind filter. When omitted, only local marketplaces are queried, plus the default remote catalog when enabled by feature flag.",
			"items":       Schema{"$ref": "#/$defs/PluginListMarketplaceKind"},
			"type":        []any{"array", "null"},
		},
		"forceRefetch": Schema{
			"description": "Whether the client requests a fresh remote plugin catalog fetch.",
			"type":        "boolean",
		},
	}, "type": "object"}
}

var (
	_ json.Marshaler   = PluginListMarketplaceKind("")
	_ json.Unmarshaler = (*PluginListMarketplaceKind)(nil)
	_ json.Marshaler   = PluginListParams{}
	_ json.Unmarshaler = (*PluginListParams)(nil)
)
