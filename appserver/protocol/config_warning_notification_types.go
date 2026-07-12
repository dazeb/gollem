package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// TextPosition is the exact fixed public config-diagnostic position value.
// Coordinates are documented as 1-based, but zero remains valid on the wire.
type TextPosition struct {
	Line   uint64 `json:"line"`
	Column uint64 `json:"column"`
}

func (p *TextPosition) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode text position into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "text position", "line", "column")
	if err != nil {
		return err
	}
	line, err := decodeRequiredThreadItemValue[uint64](payload, "text position", "line")
	if err != nil {
		return err
	}
	column, err := decodeRequiredThreadItemValue[uint64](payload, "text position", "column")
	if err != nil {
		return err
	}
	*p = TextPosition{Line: line, Column: column}
	return nil
}

// TextRange is the exact fixed public config-diagnostic range value.
type TextRange struct {
	Start TextPosition `json:"start"`
	End   TextPosition `json:"end"`
}

func (r *TextRange) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode text range into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "text range", "start", "end")
	if err != nil {
		return err
	}
	start, err := decodeRequiredThreadItemValue[TextPosition](payload, "text range", "start")
	if err != nil {
		return err
	}
	end, err := decodeRequiredThreadItemValue[TextPosition](payload, "text range", "end")
	if err != nil {
		return err
	}
	*r = TextRange{Start: start, End: end}
	return nil
}

// ConfigWarningNotification is the exact fixed public config warning value. It
// remains standalone until Gollem has a config-diagnostic producer.
type ConfigWarningNotification struct {
	Summary string     `json:"summary"`
	Details *string    `json:"details"`
	Path    *string    `json:"path,omitempty" jsonschema:"nonnullable=true"`
	Range   *TextRange `json:"range,omitempty" jsonschema:"nonnullable=true"`
}

func (n *ConfigWarningNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode config warning notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"config warning notification",
		"summary",
		"details",
		"path",
		"range",
	)
	if err != nil {
		return err
	}
	summary, err := decodeRequiredThreadItemValue[string](payload, "config warning notification", "summary")
	if err != nil {
		return err
	}
	details, err := decodeOptionalNullableConfigWarningString(payload, "details")
	if err != nil {
		return err
	}
	path, err := decodeOptionalNonNullConfigWarningValue[string](payload, "path")
	if err != nil {
		return err
	}
	textRange, err := decodeOptionalNonNullConfigWarningValue[TextRange](payload, "range")
	if err != nil {
		return err
	}
	*n = ConfigWarningNotification{
		Summary: summary, Details: details, Path: path, Range: textRange,
	}
	return nil
}

func decodeOptionalNullableConfigWarningString(
	payload map[string]json.RawMessage,
	fieldName string,
) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode config warning notification %s: %w", fieldName, err)
	}
	return &value, nil
}

func decodeOptionalNonNullConfigWarningValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return nil, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("config warning notification %s cannot be null", fieldName)
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode config warning notification %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Unmarshaler = (*TextPosition)(nil)
	_ json.Unmarshaler = (*TextRange)(nil)
	_ json.Unmarshaler = (*ConfigWarningNotification)(nil)
)
