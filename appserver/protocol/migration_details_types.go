package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MigrationDetails groups the standalone external-agent migration leaves.
// It remains unbound until the external-agent request and runtime contracts land.
type MigrationDetails struct {
	Plugins    []PluginsMigration   `json:"plugins"`
	Skills     []SkillMigration     `json:"skills"`
	Sessions   []SessionMigration   `json:"sessions"`
	MCPServers []McpServerMigration `json:"mcpServers"`
	Hooks      []HookMigration      `json:"hooks"`
	Subagents  []SubagentMigration  `json:"subagents"`
	Commands   []CommandMigration   `json:"commands"`
}

func (d MigrationDetails) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Plugins    []PluginsMigration   `json:"plugins"`
		Skills     []SkillMigration     `json:"skills"`
		Sessions   []SessionMigration   `json:"sessions"`
		MCPServers []McpServerMigration `json:"mcpServers"`
		Hooks      []HookMigration      `json:"hooks"`
		Subagents  []SubagentMigration  `json:"subagents"`
		Commands   []CommandMigration   `json:"commands"`
	}{
		Plugins:    nonNilMigrationDetailsSlice(d.Plugins),
		Skills:     nonNilMigrationDetailsSlice(d.Skills),
		Sessions:   nonNilMigrationDetailsSlice(d.Sessions),
		MCPServers: nonNilMigrationDetailsSlice(d.MCPServers),
		Hooks:      nonNilMigrationDetailsSlice(d.Hooks),
		Subagents:  nonNilMigrationDetailsSlice(d.Subagents),
		Commands:   nonNilMigrationDetailsSlice(d.Commands),
	})
}

func (d *MigrationDetails) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("decode migration details into nil receiver")
	}
	payload, err := decodeMigrationDetailsObject(data)
	if err != nil {
		return err
	}
	plugins, err := decodeMigrationDetailsArray[PluginsMigration](payload, "plugins")
	if err != nil {
		return err
	}
	skills, err := decodeMigrationDetailsArray[SkillMigration](payload, "skills")
	if err != nil {
		return err
	}
	sessions, err := decodeMigrationDetailsArray[SessionMigration](payload, "sessions")
	if err != nil {
		return err
	}
	mcpServers, err := decodeMigrationDetailsArray[McpServerMigration](payload, "mcpServers")
	if err != nil {
		return err
	}
	hooks, err := decodeMigrationDetailsArray[HookMigration](payload, "hooks")
	if err != nil {
		return err
	}
	subagents, err := decodeMigrationDetailsArray[SubagentMigration](payload, "subagents")
	if err != nil {
		return err
	}
	commands, err := decodeMigrationDetailsArray[CommandMigration](payload, "commands")
	if err != nil {
		return err
	}
	*d = MigrationDetails{
		Plugins: plugins, Skills: skills, Sessions: sessions, MCPServers: mcpServers,
		Hooks: hooks, Subagents: subagents, Commands: commands,
	}
	return nil
}

func decodeMigrationDetailsObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "migration details"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	known := map[string]struct{}{
		"plugins": {}, "skills": {}, "sessions": {}, "mcpServers": {},
		"hooks": {}, "subagents": {}, "commands": {},
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

func decodeMigrationDetailsArray[T any](payload map[string]json.RawMessage, fieldName string) ([]T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return []T{}, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("migration details field %q must be a non-null array", fieldName)
	}
	var values []T
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode migration details field %q: %w", fieldName, err)
	}
	return nonNilMigrationDetailsSlice(values), nil
}

func nonNilMigrationDetailsSlice[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

var (
	_ json.Marshaler   = MigrationDetails{}
	_ json.Unmarshaler = (*MigrationDetails)(nil)
)
