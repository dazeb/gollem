package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// SessionMigration describes one external session file and its working path.
// It remains standalone until the external-agent migration contracts land.
type SessionMigration struct {
	Path  string  `json:"path"`
	CWD   string  `json:"cwd"`
	Title *string `json:"title"`
}

func (m *SessionMigration) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode session migration into nil receiver")
	}
	payload, err := decodeSessionMigrationObject(data)
	if err != nil {
		return err
	}
	path, err := decodeRequiredThreadItemValue[string](payload, "session migration", "path")
	if err != nil {
		return err
	}
	cwd, err := decodeRequiredThreadItemValue[string](payload, "session migration", "cwd")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableSessionMigrationTitle(payload)
	if err != nil {
		return err
	}
	*m = SessionMigration{Path: path, CWD: cwd, Title: title}
	return nil
}

func decodeSessionMigrationObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "session migration"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	known := map[string]struct{}{"path": {}, "cwd": {}, "title": {}}
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

func decodeOptionalNullableSessionMigrationTitle(payload map[string]json.RawMessage) (*string, error) {
	raw, ok := payload["title"]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var title string
	if err := json.Unmarshal(raw, &title); err != nil {
		return nil, fmt.Errorf("decode session migration title: %w", err)
	}
	return &title, nil
}

var _ json.Unmarshaler = (*SessionMigration)(nil)
