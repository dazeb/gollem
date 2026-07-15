package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ExternalAgentConfigMigrationItem describes one standalone migration category.
// Its cwd and details fields are descriptive data, not filesystem authority.
type ExternalAgentConfigMigrationItem struct {
	ItemType    ExternalAgentConfigMigrationItemType `json:"itemType"`
	Description string                               `json:"description"`
	CWD         *string                              `json:"cwd"`
	Details     *MigrationDetails                    `json:"details"`
}

func (i ExternalAgentConfigMigrationItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ItemType    ExternalAgentConfigMigrationItemType `json:"itemType"`
		Description string                               `json:"description"`
		CWD         *string                              `json:"cwd"`
		Details     *MigrationDetails                    `json:"details"`
	}{
		ItemType: i.ItemType, Description: i.Description, CWD: i.CWD, Details: i.Details,
	})
}

func (i *ExternalAgentConfigMigrationItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode external-agent config migration item into nil receiver")
	}
	const objectName = "external-agent config migration item"
	payload, err := decodeExternalAgentConfigMigrationItemObject(data)
	if err != nil {
		return err
	}
	itemType, err := decodeRequiredThreadItemValue[ExternalAgentConfigMigrationItemType](payload, objectName, "itemType")
	if err != nil {
		return err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	details, err := decodeOptionalNullableConfigValue[MigrationDetails](payload, objectName, "details")
	if err != nil {
		return err
	}
	*i = ExternalAgentConfigMigrationItem{
		ItemType: itemType, Description: description, CWD: cwd, Details: details,
	}
	return nil
}

func decodeExternalAgentConfigMigrationItemObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "external-agent config migration item"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	known := map[string]struct{}{
		"itemType": {}, "description": {}, "cwd": {}, "details": {},
	}
	payload := make(map[string]json.RawMessage, len(known))
	seen := make(map[string]bool, len(known))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode %s field name: %w", objectName, err)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode %s field %q: %w", objectName, name, err)
		}
		if _, ok := known[name]; !ok {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate %s field %q", objectName, name)
		}
		seen[name] = true
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%s must contain one JSON value", objectName)
		}
		return nil, fmt.Errorf("decode %s trailing value: %w", objectName, err)
	}
	return payload, nil
}

func externalAgentConfigMigrationItemSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"itemType":    Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItemType"},
		"description": Schema{"type": "string"},
		"cwd": Schema{"anyOf": []any{
			Schema{"type": "string"}, Schema{"type": "null"},
		}},
		"details": Schema{"anyOf": []any{
			Schema{"$ref": "#/$defs/MigrationDetails"}, Schema{"type": "null"},
		}},
	}, []string{"itemType", "description"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigMigrationItem{}
	_ json.Unmarshaler = (*ExternalAgentConfigMigrationItem)(nil)
)
