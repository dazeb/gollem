package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SkillsConfigWriteParams is the exact standalone source contract for
// skills/config/write. It intentionally does not change Gollem configuration.
type SkillsConfigWriteParams struct {
	Path    *AbsolutePathBuf `json:"path"`
	Name    *string          `json:"name"`
	Enabled bool             `json:"enabled"`
}

func (p *SkillsConfigWriteParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode skills config write params into nil receiver")
	}
	const objectName = "skills config write params"
	payload, err := decodeRustSerdeObject(data, objectName, "path", "name", "enabled")
	if err != nil {
		return err
	}
	path, err := decodeOptionalSkillsConfigValue[AbsolutePathBuf](payload, objectName, "path")
	if err != nil {
		return err
	}
	name, err := decodeOptionalSkillsConfigValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	enabled, err := decodeRequiredThreadItemValue[bool](payload, objectName, "enabled")
	if err != nil {
		return err
	}
	*p = SkillsConfigWriteParams{Path: path, Name: name, Enabled: enabled}
	return nil
}

func decodeOptionalSkillsConfigValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func skillsConfigWriteParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"enabled": Schema{"type": "boolean"},
			"name": Schema{
				"description": "Name-based selector.",
				"type":        []any{"string", "null"},
			},
			"path": Schema{
				"anyOf": []any{
					Schema{"$ref": "#/$defs/AbsolutePathBuf"},
					Schema{"type": "null"},
				},
				"description": "Path-based selector.",
			},
		},
		"required": []string{"enabled"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*SkillsConfigWriteParams)(nil)
