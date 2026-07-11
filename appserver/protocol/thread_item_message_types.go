package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

type ByteRange struct {
	Start uint64 `json:"start"`
	End   uint64 `json:"end"`
}

func (r *ByteRange) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode byte range into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "byte range", "start", "end")
	if err != nil {
		return err
	}
	start, err := decodeRequiredThreadItemValue[uint64](payload, "byte range", "start")
	if err != nil {
		return err
	}
	end, err := decodeRequiredThreadItemValue[uint64](payload, "byte range", "end")
	if err != nil {
		return err
	}
	*r = ByteRange{Start: start, End: end}
	return nil
}

type TextElement struct {
	ByteRange   ByteRange `json:"byteRange"`
	Placeholder *string   `json:"placeholder"`
}

func (e *TextElement) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode text element into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "text element", "byteRange", "placeholder")
	if err != nil {
		return err
	}
	byteRange, err := decodeRequiredThreadItemValue[ByteRange](payload, "text element", "byteRange")
	if err != nil {
		return err
	}
	placeholderRaw, ok := payload["placeholder"]
	if !ok {
		return errors.New("text element requires placeholder")
	}
	var placeholder *string
	if !isJSONNull(placeholderRaw) {
		var value string
		if err := json.Unmarshal(placeholderRaw, &value); err != nil {
			return fmt.Errorf("decode text element placeholder: %w", err)
		}
		placeholder = &value
	}
	*e = TextElement{ByteRange: byteRange, Placeholder: placeholder}
	return nil
}

type ImageDetail string

const (
	ImageDetailAuto     ImageDetail = "auto"
	ImageDetailLow      ImageDetail = "low"
	ImageDetailHigh     ImageDetail = "high"
	ImageDetailOriginal ImageDetail = "original"
)

func (d ImageDetail) MarshalJSON() ([]byte, error) {
	if !d.valid() {
		return nil, fmt.Errorf("unknown image detail %q", d)
	}
	return json.Marshal(string(d))
}

func (d *ImageDetail) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("decode image detail into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("image detail cannot be null")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode image detail: %w", err)
	}
	detail := ImageDetail(value)
	if !detail.valid() {
		return fmt.Errorf("unknown image detail %q", value)
	}
	*d = detail
	return nil
}

func (d ImageDetail) valid() bool {
	switch d {
	case ImageDetailAuto, ImageDetailLow, ImageDetailHigh, ImageDetailOriginal:
		return true
	default:
		return false
	}
}

type MessagePhase string

const (
	MessagePhaseCommentary  MessagePhase = "commentary"
	MessagePhaseFinalAnswer MessagePhase = "final_answer"
)

func (p MessagePhase) MarshalJSON() ([]byte, error) {
	if !p.valid() {
		return nil, fmt.Errorf("unknown message phase %q", p)
	}
	return json.Marshal(string(p))
}

func (p *MessagePhase) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode message phase into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("message phase cannot be null")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode message phase: %w", err)
	}
	phase := MessagePhase(value)
	if !phase.valid() {
		return fmt.Errorf("unknown message phase %q", value)
	}
	*p = phase
	return nil
}

func (p MessagePhase) valid() bool {
	switch p {
	case MessagePhaseCommentary, MessagePhaseFinalAnswer:
		return true
	default:
		return false
	}
}

type MemoryCitationEntry struct {
	Path      string `json:"path"`
	LineStart uint32 `json:"lineStart"`
	LineEnd   uint32 `json:"lineEnd"`
	Note      string `json:"note"`
}

func (e *MemoryCitationEntry) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode memory citation entry into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "memory citation entry", "path", "lineStart", "lineEnd", "note")
	if err != nil {
		return err
	}
	path, err := decodeRequiredThreadItemValue[string](payload, "memory citation entry", "path")
	if err != nil {
		return err
	}
	lineStart, err := decodeRequiredThreadItemValue[uint32](payload, "memory citation entry", "lineStart")
	if err != nil {
		return err
	}
	lineEnd, err := decodeRequiredThreadItemValue[uint32](payload, "memory citation entry", "lineEnd")
	if err != nil {
		return err
	}
	note, err := decodeRequiredThreadItemValue[string](payload, "memory citation entry", "note")
	if err != nil {
		return err
	}
	*e = MemoryCitationEntry{Path: path, LineStart: lineStart, LineEnd: lineEnd, Note: note}
	return nil
}

type MemoryCitation struct {
	Entries   []MemoryCitationEntry `json:"entries" jsonschema:"nonnullable=true"`
	ThreadIDs []string              `json:"threadIds" jsonschema:"nonnullable=true"`
}

func (c MemoryCitation) MarshalJSON() ([]byte, error) {
	entries := c.Entries
	if entries == nil {
		entries = []MemoryCitationEntry{}
	}
	threadIDs := c.ThreadIDs
	if threadIDs == nil {
		threadIDs = []string{}
	}
	return json.Marshal(struct {
		Entries   []MemoryCitationEntry `json:"entries"`
		ThreadIDs []string              `json:"threadIds"`
	}{Entries: entries, ThreadIDs: threadIDs})
}

func (c *MemoryCitation) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode memory citation into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "memory citation", "entries", "threadIds")
	if err != nil {
		return err
	}
	entries, err := decodeRequiredThreadItemValue[[]MemoryCitationEntry](payload, "memory citation", "entries")
	if err != nil {
		return err
	}
	threadIDs, err := decodeRequiredThreadItemValue[[]string](payload, "memory citation", "threadIds")
	if err != nil {
		return err
	}
	*c = MemoryCitation{Entries: entries, ThreadIDs: threadIDs}
	return nil
}

type HookPromptFragment struct {
	Text      string `json:"text"`
	HookRunID string `json:"hookRunId"`
}

func (f *HookPromptFragment) UnmarshalJSON(data []byte) error {
	if f == nil {
		return errors.New("decode hook prompt fragment into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "hook prompt fragment", "text", "hookRunId")
	if err != nil {
		return err
	}
	text, err := decodeRequiredThreadItemValue[string](payload, "hook prompt fragment", "text")
	if err != nil {
		return err
	}
	hookRunID, err := decodeRequiredThreadItemValue[string](payload, "hook prompt fragment", "hookRunId")
	if err != nil {
		return err
	}
	*f = HookPromptFragment{Text: text, HookRunID: hookRunID}
	return nil
}

// UserInput is the closed public turn-input union. TextElements intentionally
// retains the public snake_case wire name.
type UserInput struct {
	Type         string        `json:"type"`
	Text         string        `json:"text,omitempty"`
	TextElements []TextElement `json:"text_elements,omitempty"`
	Detail       *ImageDetail  `json:"detail,omitempty"`
	URL          string        `json:"url,omitempty"`
	Name         string        `json:"name,omitempty"`
	Path         string        `json:"path,omitempty"`
}

func (i UserInput) MarshalJSON() ([]byte, error) {
	switch i.Type {
	case "text":
		if i.Detail != nil || i.URL != "" || i.Name != "" || i.Path != "" {
			return nil, errors.New("text user input contains fields from another variant")
		}
		elements := i.TextElements
		if elements == nil {
			elements = []TextElement{}
		}
		return json.Marshal(struct {
			Type         string        `json:"type"`
			Text         string        `json:"text"`
			TextElements []TextElement `json:"text_elements"`
		}{Type: i.Type, Text: i.Text, TextElements: elements})
	case "image":
		if i.Text != "" || i.TextElements != nil || i.Name != "" || i.Path != "" {
			return nil, errors.New("image user input contains fields from another variant")
		}
		return marshalImageUserInput(i.Type, i.Detail, "url", i.URL)
	case "localImage":
		if i.Text != "" || i.TextElements != nil || i.URL != "" || i.Name != "" {
			return nil, errors.New("localImage user input contains fields from another variant")
		}
		return marshalImageUserInput(i.Type, i.Detail, "path", i.Path)
	case "skill", "mention":
		if i.Text != "" || i.TextElements != nil || i.Detail != nil || i.URL != "" {
			return nil, fmt.Errorf("%s user input contains fields from another variant", i.Type)
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Path string `json:"path"`
		}{Type: i.Type, Name: i.Name, Path: i.Path})
	default:
		return nil, fmt.Errorf("unknown user input type %q", i.Type)
	}
}

func (i *UserInput) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode user input into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "user input", "type", "text", "text_elements", "detail", "url", "name", "path")
	if err != nil {
		return err
	}
	inputType, err := decodeRequiredThreadItemValue[string](payload, "user input", "type")
	if err != nil {
		return err
	}
	switch inputType {
	case "text":
		if err := rejectThreadItemFields(payload, "text user input", "type", "text", "text_elements"); err != nil {
			return err
		}
		text, err := decodeRequiredThreadItemValue[string](payload, "text user input", "text")
		if err != nil {
			return err
		}
		elements, err := decodeRequiredThreadItemValue[[]TextElement](payload, "text user input", "text_elements")
		if err != nil {
			return err
		}
		*i = UserInput{Type: inputType, Text: text, TextElements: elements}
		return nil
	case "image":
		if err := rejectThreadItemFields(payload, "image user input", "type", "detail", "url"); err != nil {
			return err
		}
		url, err := decodeRequiredThreadItemValue[string](payload, "image user input", "url")
		if err != nil {
			return err
		}
		detail, err := decodeOptionalImageDetail(payload, "image user input")
		if err != nil {
			return err
		}
		*i = UserInput{Type: inputType, Detail: detail, URL: url}
		return nil
	case "localImage":
		if err := rejectThreadItemFields(payload, "localImage user input", "type", "detail", "path"); err != nil {
			return err
		}
		path, err := decodeRequiredThreadItemValue[string](payload, "localImage user input", "path")
		if err != nil {
			return err
		}
		detail, err := decodeOptionalImageDetail(payload, "localImage user input")
		if err != nil {
			return err
		}
		*i = UserInput{Type: inputType, Detail: detail, Path: path}
		return nil
	case "skill", "mention":
		if err := rejectThreadItemFields(payload, inputType+" user input", "type", "name", "path"); err != nil {
			return err
		}
		name, err := decodeRequiredThreadItemValue[string](payload, inputType+" user input", "name")
		if err != nil {
			return err
		}
		path, err := decodeRequiredThreadItemValue[string](payload, inputType+" user input", "path")
		if err != nil {
			return err
		}
		*i = UserInput{Type: inputType, Name: name, Path: path}
		return nil
	default:
		return fmt.Errorf("unknown user input type %q", inputType)
	}
}

func marshalImageUserInput(inputType string, detail *ImageDetail, valueName, value string) ([]byte, error) {
	if detail != nil && !detail.valid() {
		return nil, fmt.Errorf("unknown image detail %q", *detail)
	}
	if valueName == "url" {
		return json.Marshal(struct {
			Type   string       `json:"type"`
			Detail *ImageDetail `json:"detail,omitempty"`
			URL    string       `json:"url"`
		}{Type: inputType, Detail: detail, URL: value})
	}
	return json.Marshal(struct {
		Type   string       `json:"type"`
		Detail *ImageDetail `json:"detail,omitempty"`
		Path   string       `json:"path"`
	}{Type: inputType, Detail: detail, Path: value})
}

func decodeOptionalImageDetail(payload map[string]json.RawMessage, objectName string) (*ImageDetail, error) {
	raw, ok := payload["detail"]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("%s detail cannot be null", objectName)
	}
	var detail ImageDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, fmt.Errorf("decode %s detail: %w", objectName, err)
	}
	return &detail, nil
}

func decodeExactThreadItemObject(data []byte, objectName string, allowed ...string) (map[string]json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if payload == nil {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	if err := rejectThreadItemFields(payload, objectName, allowed...); err != nil {
		return nil, err
	}
	return payload, nil
}

func rejectThreadItemFields(payload map[string]json.RawMessage, objectName string, allowed ...string) error {
	for name := range payload {
		known := false
		for _, candidate := range allowed {
			if name == candidate {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("unknown %s field %q", objectName, name)
		}
	}
	return nil
}

func decodeRequiredThreadItemValue[T any](payload map[string]json.RawMessage, objectName, fieldName string) (T, error) {
	var zero T
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return zero, fmt.Errorf("%s requires %s", objectName, fieldName)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return zero, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func isJSONNull(data []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), []byte("null"))
}
