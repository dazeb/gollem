package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// McpServerInfo is the exact public presentation metadata advertised by an
// initialized MCP server. It remains standalone until status projection exists.
type McpServerInfo struct {
	Name        string      `json:"name"`
	Title       *string     `json:"title"`
	Version     string      `json:"version"`
	Description *string     `json:"description"`
	Icons       []JsonValue `json:"icons"`
	WebsiteURL  *string     `json:"websiteUrl"`
}

func (i *McpServerInfo) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode MCP server info into nil receiver")
	}
	payload, err := decodeMcpServerInfoObject(data)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, "MCP server info", "name")
	if err != nil {
		return err
	}
	version, err := decodeRequiredThreadItemValue[string](payload, "MCP server info", "version")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableMcpServerInfoValue[string](payload, "title")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableMcpServerInfoValue[string](payload, "description")
	if err != nil {
		return err
	}
	iconsValue, err := decodeOptionalNullableMcpServerInfoValue[[]JsonValue](payload, "icons")
	if err != nil {
		return err
	}
	var icons []JsonValue
	if iconsValue != nil {
		icons = *iconsValue
	}
	websiteURL, err := decodeOptionalNullableMcpServerInfoValue[string](payload, "websiteUrl")
	if err != nil {
		return err
	}
	*i = McpServerInfo{
		Name:        name,
		Title:       title,
		Version:     version,
		Description: description,
		Icons:       icons,
		WebsiteURL:  websiteURL,
	}
	return nil
}

func decodeMcpServerInfoObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "MCP server info"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	known := map[string]struct{}{
		"name": {}, "title": {}, "version": {}, "description": {}, "icons": {}, "websiteUrl": {},
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

func decodeOptionalNullableMcpServerInfoValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode MCP server info %s: %w", fieldName, err)
	}
	return &value, nil
}

var _ json.Unmarshaler = (*McpServerInfo)(nil)
