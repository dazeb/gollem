package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// TurnEnvironmentParams is the exact standalone experimental public
// environment selection record. Gollem does not bind it to turn execution or
// interpret its environment identifiers and roots.
type TurnEnvironmentParams struct {
	EnvironmentID         string                 `json:"environmentId"`
	CWD                   LegacyAppPathString    `json:"cwd"`
	RuntimeWorkspaceRoots *[]LegacyAppPathString `json:"runtimeWorkspaceRoots"`
}

func (p TurnEnvironmentParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		EnvironmentID         string                 `json:"environmentId"`
		CWD                   LegacyAppPathString    `json:"cwd"`
		RuntimeWorkspaceRoots *[]LegacyAppPathString `json:"runtimeWorkspaceRoots"`
	}{
		EnvironmentID:         p.EnvironmentID,
		CWD:                   p.CWD,
		RuntimeWorkspaceRoots: p.RuntimeWorkspaceRoots,
	})
}

func (p *TurnEnvironmentParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode turn-environment params into nil receiver")
	}
	const objectName = "turn-environment params"
	payload, err := decodeRustSerdeObject(data, objectName, "environmentId", "cwd", "runtimeWorkspaceRoots")
	if err != nil {
		return err
	}
	environmentID, err := decodeRequiredThreadItemValue[string](payload, objectName, "environmentId")
	if err != nil {
		return err
	}
	cwd, err := decodeRequiredThreadItemValue[LegacyAppPathString](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	runtimeWorkspaceRoots, err := decodeOptionalNullableTurnEnvironmentArray[LegacyAppPathString](
		payload, objectName, "runtimeWorkspaceRoots",
	)
	if err != nil {
		return err
	}
	*p = TurnEnvironmentParams{
		EnvironmentID:         environmentID,
		CWD:                   cwd,
		RuntimeWorkspaceRoots: runtimeWorkspaceRoots,
	}
	return nil
}

func decodeOptionalNullableTurnEnvironmentArray[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}

func turnEnvironmentParamsSchema() Schema {
	return Schema{
		"properties": Schema{
			"cwd":           Schema{"$ref": "#/$defs/LegacyAppPathString"},
			"environmentId": Schema{"type": "string"},
			"runtimeWorkspaceRoots": Schema{
				"description": "Environment-native runtime workspace roots. Omitted defaults to `cwd`.",
				"items":       Schema{"$ref": "#/$defs/LegacyAppPathString"},
				"type":        []string{"array", "null"},
			},
		},
		"required": []string{"cwd", "environmentId"},
		"type":     "object",
	}
}

var (
	_ json.Marshaler   = TurnEnvironmentParams{}
	_ json.Unmarshaler = (*TurnEnvironmentParams)(nil)
)
