package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JsonValue retains one precision-preserving JSON value.
type JsonValue struct {
	raw json.RawMessage
}

func (v JsonValue) MarshalJSON() ([]byte, error) {
	if len(v.raw) == 0 {
		return nil, errors.New("JSON value has no value")
	}
	return canonicalizeResponseItemJSONValue(v.raw)
}

func (v *JsonValue) UnmarshalJSON(data []byte) error {
	if v == nil {
		return errors.New("decode JSON value into nil receiver")
	}
	canonical, err := canonicalizeResponseItemJSONValue(data)
	if err != nil {
		return fmt.Errorf("decode JSON value: %w", err)
	}
	v.raw = canonical
	return nil
}

type ImageGenerationItem struct {
	ID            string           `json:"id"`
	Status        string           `json:"status"`
	RevisedPrompt *string          `json:"revisedPrompt"`
	Result        string           `json:"result"`
	SavedPath     *AbsolutePathBuf `json:"savedPath,omitempty" jsonschema:"nonnullable=true"`
}

func (i *ImageGenerationItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode image-generation item into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "image-generation item", "id", "status", "revisedPrompt", "result", "savedPath")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, "image-generation item", "id")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[string](payload, "image-generation item", "status")
	if err != nil {
		return err
	}
	revisedPrompt, err := decodeRequiredNullableThreadItemValue[string](payload, "image-generation item", "revisedPrompt")
	if err != nil {
		return err
	}
	result, err := decodeRequiredThreadItemValue[string](payload, "image-generation item", "result")
	if err != nil {
		return err
	}
	savedPath, err := decodeOptionalResponseItemValue[AbsolutePathBuf](payload, "image-generation item", "savedPath")
	if err != nil {
		return err
	}
	*i = ImageGenerationItem{ID: id, Status: status, RevisedPrompt: revisedPrompt, Result: result, SavedPath: savedPath}
	return nil
}

// McpToolCallResult is the exact public JSON-valued result. Gollem's live
// MCPToolCallResult remains a separate legacy MCPContent-backed value.
type McpToolCallResult struct {
	Content           []JsonValue `json:"content" jsonschema:"nonnullable=true"`
	StructuredContent *JsonValue  `json:"structuredContent"`
	Meta              *JsonValue  `json:"_meta"`
}

func (r McpToolCallResult) MarshalJSON() ([]byte, error) {
	content := r.Content
	if content == nil {
		content = []JsonValue{}
	}
	return json.Marshal(struct {
		Content           []JsonValue `json:"content"`
		StructuredContent *JsonValue  `json:"structuredContent"`
		Meta              *JsonValue  `json:"_meta"`
	}{Content: content, StructuredContent: r.StructuredContent, Meta: r.Meta})
}

func (r *McpToolCallResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP tool-call result into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "MCP tool-call result", "content", "structuredContent", "_meta")
	if err != nil {
		return err
	}
	content, err := decodeRequiredThreadItemValue[[]JsonValue](payload, "MCP tool-call result", "content")
	if err != nil {
		return err
	}
	structuredContent, err := decodeRequiredNullableThreadItemValue[JsonValue](payload, "MCP tool-call result", "structuredContent")
	if err != nil {
		return err
	}
	meta, err := decodeRequiredNullableThreadItemValue[JsonValue](payload, "MCP tool-call result", "_meta")
	if err != nil {
		return err
	}
	*r = McpToolCallResult{Content: content, StructuredContent: structuredContent, Meta: meta}
	return nil
}
