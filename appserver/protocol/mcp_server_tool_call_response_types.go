package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// McpServerToolCallResponse is the exact public JSON-valued tool result. It
// remains separate from the broader live MCP response until an adapter exists.
type McpServerToolCallResponse struct {
	Content           []JsonValue `json:"content" jsonschema:"nonnullable=true"`
	StructuredContent *JsonValue  `json:"structuredContent,omitempty"`
	IsError           *bool       `json:"isError,omitempty"`
	Meta              *JsonValue  `json:"_meta,omitempty"`
}

func (r McpServerToolCallResponse) MarshalJSON() ([]byte, error) {
	content := r.Content
	if content == nil {
		content = []JsonValue{}
	}
	return json.Marshal(struct {
		Content           []JsonValue `json:"content"`
		StructuredContent *JsonValue  `json:"structuredContent,omitempty"`
		IsError           *bool       `json:"isError,omitempty"`
		Meta              *JsonValue  `json:"_meta,omitempty"`
	}{
		Content:           content,
		StructuredContent: r.StructuredContent,
		IsError:           r.IsError,
		Meta:              r.Meta,
	})
}

func (r *McpServerToolCallResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP server tool-call response into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"MCP server tool-call response",
		"content",
		"structuredContent",
		"isError",
		"_meta",
	)
	if err != nil {
		return err
	}
	content, err := decodeRequiredThreadItemValue[[]JsonValue](
		payload,
		"MCP server tool-call response",
		"content",
	)
	if err != nil {
		return err
	}
	structuredContent := decodeOptionalMcpServerToolCallJSONValue(payload, "structuredContent")
	isError, err := decodeOptionalMcpServerToolCallResponseBool(payload, "isError")
	if err != nil {
		return err
	}
	meta := decodeOptionalMcpServerToolCallJSONValue(payload, "_meta")
	*r = McpServerToolCallResponse{
		Content: content, StructuredContent: structuredContent, IsError: isError, Meta: meta,
	}
	return nil
}

func decodeOptionalMcpServerToolCallResponseBool(
	payload map[string]json.RawMessage,
	fieldName string,
) (*bool, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode MCP server tool-call response %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = McpServerToolCallResponse{}
	_ json.Unmarshaler = (*McpServerToolCallResponse)(nil)
)
