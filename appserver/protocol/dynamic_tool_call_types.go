package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// DynamicToolCallParams is the public client-tool request.
type DynamicToolCallParams struct {
	ThreadID  string          `json:"threadId"`
	TurnID    string          `json:"turnId"`
	CallID    string          `json:"callId"`
	Namespace *string         `json:"namespace"`
	Tool      string          `json:"tool"`
	Arguments json.RawMessage `json:"arguments"`
}

func (p DynamicToolCallParams) MarshalJSON() ([]byte, error) {
	arguments := p.Arguments
	if len(bytes.TrimSpace(arguments)) == 0 {
		arguments = json.RawMessage("null")
	}
	if err := requireDynamicToolJSONValue(arguments, "arguments"); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		CallID    string          `json:"callId"`
		Namespace *string         `json:"namespace"`
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}{p.ThreadID, p.TurnID, p.CallID, p.Namespace, p.Tool, arguments})
}

func (p *DynamicToolCallParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode dynamic tool call params into nil receiver")
	}
	const objectName = "dynamic tool call params"
	payload, err := decodeRustSerdeObject(
		data, objectName, "threadId", "turnId", "callId", "namespace", "tool", "arguments",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, objectName, "turnId")
	if err != nil {
		return err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "callId")
	if err != nil {
		return err
	}
	namespace, err := decodeOptionalNullableConfigValue[string](payload, objectName, "namespace")
	if err != nil {
		return err
	}
	tool, err := decodeRequiredThreadItemValue[string](payload, objectName, "tool")
	if err != nil {
		return err
	}
	arguments, err := requiredDynamicToolJSONValue(payload, objectName, "arguments")
	if err != nil {
		return err
	}
	*p = DynamicToolCallParams{
		ThreadID: threadID, TurnID: turnID, CallID: callID, Namespace: namespace, Tool: tool, Arguments: arguments,
	}
	return nil
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
	const objectName = "dynamic tool call response"
	payload, err := decodeRustSerdeObject(data, objectName, "contentItems", "success")
	if err != nil {
		return err
	}
	contentItems, err := decodeRequiredThreadItemValue[[]DynamicToolCallOutputContentItem](payload, objectName, "contentItems")
	if err != nil {
		return err
	}
	success, err := decodeRequiredThreadItemValue[bool](payload, objectName, "success")
	if err != nil {
		return err
	}
	*r = DynamicToolCallResponse{ContentItems: contentItems, Success: success}
	return nil
}

type DynamicToolCallOutputContentItem struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	AudioURL string `json:"audioUrl,omitempty"`
}

func (i DynamicToolCallOutputContentItem) MarshalJSON() ([]byte, error) {
	switch i.Type {
	case "inputText":
		if i.ImageURL != "" || i.AudioURL != "" {
			return nil, errors.New("inputText dynamic tool content cannot include imageUrl or audioUrl")
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{Type: i.Type, Text: i.Text})
	case "inputImage":
		if i.Text != "" || i.AudioURL != "" {
			return nil, errors.New("inputImage dynamic tool content cannot include text or audioUrl")
		}
		return json.Marshal(struct {
			Type     string `json:"type"`
			ImageURL string `json:"imageUrl"`
		}{Type: i.Type, ImageURL: i.ImageURL})
	case "inputAudio":
		if i.Text != "" || i.ImageURL != "" {
			return nil, errors.New("inputAudio dynamic tool content cannot include text or imageUrl")
		}
		return json.Marshal(struct {
			Type     string `json:"type"`
			AudioURL string `json:"audioUrl"`
		}{Type: i.Type, AudioURL: i.AudioURL})
	default:
		return nil, fmt.Errorf("unknown dynamic tool content type %q", i.Type)
	}
}

func (i *DynamicToolCallOutputContentItem) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode dynamic tool content into nil receiver")
	}
	const objectName = "dynamic tool call output content item"
	tagPayload, err := decodeRustSerdeObject(data, objectName, "type")
	if err != nil {
		return err
	}
	contentType, err := decodeRequiredThreadItemValue[string](tagPayload, objectName, "type")
	if err != nil {
		return err
	}
	switch contentType {
	case "inputText":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "text")
		if err != nil {
			return err
		}
		text, err := decodeRequiredThreadItemValue[string](payload, objectName, "text")
		if err != nil {
			return err
		}
		*i = DynamicToolCallOutputContentItem{Type: contentType, Text: text}
		return nil
	case "inputImage":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "imageUrl")
		if err != nil {
			return err
		}
		imageURL, err := decodeRequiredThreadItemValue[string](payload, objectName, "imageUrl")
		if err != nil {
			return err
		}
		*i = DynamicToolCallOutputContentItem{Type: contentType, ImageURL: imageURL}
		return nil
	case "inputAudio":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "audioUrl")
		if err != nil {
			return err
		}
		audioURL, err := decodeRequiredThreadItemValue[string](payload, objectName, "audioUrl")
		if err != nil {
			return err
		}
		*i = DynamicToolCallOutputContentItem{Type: contentType, AudioURL: audioURL}
		return nil
	default:
		return fmt.Errorf("unknown dynamic tool content type %q", contentType)
	}
}

func requiredDynamicToolJSONValue(
	payload map[string]json.RawMessage,
	objectName, fieldName string,
) (json.RawMessage, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if err := requireDynamicToolJSONValue(raw, fieldName); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return append(json.RawMessage(nil), raw...), nil
}

func requireDynamicToolJSONValue(raw json.RawMessage, fieldName string) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		return fmt.Errorf("dynamic tool call requires %s", fieldName)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("dynamic tool call %s must be valid JSON: %w", fieldName, err)
	}
	return nil
}
