package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// PluginsMigration identifies plugin names from one marketplace migration.
// It remains standalone until the external-agent migration contracts land.
type PluginsMigration struct {
	MarketplaceName string   `json:"marketplaceName"`
	PluginNames     []string `json:"pluginNames" jsonschema:"nonnullable=true"`
}

func (m PluginsMigration) MarshalJSON() ([]byte, error) {
	pluginNames := m.PluginNames
	if pluginNames == nil {
		pluginNames = []string{}
	}
	return json.Marshal(struct {
		MarketplaceName string   `json:"marketplaceName"`
		PluginNames     []string `json:"pluginNames"`
	}{
		MarketplaceName: m.MarketplaceName,
		PluginNames:     pluginNames,
	})
}

func (m *PluginsMigration) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode plugins migration into nil receiver")
	}
	payload, err := decodePluginsMigrationObject(data)
	if err != nil {
		return err
	}
	marketplaceName, err := decodeRequiredThreadItemValue[string](payload, "plugins migration", "marketplaceName")
	if err != nil {
		return err
	}
	pluginNames, err := decodeRequiredNonNullStringArray(payload, "plugins migration", "pluginNames")
	if err != nil {
		return err
	}
	*m = PluginsMigration{MarketplaceName: marketplaceName, PluginNames: pluginNames}
	return nil
}

func decodePluginsMigrationObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "plugins migration"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	payload := make(map[string]json.RawMessage, 2)
	seen := make(map[string]struct{}, 2)
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
		if name != "marketplaceName" && name != "pluginNames" {
			continue
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate %s field %q", objectName, name)
		}
		seen[name] = struct{}{}
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

var (
	_ json.Marshaler   = PluginsMigration{}
	_ json.Unmarshaler = (*PluginsMigration)(nil)
)
