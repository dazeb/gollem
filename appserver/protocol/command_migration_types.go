package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// CommandMigration identifies one command configuration migration.
// It remains standalone until the external-agent migration contracts land.
type CommandMigration struct {
	Name string `json:"name"`
}

func (m *CommandMigration) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode command migration into nil receiver")
	}
	payload, err := decodeCommandMigrationObject(data)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, "command migration", "name")
	if err != nil {
		return err
	}
	*m = CommandMigration{Name: name}
	return nil
}

func decodeCommandMigrationObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "command migration"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	payload := make(map[string]json.RawMessage, 1)
	seenName := false
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
		if name != "name" {
			continue
		}
		if seenName {
			return nil, fmt.Errorf("duplicate %s field %q", objectName, name)
		}
		seenName = true
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

var _ json.Unmarshaler = (*CommandMigration)(nil)
