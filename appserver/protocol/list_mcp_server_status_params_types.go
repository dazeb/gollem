package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ListMcpServerStatusParams is the exact public MCP inventory page selector.
// It remains separate from the live server-filter request until an adapter is defined.
type ListMcpServerStatusParams struct {
	Cursor   *string                `json:"cursor,omitempty"`
	Limit    *uint32                `json:"limit,omitempty"`
	Detail   *McpServerStatusDetail `json:"detail,omitempty"`
	ThreadID *string                `json:"threadId,omitempty"`
}

func (p ListMcpServerStatusParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		Cursor   *string                `json:"cursor"`
		Limit    *uint32                `json:"limit"`
		Detail   *McpServerStatusDetail `json:"detail"`
		ThreadID *string                `json:"threadId"`
	}
	return json.Marshal(wire(p))
}

func (p *ListMcpServerStatusParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode MCP server status-list params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"MCP server status-list params",
		"cursor",
		"limit",
		"detail",
		"threadId",
	)
	if err != nil {
		return err
	}
	cursor, err := decodeOptionalNullableMcpStatusListValue[string](payload, "cursor")
	if err != nil {
		return err
	}
	limit, err := decodeOptionalNullableMcpStatusListValue[uint32](payload, "limit")
	if err != nil {
		return err
	}
	detail, err := decodeOptionalNullableMcpStatusListValue[McpServerStatusDetail](payload, "detail")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableMcpStatusListValue[string](payload, "threadId")
	if err != nil {
		return err
	}
	*p = ListMcpServerStatusParams{
		Cursor: cursor, Limit: limit, Detail: detail, ThreadID: threadID,
	}
	return nil
}

func decodeOptionalNullableMcpStatusListValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode MCP server status-list %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = ListMcpServerStatusParams{}
	_ json.Unmarshaler = (*ListMcpServerStatusParams)(nil)
)
