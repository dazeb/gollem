package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type ContentItem struct {
	raw json.RawMessage
}

func (c ContentItem) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("content item has no value")
	}
	return validateContentItemJSON(c.raw, "content item", contentItemVariants)
}

func (c *ContentItem) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode content item into nil receiver")
	}
	canonical, err := validateContentItemJSON(data, "content item", contentItemVariants)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

var contentItemVariants = []contentItemVariant{
	{itemType: "input_text", field: "text"},
	{itemType: "input_image", field: "image_url", optionalDetail: true},
	{itemType: "output_text", field: "text"},
}

type FunctionCallOutputContentItem struct {
	raw json.RawMessage
}

func (c FunctionCallOutputContentItem) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("function-call output content item has no value")
	}
	return validateContentItemJSON(c.raw, "function-call output content item", functionCallOutputContentItemVariants)
}

func (c *FunctionCallOutputContentItem) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode function-call output content item into nil receiver")
	}
	canonical, err := validateContentItemJSON(data, "function-call output content item", functionCallOutputContentItemVariants)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

var functionCallOutputContentItemVariants = []contentItemVariant{
	{itemType: "input_text", field: "text"},
	{itemType: "input_image", field: "image_url", optionalDetail: true},
	{itemType: "encrypted_content", field: "encrypted_content"},
}

type contentItemVariant struct {
	itemType       string
	field          string
	optionalDetail bool
}

func validateContentItemJSON(data []byte, objectName string, variants []contentItemVariant) (json.RawMessage, error) {
	allowedFields := []string{"type"}
	for _, variant := range variants {
		if !containsRawResponseField(allowedFields, variant.field) {
			allowedFields = append(allowedFields, variant.field)
		}
		if variant.optionalDetail && !containsRawResponseField(allowedFields, "detail") {
			allowedFields = append(allowedFields, "detail")
		}
	}
	payload, err := decodeExactThreadItemObject(data, objectName, allowedFields...)
	if err != nil {
		return nil, err
	}
	itemType, err := decodeRequiredThreadItemValue[string](payload, objectName, "type")
	if err != nil {
		return nil, err
	}
	for _, variant := range variants {
		if itemType != variant.itemType {
			continue
		}
		allowed := []string{"type", variant.field}
		if variant.optionalDetail {
			allowed = append(allowed, "detail")
		}
		if err := rejectThreadItemFields(payload, objectName+" "+itemType, allowed...); err != nil {
			return nil, err
		}
		value, err := decodeRequiredThreadItemValue[string](payload, objectName+" "+itemType, variant.field)
		if err != nil {
			return nil, err
		}
		switch variant.field {
		case "image_url":
			detail, err := decodeOptionalImageDetail(payload, objectName+" "+itemType)
			if err != nil {
				return nil, err
			}
			return json.Marshal(struct {
				Type     string       `json:"type"`
				ImageURL string       `json:"image_url"`
				Detail   *ImageDetail `json:"detail,omitempty"`
			}{Type: itemType, ImageURL: value, Detail: detail})
		case "encrypted_content":
			return json.Marshal(struct {
				Type             string `json:"type"`
				EncryptedContent string `json:"encrypted_content"`
			}{Type: itemType, EncryptedContent: value})
		default:
			return json.Marshal(struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{Type: itemType, Text: value})
		}
	}
	return nil, fmt.Errorf("unknown %s type %q", objectName, itemType)
}

type FunctionCallOutputBody struct {
	raw json.RawMessage
}

func (b FunctionCallOutputBody) MarshalJSON() ([]byte, error) {
	if len(b.raw) == 0 {
		return nil, errors.New("function-call output body has no value")
	}
	return validateFunctionCallOutputBodyJSON(b.raw)
}

func (b *FunctionCallOutputBody) UnmarshalJSON(data []byte) error {
	if b == nil {
		return errors.New("decode function-call output body into nil receiver")
	}
	canonical, err := validateFunctionCallOutputBodyJSON(data)
	if err != nil {
		return err
	}
	b.raw = canonical
	return nil
}

func validateFunctionCallOutputBodyJSON(data []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("function-call output body is empty")
	}
	switch trimmed[0] {
	case '"':
		var value string
		if err := json.Unmarshal(trimmed, &value); err != nil {
			return nil, fmt.Errorf("decode function-call output body string: %w", err)
		}
		return json.Marshal(value)
	case '[':
		var items []FunctionCallOutputContentItem
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, fmt.Errorf("decode function-call output body items: %w", err)
		}
		return json.Marshal(items)
	default:
		return nil, errors.New("function-call output body must be a string or content-item array")
	}
}

type InternalChatMessageMetadataPassthrough struct {
	TurnID *string `json:"turn_id,omitempty"`
}

func (m InternalChatMessageMetadataPassthrough) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		TurnID *string `json:"turn_id,omitempty"`
	}{TurnID: m.TurnID})
}

func (m *InternalChatMessageMetadataPassthrough) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode internal chat-message metadata into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "internal chat-message metadata", "turn_id")
	if err != nil {
		return err
	}
	turnID, err := decodeOptionalNonNullMetadataString(payload, "turn_id")
	if err != nil {
		return err
	}
	m.TurnID = turnID
	return nil
}

func decodeOptionalNonNullMetadataString(payload map[string]json.RawMessage, fieldName string) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("internal chat-message metadata %s cannot be null", fieldName)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode internal chat-message metadata %s: %w", fieldName, err)
	}
	return &value, nil
}

type LocalShellStatus string

const (
	LocalShellStatusCompleted  LocalShellStatus = "completed"
	LocalShellStatusInProgress LocalShellStatus = "in_progress"
	LocalShellStatusIncomplete LocalShellStatus = "incomplete"
)

func (s LocalShellStatus) MarshalJSON() ([]byte, error) {
	if !s.valid() {
		return nil, fmt.Errorf("invalid local-shell status %q", s)
	}
	return json.Marshal(string(s))
}

func (s *LocalShellStatus) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode local-shell status into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("local-shell status cannot be null")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode local-shell status: %w", err)
	}
	status := LocalShellStatus(value)
	if !status.valid() {
		return fmt.Errorf("invalid local-shell status %q", value)
	}
	*s = status
	return nil
}

func (s LocalShellStatus) valid() bool {
	switch s {
	case LocalShellStatusCompleted, LocalShellStatusInProgress, LocalShellStatusIncomplete:
		return true
	default:
		return false
	}
}

type LocalShellAction struct {
	raw json.RawMessage
}

func (a LocalShellAction) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("local-shell action has no value")
	}
	return validateLocalShellActionJSON(a.raw)
}

func (a *LocalShellAction) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode local-shell action into nil receiver")
	}
	canonical, err := validateLocalShellActionJSON(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

func validateLocalShellActionJSON(data []byte) (json.RawMessage, error) {
	payload, err := decodeExactThreadItemObject(
		data,
		"local-shell action",
		"type",
		"command",
		"timeout_ms",
		"working_directory",
		"env",
		"user",
	)
	if err != nil {
		return nil, err
	}
	actionType, err := decodeRequiredThreadItemValue[string](payload, "local-shell action", "type")
	if err != nil {
		return nil, err
	}
	if actionType != "exec" {
		return nil, fmt.Errorf("unknown local-shell action type %q", actionType)
	}
	command, err := decodeRequiredNonNullStringArray(payload, "local-shell exec action", "command")
	if err != nil {
		return nil, err
	}
	timeoutMS, err := decodeRequiredNullableThreadItemValue[uint64](payload, "local-shell exec action", "timeout_ms")
	if err != nil {
		return nil, err
	}
	workingDirectory, err := decodeRequiredNullableThreadItemValue[string](payload, "local-shell exec action", "working_directory")
	if err != nil {
		return nil, err
	}
	env, err := decodeRequiredNullableStringMap(payload, "local-shell exec action", "env")
	if err != nil {
		return nil, err
	}
	user, err := decodeRequiredNullableThreadItemValue[string](payload, "local-shell exec action", "user")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type             string             `json:"type"`
		Command          []string           `json:"command"`
		TimeoutMS        *uint64            `json:"timeout_ms"`
		WorkingDirectory *string            `json:"working_directory"`
		Env              *map[string]string `json:"env"`
		User             *string            `json:"user"`
	}{
		Type:             actionType,
		Command:          command,
		TimeoutMS:        timeoutMS,
		WorkingDirectory: workingDirectory,
		Env:              env,
		User:             user,
	})
}

func decodeRequiredNullableThreadItemValue[T any](payload map[string]json.RawMessage, objectName, fieldName string) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func decodeRequiredNonNullStringArray(payload map[string]json.RawMessage, objectName, fieldName string) ([]string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]string, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: expected string", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return values, nil
}

func decodeRequiredNullableStringMap(payload map[string]json.RawMessage, objectName, fieldName string) (*map[string]string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	if isJSONNull(raw) {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make(map[string]string, len(fields))
	for name, value := range fields {
		if isJSONNull(value) {
			return nil, fmt.Errorf("decode %s %s[%q]: expected string", objectName, fieldName, name)
		}
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, fmt.Errorf("decode %s %s[%q]: %w", objectName, fieldName, name, err)
		}
		values[name] = decoded
	}
	return &values, nil
}
