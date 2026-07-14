package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ModelListParams is the exact public model-catalog page selector. It remains
// separate from the provider-filtered live request until an adapter is defined.
type ModelListParams struct {
	Cursor        *string `json:"cursor,omitempty"`
	Limit         *uint32 `json:"limit,omitempty"`
	IncludeHidden *bool   `json:"includeHidden,omitempty"`
}

func (p ModelListParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		Cursor        *string `json:"cursor"`
		Limit         *uint32 `json:"limit"`
		IncludeHidden *bool   `json:"includeHidden"`
	}
	return json.Marshal(wire(p))
}

func (p *ModelListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode model-list params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model-list params",
		"cursor",
		"limit",
		"includeHidden",
	)
	if err != nil {
		return err
	}
	cursor, err := decodeOptionalNullableModelListValue[string](payload, "cursor")
	if err != nil {
		return err
	}
	limit, err := decodeOptionalNullableModelListValue[uint32](payload, "limit")
	if err != nil {
		return err
	}
	includeHidden, err := decodeOptionalNullableModelListValue[bool](payload, "includeHidden")
	if err != nil {
		return err
	}
	*p = ModelListParams{Cursor: cursor, Limit: limit, IncludeHidden: includeHidden}
	return nil
}

// ModelListResponse is the exact public page of strict Model values. It stays
// outside the live method binding until provider extensions can be projected.
type ModelListResponse struct {
	Data       []Model `json:"data" jsonschema:"nonnullable=true"`
	NextCursor *string `json:"nextCursor"`
}

func (r ModelListResponse) MarshalJSON() ([]byte, error) {
	if r.Data == nil {
		return nil, errors.New("model-list response data cannot be null")
	}
	type wire ModelListResponse
	encoded, err := json.Marshal(wire(r))
	if err != nil {
		return nil, fmt.Errorf("encode model-list response: %w", err)
	}
	return encoded, nil
}

func (r *ModelListResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode model-list response into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model-list response",
		"data",
		"nextCursor",
	)
	if err != nil {
		return err
	}
	models, err := decodeRequiredThreadItemArray[Model](payload, "model-list response", "data")
	if err != nil {
		return err
	}
	nextCursor, err := decodeOptionalNullableModelListValue[string](payload, "nextCursor")
	if err != nil {
		return err
	}
	*r = ModelListResponse{Data: models, NextCursor: nextCursor}
	return nil
}

func decodeOptionalNullableModelListValue[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode model-list %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = ModelListParams{}
	_ json.Unmarshaler = (*ModelListParams)(nil)
	_ json.Marshaler   = ModelListResponse{}
	_ json.Unmarshaler = (*ModelListResponse)(nil)
)
