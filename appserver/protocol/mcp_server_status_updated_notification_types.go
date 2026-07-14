package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// McpServerStatusUpdatedNotification is the exact public MCP startup-status
// value. It remains standalone until Gollem has a startup producer.
type McpServerStatusUpdatedNotification struct {
	ThreadID      *string                        `json:"threadId"`
	Name          string                         `json:"name"`
	Status        McpServerStartupState          `json:"status"`
	Error         *string                        `json:"error"`
	FailureReason *McpServerStartupFailureReason `json:"failureReason"`
}

func (n McpServerStatusUpdatedNotification) MarshalJSON() ([]byte, error) {
	type wire McpServerStatusUpdatedNotification
	encoded, err := json.Marshal(wire(n))
	if err != nil {
		return nil, fmt.Errorf("encode MCP server status-updated notification: %w", err)
	}
	return encoded, nil
}

func (n *McpServerStatusUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode MCP server status-updated notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"MCP server status-updated notification",
		"threadId",
		"name",
		"status",
		"error",
		"failureReason",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](
		payload,
		"MCP server status-updated notification",
		"name",
	)
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[McpServerStartupState](
		payload,
		"MCP server status-updated notification",
		"status",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableMcpServerStatusUpdatedValue[string](payload, "threadId")
	if err != nil {
		return err
	}
	errorMessage, err := decodeOptionalNullableMcpServerStatusUpdatedValue[string](payload, "error")
	if err != nil {
		return err
	}
	failureReason, err := decodeOptionalNullableMcpServerStatusUpdatedValue[McpServerStartupFailureReason](
		payload,
		"failureReason",
	)
	if err != nil {
		return err
	}
	*n = McpServerStatusUpdatedNotification{
		ThreadID:      threadID,
		Name:          name,
		Status:        status,
		Error:         errorMessage,
		FailureReason: failureReason,
	}
	return nil
}

func decodeOptionalNullableMcpServerStatusUpdatedValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode MCP server status-updated notification %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = McpServerStatusUpdatedNotification{}
	_ json.Unmarshaler = (*McpServerStatusUpdatedNotification)(nil)
)
