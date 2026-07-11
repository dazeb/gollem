package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// DynamicToolCallParams is the public client-tool request. The trailing fields
// preserve Gollem's v1 request metadata for existing clients.
type DynamicToolCallParams struct {
	ThreadID    string          `json:"threadId"`
	TurnID      string          `json:"turnId"`
	CallID      string          `json:"callId"`
	Namespace   *string         `json:"namespace"`
	Tool        string          `json:"tool"`
	Arguments   json.RawMessage `json:"arguments"`
	RequestID   string          `json:"requestId,omitempty"`
	ItemID      string          `json:"itemId,omitempty"`
	StartedAtMS int64           `json:"startedAtMs,omitempty"`
	ToolName    string          `json:"toolName,omitempty"`
	Name        string          `json:"name,omitempty"`
	Metadata    map[string]any  `json:"metadata,omitempty"`
	Reason      string          `json:"reason,omitempty"`
}

type DynamicToolCallResponse struct {
	ContentItems []DynamicToolCallOutputContentItem `json:"contentItems" jsonschema:"nonnullable=true"`
	Success      bool                               `json:"success"`
}

func (r DynamicToolCallResponse) MarshalJSON() ([]byte, error) {
	items := r.ContentItems
	if items == nil {
		items = []DynamicToolCallOutputContentItem{}
	}
	return json.Marshal(struct {
		ContentItems []DynamicToolCallOutputContentItem `json:"contentItems"`
		Success      bool                               `json:"success"`
	}{ContentItems: items, Success: r.Success})
}

func (r *DynamicToolCallResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode dynamic tool call response into nil receiver")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	for name := range payload {
		switch name {
		case "contentItems", "success":
		default:
			return fmt.Errorf("unknown dynamic tool call response field %q", name)
		}
	}
	contentItemsRaw := payload["contentItems"]
	if len(contentItemsRaw) == 0 || bytes.Equal(bytes.TrimSpace(contentItemsRaw), []byte("null")) {
		return errors.New("dynamic tool call response requires contentItems array")
	}
	successRaw := payload["success"]
	if len(successRaw) == 0 || bytes.Equal(bytes.TrimSpace(successRaw), []byte("null")) {
		return errors.New("dynamic tool call response requires success")
	}
	var contentItems []DynamicToolCallOutputContentItem
	if err := json.Unmarshal(contentItemsRaw, &contentItems); err != nil {
		return fmt.Errorf("decode dynamic tool call response contentItems: %w", err)
	}
	var success bool
	if err := json.Unmarshal(successRaw, &success); err != nil {
		return fmt.Errorf("decode dynamic tool call response success: %w", err)
	}
	*r = DynamicToolCallResponse{ContentItems: contentItems, Success: success}
	return nil
}

type DynamicToolCallOutputContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
}

func (i DynamicToolCallOutputContentItem) MarshalJSON() ([]byte, error) {
	switch i.Type {
	case "inputText":
		if i.ImageURL != "" {
			return nil, errors.New("inputText dynamic tool content cannot include imageUrl")
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: i.Type, Text: i.Text})
	case "inputImage":
		if i.Text != "" {
			return nil, errors.New("inputImage dynamic tool content cannot include text")
		}
		return json.Marshal(struct {
			Type     string `json:"type"`
			ImageURL string `json:"imageUrl"`
		}{Type: i.Type, ImageURL: i.ImageURL})
	default:
		return nil, fmt.Errorf("unknown dynamic tool content type %q", i.Type)
	}
}

func (i *DynamicToolCallOutputContentItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode dynamic tool content into nil receiver")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	for name := range payload {
		switch name {
		case "type", "text", "imageUrl":
		default:
			return fmt.Errorf("unknown dynamic tool content field %q", name)
		}
	}
	contentType, err := decodeRequiredDynamicToolString(payload, "type")
	if err != nil {
		return err
	}
	switch contentType {
	case "inputText":
		if _, exists := payload["imageUrl"]; exists {
			return errors.New("inputText dynamic tool content cannot include imageUrl")
		}
		text, err := decodeRequiredDynamicToolString(payload, "text")
		if err != nil {
			return err
		}
		*i = DynamicToolCallOutputContentItem{Type: contentType, Text: text}
		return nil
	case "inputImage":
		if _, exists := payload["text"]; exists {
			return errors.New("inputImage dynamic tool content cannot include text")
		}
		imageURL, err := decodeRequiredDynamicToolString(payload, "imageUrl")
		if err != nil {
			return err
		}
		*i = DynamicToolCallOutputContentItem{Type: contentType, ImageURL: imageURL}
		return nil
	default:
		return fmt.Errorf("unknown dynamic tool content type %q", contentType)
	}
}

func decodeRequiredDynamicToolString(payload map[string]json.RawMessage, name string) (string, error) {
	raw, ok := payload[name]
	if !ok {
		return "", fmt.Errorf("dynamic tool content requires %s", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("decode dynamic tool content %s: %w", name, err)
	}
	return value, nil
}
