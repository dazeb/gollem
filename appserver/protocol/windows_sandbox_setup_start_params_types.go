package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// WindowsSandboxSetupStartParams is the exact standalone source contract for
// windowsSandbox/setupStart. It intentionally does not bind Windows sandbox
// setup execution to Gollem.
type WindowsSandboxSetupStartParams struct {
	Mode WindowsSandboxSetupMode `json:"mode"`
	CWD  *AbsolutePathBuf        `json:"cwd"`
}

func (p *WindowsSandboxSetupStartParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode Windows sandbox setup start params into nil receiver")
	}
	const objectName = "Windows sandbox setup start params"
	payload, err := decodeRustSerdeObject(data, objectName, "mode", "cwd")
	if err != nil {
		return err
	}
	mode, err := decodeRequiredThreadItemValue[WindowsSandboxSetupMode](payload, objectName, "mode")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalWindowsSandboxSetupStartValue[AbsolutePathBuf](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	*p = WindowsSandboxSetupStartParams{Mode: mode, CWD: cwd}
	return nil
}

func decodeOptionalWindowsSandboxSetupStartValue[T any](
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

func windowsSandboxSetupStartParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"cwd": Schema{"anyOf": []any{
				Schema{"$ref": "#/$defs/AbsolutePathBuf"},
				Schema{"type": "null"},
			}},
			"mode": Schema{"$ref": "#/$defs/WindowsSandboxSetupMode"},
		},
		"required": []string{"mode"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*WindowsSandboxSetupStartParams)(nil)
