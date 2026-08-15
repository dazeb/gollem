package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ThreadSectionAppearance is the source-defined extensible presentation for a
// server-owned thread section. It remains standalone with the section request
// contracts until the thread-section runtime has an exact compatibility path.
type ThreadSectionAppearance struct {
	Icon  *string `json:"icon"`
	Color *string `json:"color"`
}

func (a *ThreadSectionAppearance) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode thread section appearance into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "thread section appearance", "icon", "color")
	if err != nil {
		return err
	}
	icon, err := decodeOptionalThreadSectionValue[string](payload, "thread section appearance", "icon")
	if err != nil {
		return err
	}
	color, err := decodeOptionalThreadSectionValue[string](payload, "thread section appearance", "color")
	if err != nil {
		return err
	}
	*a = ThreadSectionAppearance{Icon: icon, Color: color}
	return nil
}

// ThreadSectionCreateParams is the exact standalone source contract for
// threadSection/create. It is intentionally not bound to the live runtime.
type ThreadSectionCreateParams struct {
	Name       string                   `json:"name"`
	Appearance *ThreadSectionAppearance `json:"appearance"`
}

func (p *ThreadSectionCreateParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread section create params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "thread section create params", "name", "appearance")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, "thread section create params", "name")
	if err != nil {
		return err
	}
	appearance, err := decodeOptionalThreadSectionValue[ThreadSectionAppearance](payload, "thread section create params", "appearance")
	if err != nil {
		return err
	}
	*p = ThreadSectionCreateParams{Name: name, Appearance: appearance}
	return nil
}

// ThreadSectionDeleteParams is the exact standalone source contract for
// threadSection/delete. It is intentionally not bound to the live runtime.
type ThreadSectionDeleteParams struct {
	SectionID string `json:"sectionId"`
}

func (p *ThreadSectionDeleteParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread section delete params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "thread section delete params", "sectionId")
	if err != nil {
		return err
	}
	sectionID, err := decodeRequiredThreadItemValue[string](payload, "thread section delete params", "sectionId")
	if err != nil {
		return err
	}
	*p = ThreadSectionDeleteParams{SectionID: sectionID}
	return nil
}

// ThreadSectionListParams is the exact standalone source contract for
// threadSection/list. It is intentionally not bound to the live runtime.
type ThreadSectionListParams struct {
	Cursor *string `json:"cursor"`
	Limit  *uint32 `json:"limit"`
}

func (p *ThreadSectionListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread section list params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "thread section list params", "cursor", "limit")
	if err != nil {
		return err
	}
	cursor, err := decodeOptionalThreadSectionValue[string](payload, "thread section list params", "cursor")
	if err != nil {
		return err
	}
	limit, err := decodeOptionalThreadSectionValue[uint32](payload, "thread section list params", "limit")
	if err != nil {
		return err
	}
	*p = ThreadSectionListParams{Cursor: cursor, Limit: limit}
	return nil
}

// ThreadSectionMoveParams is the exact standalone source contract for
// thread/section/move. It is intentionally not bound to the live runtime.
type ThreadSectionMoveParams struct {
	ThreadID       string  `json:"threadId"`
	SectionID      *string `json:"sectionId"`
	BeforeThreadID *string `json:"beforeThreadId"`
}

func (p *ThreadSectionMoveParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread section move params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "thread section move params", "threadId", "sectionId", "beforeThreadId")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "thread section move params", "threadId")
	if err != nil {
		return err
	}
	sectionID, err := decodeRequiredNullableThreadSectionValue[string](payload, "thread section move params", "sectionId")
	if err != nil {
		return err
	}
	beforeThreadID, err := decodeOptionalThreadSectionValue[string](payload, "thread section move params", "beforeThreadId")
	if err != nil {
		return err
	}
	*p = ThreadSectionMoveParams{ThreadID: threadID, SectionID: sectionID, BeforeThreadID: beforeThreadID}
	return nil
}

// ThreadSectionUpdateParams is the exact standalone source contract for
// threadSection/update. Appearance preserves the source's omission versus
// explicit-null distinction.
type ThreadSectionUpdateParams struct {
	SectionID  string                   `json:"sectionId"`
	Name       string                   `json:"name"`
	Appearance *ThreadSectionAppearance `json:"appearance,omitempty"`

	appearancePresent bool
}

func (p *ThreadSectionUpdateParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread section update params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "thread section update params", "sectionId", "name", "appearance")
	if err != nil {
		return err
	}
	sectionID, err := decodeRequiredThreadItemValue[string](payload, "thread section update params", "sectionId")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, "thread section update params", "name")
	if err != nil {
		return err
	}
	appearance, err := decodeOptionalThreadSectionValue[ThreadSectionAppearance](payload, "thread section update params", "appearance")
	if err != nil {
		return err
	}
	_, appearancePresent := payload["appearance"]
	*p = ThreadSectionUpdateParams{
		SectionID: sectionID, Name: name, Appearance: appearance, appearancePresent: appearancePresent,
	}
	return nil
}

func (p ThreadSectionUpdateParams) MarshalJSON() ([]byte, error) {
	if !p.HasAppearance() {
		return json.Marshal(struct {
			SectionID string `json:"sectionId"`
			Name      string `json:"name"`
		}{SectionID: p.SectionID, Name: p.Name})
	}
	return json.Marshal(struct {
		SectionID  string                   `json:"sectionId"`
		Name       string                   `json:"name"`
		Appearance *ThreadSectionAppearance `json:"appearance"`
	}{SectionID: p.SectionID, Name: p.Name, Appearance: p.Appearance})
}

// HasAppearance reports whether an update carries either a replacement or an
// explicit null that clears the source appearance.
func (p ThreadSectionUpdateParams) HasAppearance() bool {
	return p.appearancePresent || p.Appearance != nil
}

// SetAppearance makes an update carry value. A nil value is the source's
// explicit-null clear operation; leaving it unset preserves the appearance.
func (p *ThreadSectionUpdateParams) SetAppearance(value *ThreadSectionAppearance) {
	p.Appearance = value
	p.appearancePresent = true
}

func decodeOptionalThreadSectionValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func decodeRequiredNullableThreadSectionValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
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

func threadSectionParamSchemas() map[string]Schema {
	appearance := Schema{
		"description": "Extensible visual presentation for a custom thread section.",
		"properties": Schema{
			"color": Schema{"type": []any{"string", "null"}},
			"icon":  Schema{"type": []any{"string", "null"}},
		},
		"type": "object",
	}
	appearanceOrNull := Schema{"anyOf": []any{
		Schema{"$ref": "#/$defs/ThreadSectionAppearance"},
		Schema{"type": "null"},
	}}
	return map[string]Schema{
		"ThreadSectionAppearance": appearance,
		"ThreadSectionCreateParams": {
			"description": "Parameters for creating an independently persisted thread section.",
			"properties": Schema{
				"appearance": Schema{"anyOf": appearanceOrNull["anyOf"], "default": nil},
				"name": Schema{
					"description": "The user-visible name of the section.",
					"type":        "string",
				},
			},
			"required": []string{"name"},
			"type":     "object",
		},
		"ThreadSectionDeleteParams": {
			"description": "Parameters for deleting an independently persisted thread section.",
			"properties": Schema{
				"sectionId": Schema{
					"description": "The stable, server-generated identity of the section to delete.",
					"type":        "string",
				},
			},
			"required": []string{"sectionId"},
			"type":     "object",
		},
		"ThreadSectionListParams": {
			"description": "Parameters for listing independently persisted thread sections.",
			"properties": Schema{
				"cursor": Schema{
					"description": "Opaque pagination cursor returned by a previous call.",
					"type":        []any{"string", "null"},
				},
				"limit": Schema{
					"description": "Maximum number of sections to return.",
					"format":      "uint32",
					"minimum":     json.Number("0.0"),
					"type":        []any{"integer", "null"},
				},
			},
			"type": "object",
		},
		"ThreadSectionMoveParams": {
			"description": "Parameters for moving a thread within a server-owned section ordering.",
			"properties": Schema{
				"beforeThreadId": Schema{
					"description": "Existing thread to insert before; omission or null appends to the section.",
					"type":        []any{"string", "null"},
				},
				"sectionId": Schema{
					"description": "Destination section, or `null` to remove the thread from its section.",
					"type":        []any{"string", "null"},
				},
				"threadId": Schema{
					"description": "Thread to move into, within, or out of a section.",
					"type":        "string",
				},
			},
			"required": []string{"sectionId", "threadId"},
			"type":     "object",
		},
		"ThreadSectionUpdateParams": {
			"description": "Parameters for updating an independently persisted thread section.",
			"properties": Schema{
				"appearance": Schema{
					"anyOf":       appearanceOrNull["anyOf"],
					"description": "Omit to preserve appearance, use `null` to clear it, or provide a replacement.",
				},
				"name": Schema{
					"description": "The updated user-visible name of the section.",
					"type":        "string",
				},
				"sectionId": Schema{
					"description": "The stable, server-generated identity of the section to update.",
					"type":        "string",
				},
			},
			"required": []string{"name", "sectionId"},
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*ThreadSectionAppearance)(nil)
	_ json.Unmarshaler = (*ThreadSectionCreateParams)(nil)
	_ json.Unmarshaler = (*ThreadSectionDeleteParams)(nil)
	_ json.Unmarshaler = (*ThreadSectionListParams)(nil)
	_ json.Unmarshaler = (*ThreadSectionMoveParams)(nil)
	_ json.Marshaler   = ThreadSectionUpdateParams{}
	_ json.Unmarshaler = (*ThreadSectionUpdateParams)(nil)
)
