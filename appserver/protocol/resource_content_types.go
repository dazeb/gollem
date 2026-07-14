package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ResourceContent retains one exact public MCP resource value. It remains
// standalone until the public resource-read response and live adapter exist.
type ResourceContent struct {
	raw json.RawMessage
}

func (c ResourceContent) MarshalJSON() ([]byte, error) {
	if len(c.raw) == 0 {
		return nil, errors.New("resource content has no value")
	}
	return validateResourceContentJSON(c.raw)
}

func (c *ResourceContent) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode resource content into nil receiver")
	}
	canonical, err := validateResourceContentJSON(data)
	if err != nil {
		return err
	}
	c.raw = canonical
	return nil
}

func validateResourceContentJSON(data []byte) (json.RawMessage, error) {
	text, textErr := validateTextResourceContentJSON(data)
	if textErr == nil {
		return text, nil
	}
	blob, blobErr := validateBlobResourceContentJSON(data)
	if blobErr == nil {
		return blob, nil
	}
	return nil, fmt.Errorf("decode resource content: text variant: %w; blob variant: %w", textErr, blobErr)
}

func validateTextResourceContentJSON(data []byte) (json.RawMessage, error) {
	const objectName = "text resource content"
	payload, err := decodeResourceContentVariantObject(data, objectName, "uri", "mimeType", "text", "_meta")
	if err != nil {
		return nil, err
	}
	uri, err := decodeRequiredThreadItemValue[string](payload, objectName, "uri")
	if err != nil {
		return nil, err
	}
	mimeType, err := decodeOptionalResourceContentString(payload, objectName, "mimeType")
	if err != nil {
		return nil, err
	}
	text, err := decodeRequiredThreadItemValue[string](payload, objectName, "text")
	if err != nil {
		return nil, err
	}
	meta := decodeOptionalResourceContentJSONValue(payload, "_meta")
	return json.Marshal(struct {
		URI      string     `json:"uri"`
		MimeType *string    `json:"mimeType,omitempty"`
		Text     string     `json:"text"`
		Meta     *JsonValue `json:"_meta,omitempty"`
	}{URI: uri, MimeType: mimeType, Text: text, Meta: meta})
}

func validateBlobResourceContentJSON(data []byte) (json.RawMessage, error) {
	const objectName = "blob resource content"
	payload, err := decodeResourceContentVariantObject(data, objectName, "uri", "mimeType", "blob", "_meta")
	if err != nil {
		return nil, err
	}
	uri, err := decodeRequiredThreadItemValue[string](payload, objectName, "uri")
	if err != nil {
		return nil, err
	}
	mimeType, err := decodeOptionalResourceContentString(payload, objectName, "mimeType")
	if err != nil {
		return nil, err
	}
	blob, err := decodeRequiredThreadItemValue[string](payload, objectName, "blob")
	if err != nil {
		return nil, err
	}
	meta := decodeOptionalResourceContentJSONValue(payload, "_meta")
	return json.Marshal(struct {
		URI      string     `json:"uri"`
		MimeType *string    `json:"mimeType,omitempty"`
		Blob     string     `json:"blob"`
		Meta     *JsonValue `json:"_meta,omitempty"`
	}{URI: uri, MimeType: mimeType, Blob: blob, Meta: meta})
}

func decodeResourceContentVariantObject(
	data []byte,
	objectName string,
	fields ...string,
) (map[string]json.RawMessage, error) {
	known := make(map[string]bool, len(fields))
	for _, field := range fields {
		known[field] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	payload := make(map[string]json.RawMessage, len(fields))
	seen := make(map[string]bool, len(fields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode %s field name: %w", objectName, err)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode %s field %q: %w", objectName, name, err)
		}
		if !known[name] {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate %s field %q", objectName, name)
		}
		seen[name] = true
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%s must contain one JSON value", objectName)
		}
		return nil, fmt.Errorf("decode %s trailing value: %w", objectName, err)
	}
	return payload, nil
}

func decodeOptionalResourceContentString(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func decodeOptionalResourceContentJSONValue(
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
	_ json.Marshaler   = ResourceContent{}
	_ json.Unmarshaler = (*ResourceContent)(nil)
)
