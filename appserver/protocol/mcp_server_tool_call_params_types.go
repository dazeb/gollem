package protocol

import (
	"encoding/json"
	"errors"
)

// McpServerToolCallParams is the exact public MCP tool selector. It remains
// separate from the alias-compatible live request until an adapter is defined.
type McpServerToolCallParams struct {
	ThreadID  string     `json:"threadId"`
	Server    string     `json:"server"`
	Tool      string     `json:"tool"`
	Arguments *JsonValue `json:"arguments,omitempty"`
	Meta      *JsonValue `json:"_meta,omitempty"`
}

func (p McpServerToolCallParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		ThreadID  string     `json:"threadId"`
		Server    string     `json:"server"`
		Tool      string     `json:"tool"`
		Arguments *JsonValue `json:"arguments,omitempty"`
		Meta      *JsonValue `json:"_meta,omitempty"`
	}
	return json.Marshal(wire(p))
}

func (p *McpServerToolCallParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode MCP server tool-call params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"MCP server tool-call params",
		"threadId",
		"server",
		"tool",
		"arguments",
		"_meta",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](
		payload,
		"MCP server tool-call params",
		"threadId",
	)
	if err != nil {
		return err
	}
	server, err := decodeRequiredThreadItemValue[string](
		payload,
		"MCP server tool-call params",
		"server",
	)
	if err != nil {
		return err
	}
	tool, err := decodeRequiredThreadItemValue[string](
		payload,
		"MCP server tool-call params",
		"tool",
	)
	if err != nil {
		return err
	}
	arguments := decodeOptionalMcpServerToolCallJSONValue(payload, "arguments")
	meta := decodeOptionalMcpServerToolCallJSONValue(payload, "_meta")
	*p = McpServerToolCallParams{
		ThreadID:  threadID,
		Server:    server,
		Tool:      tool,
		Arguments: arguments,
		Meta:      meta,
	}
	return nil
}

func decodeOptionalMcpServerToolCallJSONValue(
	payload map[string]json.RawMessage,
	fieldName string,
) *JsonValue {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil
	}
	return &JsonValue{raw: append(json.RawMessage(nil), raw...)}
}

var (
	_ json.Marshaler   = McpServerToolCallParams{}
	_ json.Unmarshaler = (*McpServerToolCallParams)(nil)
)
