package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// CommandAction retains one validated public command-action variant without
// flattening its variant-specific path types into an invalid Go state.
type CommandAction struct {
	raw json.RawMessage
}

func (a CommandAction) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("command action has no value")
	}
	return validateCommandActionJSON(a.raw)
}

func (a *CommandAction) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode command action into nil receiver")
	}
	canonical, err := validateCommandActionJSON(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

func validateCommandActionJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(
		data,
		"command action",
		"type",
		"command",
		"name",
		"path",
		"query",
	)
	if err != nil {
		return nil, err
	}
	actionType, err := decodeRequiredThreadItemValue[string](payload, "command action", "type")
	if err != nil {
		return nil, err
	}
	command, err := decodeRequiredThreadItemValue[string](payload, "command action", "command")
	if err != nil {
		return nil, err
	}
	switch actionType {
	case "read":
		if err := rejectThreadItemFields(payload, "read command action", "type", "command", "name", "path"); err != nil {
			return nil, err
		}
		name, err := decodeRequiredThreadItemValue[string](payload, "read command action", "name")
		if err != nil {
			return nil, err
		}
		path, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, "read command action", "path")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string          `json:"type"`
			Command string          `json:"command"`
			Name    string          `json:"name"`
			Path    AbsolutePathBuf `json:"path"`
		}{Type: actionType, Command: command, Name: name, Path: path})
	case "listFiles":
		if err := rejectThreadItemFields(payload, "listFiles command action", "type", "command", "path"); err != nil {
			return nil, err
		}
		path, err := decodeRequiredNullableCommandActionString(payload, "listFiles command action", "path")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string  `json:"type"`
			Command string  `json:"command"`
			Path    *string `json:"path"`
		}{Type: actionType, Command: command, Path: path})
	case "search":
		if err := rejectThreadItemFields(payload, "search command action", "type", "command", "query", "path"); err != nil {
			return nil, err
		}
		query, err := decodeRequiredNullableCommandActionString(payload, "search command action", "query")
		if err != nil {
			return nil, err
		}
		path, err := decodeRequiredNullableCommandActionString(payload, "search command action", "path")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string  `json:"type"`
			Command string  `json:"command"`
			Query   *string `json:"query"`
			Path    *string `json:"path"`
		}{Type: actionType, Command: command, Query: query, Path: path})
	case "unknown":
		if err := rejectThreadItemFields(payload, "unknown command action", "type", "command"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}{Type: actionType, Command: command})
	default:
		return nil, fmt.Errorf("unknown command action type %q", actionType)
	}
}

func decodeRequiredNullableCommandActionString(payload map[string]json.RawMessage, objectName, fieldName string) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}
