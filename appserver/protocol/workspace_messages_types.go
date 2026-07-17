package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// WorkspaceMessageType is the public workspace-message category. Unknown
// backend strings collapse to the serde fallback literal.
type WorkspaceMessageType string

const (
	WorkspaceMessageTypeHeadline     WorkspaceMessageType = "headline"
	WorkspaceMessageTypeAnnouncement WorkspaceMessageType = "announcement"
	WorkspaceMessageTypeUnknown      WorkspaceMessageType = "unknown"
)

func (t WorkspaceMessageType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(t, "workspace message type", WorkspaceMessageType.valid)
}

func (t *WorkspaceMessageType) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode workspace message type into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("decode workspace message type: value cannot be null")
	}
	var literal string
	if err := json.Unmarshal(data, &literal); err != nil {
		return fmt.Errorf("decode workspace message type: %w", err)
	}
	value := WorkspaceMessageType(literal)
	if !value.valid() {
		value = WorkspaceMessageTypeUnknown
	}
	*t = value
	return nil
}

func (t WorkspaceMessageType) valid() bool {
	return t == WorkspaceMessageTypeHeadline ||
		t == WorkspaceMessageTypeAnnouncement ||
		t == WorkspaceMessageTypeUnknown
}

// WorkspaceMessage is exact standalone backend message data.
type WorkspaceMessage struct {
	MessageID   string               `json:"messageId"`
	MessageType WorkspaceMessageType `json:"messageType"`
	MessageBody string               `json:"messageBody"`
	CreatedAt   *int64               `json:"createdAt,omitempty"`
	ArchivedAt  *int64               `json:"archivedAt,omitempty"`
}

func (m WorkspaceMessage) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		MessageID   string               `json:"messageId"`
		MessageType WorkspaceMessageType `json:"messageType"`
		MessageBody string               `json:"messageBody"`
		CreatedAt   *int64               `json:"createdAt"`
		ArchivedAt  *int64               `json:"archivedAt"`
	}{
		MessageID: m.MessageID, MessageType: m.MessageType, MessageBody: m.MessageBody,
		CreatedAt: m.CreatedAt, ArchivedAt: m.ArchivedAt,
	})
}

func (m *WorkspaceMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode workspace message into nil receiver")
	}
	const objectName = "workspace message"
	payload, err := decodeRustSerdeObject(
		data, objectName, "messageId", "messageType", "messageBody", "createdAt", "archivedAt",
	)
	if err != nil {
		return err
	}
	messageID, err := decodeRequiredThreadItemValue[string](payload, objectName, "messageId")
	if err != nil {
		return err
	}
	messageType, err := decodeRequiredThreadItemValue[WorkspaceMessageType](payload, objectName, "messageType")
	if err != nil {
		return err
	}
	messageBody, err := decodeRequiredThreadItemValue[string](payload, objectName, "messageBody")
	if err != nil {
		return err
	}
	createdAt, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "createdAt")
	if err != nil {
		return err
	}
	archivedAt, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "archivedAt")
	if err != nil {
		return err
	}
	*m = WorkspaceMessage{
		MessageID: messageID, MessageType: messageType, MessageBody: messageBody,
		CreatedAt: createdAt, ArchivedAt: archivedAt,
	}
	return nil
}

// GetWorkspaceMessagesResponse is exact standalone workspace-message list
// data. It does not imply that Gollem owns a workspace-message backend.
type GetWorkspaceMessagesResponse struct {
	FeatureEnabled bool               `json:"featureEnabled"`
	Messages       []WorkspaceMessage `json:"messages"`
}

func (r GetWorkspaceMessagesResponse) MarshalJSON() ([]byte, error) {
	if r.Messages == nil {
		return nil, errors.New("workspace messages response messages cannot be null")
	}
	type wire GetWorkspaceMessagesResponse
	return json.Marshal(wire(r))
}

func (r *GetWorkspaceMessagesResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode workspace messages response into nil receiver")
	}
	const objectName = "workspace messages response"
	payload, err := decodeRustSerdeObject(data, objectName, "featureEnabled", "messages")
	if err != nil {
		return err
	}
	featureEnabled, err := decodeRequiredThreadItemValue[bool](payload, objectName, "featureEnabled")
	if err != nil {
		return err
	}
	messages, err := decodeRequiredThreadItemArray[WorkspaceMessage](payload, objectName, "messages")
	if err != nil {
		return err
	}
	*r = GetWorkspaceMessagesResponse{FeatureEnabled: featureEnabled, Messages: messages}
	return nil
}

func workspaceMessageSchemas() map[string]Schema {
	return map[string]Schema{
		"WorkspaceMessageType": stringEnumSchema("headline", "announcement", "unknown"),
		"WorkspaceMessage": {
			"type": "object",
			"properties": Schema{
				"archivedAt": Schema{
					"description": "Unix timestamp (in seconds) when the message was archived.",
					"format":      "int64", "type": []any{"integer", "null"},
				},
				"createdAt": Schema{
					"description": "Unix timestamp (in seconds) when the message was created.",
					"format":      "int64", "type": []any{"integer", "null"},
				},
				"messageBody": Schema{"type": "string"},
				"messageId":   Schema{"type": "string"},
				"messageType": Schema{"$ref": "#/$defs/WorkspaceMessageType"},
			},
			"required": []string{"messageBody", "messageId", "messageType"},
		},
		"GetWorkspaceMessagesResponse": {
			"type": "object",
			"properties": Schema{
				"featureEnabled": Schema{
					"description": "Whether the workspace-message backend route is available for this client.",
					"type":        "boolean",
				},
				"messages": Schema{
					"description": "Active workspace messages returned by the backend.",
					"items":       Schema{"$ref": "#/$defs/WorkspaceMessage"}, "type": "array",
				},
			},
			"required": []string{"featureEnabled", "messages"},
		},
	}
}

var (
	_ json.Marshaler   = WorkspaceMessageType("")
	_ json.Unmarshaler = (*WorkspaceMessageType)(nil)
	_ json.Marshaler   = WorkspaceMessage{}
	_ json.Unmarshaler = (*WorkspaceMessage)(nil)
	_ json.Marshaler   = GetWorkspaceMessagesResponse{}
	_ json.Unmarshaler = (*GetWorkspaceMessagesResponse)(nil)
)
