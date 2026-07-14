package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// McpResourceReadParams is the exact public MCP resource selector. It remains
// separate from the alias-compatible live request until an adapter is defined.
type McpResourceReadParams struct {
	ThreadID *string `json:"threadId,omitempty"`
	Server   string  `json:"server"`
	URI      string  `json:"uri"`
}

func (p McpResourceReadParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		ThreadID *string `json:"threadId"`
		Server   string  `json:"server"`
		URI      string  `json:"uri"`
	}
	return json.Marshal(wire(p))
}

func (p *McpResourceReadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode MCP resource-read params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"MCP resource-read params",
		"threadId",
		"server",
		"uri",
	)
	if err != nil {
		return err
	}
	server, err := decodeRequiredThreadItemValue[string](
		payload,
		"MCP resource-read params",
		"server",
	)
	if err != nil {
		return err
	}
	uri, err := decodeRequiredThreadItemValue[string](
		payload,
		"MCP resource-read params",
		"uri",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableMcpResourceReadValue[string](payload, "threadId")
	if err != nil {
		return err
	}
	*p = McpResourceReadParams{ThreadID: threadID, Server: server, URI: uri}
	return nil
}

func decodeOptionalNullableMcpResourceReadValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode MCP resource-read %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = McpResourceReadParams{}
	_ json.Unmarshaler = (*McpResourceReadParams)(nil)
)
