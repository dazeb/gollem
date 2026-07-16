package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ParsedCommand retains one exact public parsed-command variant without
// aliasing Gollem's distinct live command-action types.
type ParsedCommand struct {
	raw json.RawMessage
}

func (c ParsedCommand) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("parsed command has no value")
	}
	return validateParsedCommandJSON(c.raw)
}

func (c *ParsedCommand) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode parsed command into nil receiver")
	}
	canonical, err := validateParsedCommandJSON(data)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

func validateParsedCommandJSON(data []byte) (json.RawMessage, error) {
	const objectName = "parsed command"
	discriminator, err := decodeRustSerdeObject(data, objectName, "type")
	if err != nil {
		return nil, err
	}
	commandType, err := decodeRequiredThreadItemValue[string](discriminator, objectName, "type")
	if err != nil {
		return nil, err
	}

	switch commandType {
	case "read":
		payload, err := decodeRustSerdeObject(data, "read parsed command", "type", "cmd", "name", "path")
		if err != nil {
			return nil, err
		}
		cmd, err := decodeRequiredThreadItemValue[string](payload, "read parsed command", "cmd")
		if err != nil {
			return nil, err
		}
		name, err := decodeRequiredThreadItemValue[string](payload, "read parsed command", "name")
		if err != nil {
			return nil, err
		}
		path, err := decodeRequiredThreadItemValue[string](payload, "read parsed command", "path")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Cmd  string `json:"cmd"`
			Name string `json:"name"`
			Path string `json:"path"`
		}{Type: commandType, Cmd: cmd, Name: name, Path: path})
	case "list_files":
		payload, err := decodeRustSerdeObject(data, "list_files parsed command", "type", "cmd", "path")
		if err != nil {
			return nil, err
		}
		cmd, err := decodeRequiredThreadItemValue[string](payload, "list_files parsed command", "cmd")
		if err != nil {
			return nil, err
		}
		path, err := decodeOptionalNullableParsedCommandString(payload, "list_files parsed command", "path")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string  `json:"type"`
			Cmd  string  `json:"cmd"`
			Path *string `json:"path"`
		}{Type: commandType, Cmd: cmd, Path: path})
	case "search":
		payload, err := decodeRustSerdeObject(data, "search parsed command", "type", "cmd", "query", "path")
		if err != nil {
			return nil, err
		}
		cmd, err := decodeRequiredThreadItemValue[string](payload, "search parsed command", "cmd")
		if err != nil {
			return nil, err
		}
		query, err := decodeOptionalNullableParsedCommandString(payload, "search parsed command", "query")
		if err != nil {
			return nil, err
		}
		path, err := decodeOptionalNullableParsedCommandString(payload, "search parsed command", "path")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type  string  `json:"type"`
			Cmd   string  `json:"cmd"`
			Query *string `json:"query"`
			Path  *string `json:"path"`
		}{Type: commandType, Cmd: cmd, Query: query, Path: path})
	case "unknown":
		payload, err := decodeRustSerdeObject(data, "unknown parsed command", "type", "cmd")
		if err != nil {
			return nil, err
		}
		cmd, err := decodeRequiredThreadItemValue[string](payload, "unknown parsed command", "cmd")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Cmd  string `json:"cmd"`
		}{Type: commandType, Cmd: cmd})
	default:
		return nil, fmt.Errorf("unknown parsed command type %q", commandType)
	}
}

func decodeOptionalNullableParsedCommandString(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

var _ json.Marshaler = ParsedCommand{}
var _ json.Unmarshaler = (*ParsedCommand)(nil)
