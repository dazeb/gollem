package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SkillsListParams is the exact standalone source contract for skills/list.
// It intentionally does not bind skill discovery or filesystem scanning.
type SkillsListParams struct {
	CWDs        []string `json:"cwds,omitempty"`
	ForceReload bool     `json:"forceReload,omitempty"`
}

func (p *SkillsListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode skills list params into nil receiver")
	}
	const objectName = "skills list params"
	payload, err := decodeRustSerdeObject(data, objectName, "cwds", "forceReload")
	if err != nil {
		return err
	}
	cwds, err := decodeOptionalSkillsListStrings(payload, objectName, "cwds")
	if err != nil {
		return err
	}
	forceReload, err := decodeOptionalSkillsListBool(payload, objectName, "forceReload")
	if err != nil {
		return err
	}
	*p = SkillsListParams{CWDs: cwds, ForceReload: forceReload}
	return nil
}

func decodeOptionalSkillsListStrings(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) ([]string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return []string{}, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("decode %s %s: value cannot be null", objectName, fieldName)
	}
	var value []string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func decodeOptionalSkillsListBool(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (bool, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return false, nil
	}
	if isJSONNull(raw) {
		return false, fmt.Errorf("decode %s %s: value cannot be null", objectName, fieldName)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func skillsListParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"cwds": Schema{
				"description": "When empty, defaults to the current session working directory.",
				"items":       Schema{"type": "string"},
				"type":        "array",
			},
			"forceReload": Schema{
				"description": "When true, bypass the skills cache and re-scan skills from disk.",
				"type":        "boolean",
			},
		},
		"type": "object",
	}
}

var _ json.Unmarshaler = (*SkillsListParams)(nil)
