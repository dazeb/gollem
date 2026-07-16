package protocol

import (
	"encoding/json"
	"errors"
)

// AppsListParams is the exact public app-catalog page selector. It remains
// separate from the deferred app/list method until a live adapter is defined.
type AppsListParams struct {
	Cursor       *string `json:"cursor,omitempty"`
	ForceRefetch bool    `json:"forceRefetch,omitempty"`
	Limit        *uint32 `json:"limit,omitempty"`
	ThreadID     *string `json:"threadId,omitempty"`
}

func (p AppsListParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		Cursor       *string `json:"cursor"`
		Limit        *uint32 `json:"limit"`
		ThreadID     *string `json:"threadId"`
		ForceRefetch bool    `json:"forceRefetch,omitempty"`
	}
	return json.Marshal(wire{
		Cursor: p.Cursor, Limit: p.Limit, ThreadID: p.ThreadID, ForceRefetch: p.ForceRefetch,
	})
}

func (p *AppsListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode apps-list params into nil receiver")
	}
	const objectName = "apps-list params"
	payload, err := decodeRustSerdeObject(
		data, objectName, "cursor", "forceRefetch", "limit", "threadId",
	)
	if err != nil {
		return err
	}
	cursor, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cursor")
	if err != nil {
		return err
	}
	forceRefetch, err := decodeOptionalConfigBool(payload, objectName, "forceRefetch")
	if err != nil {
		return err
	}
	limit, err := decodeOptionalNullableConfigValue[uint32](payload, objectName, "limit")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	*p = AppsListParams{
		Cursor: cursor, ForceRefetch: forceRefetch, Limit: limit, ThreadID: threadID,
	}
	return nil
}

// AppsListResponse is the exact public page of strict AppInfo values. It stays
// outside the deferred app/list method binding until that method is implemented.
type AppsListResponse struct {
	Data       []AppInfo `json:"data" jsonschema:"nonnullable=true"`
	NextCursor *string   `json:"nextCursor"`
}

func (r AppsListResponse) MarshalJSON() ([]byte, error) {
	if r.Data == nil {
		return nil, errors.New("apps-list response data cannot be null")
	}
	type wire AppsListResponse
	return json.Marshal(wire(r))
}

func (r *AppsListResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode apps-list response into nil receiver")
	}
	const objectName = "apps-list response"
	payload, err := decodeRustSerdeObject(data, objectName, "data", "nextCursor")
	if err != nil {
		return err
	}
	apps, err := decodeRequiredThreadItemArray[AppInfo](payload, objectName, "data")
	if err != nil {
		return err
	}
	nextCursor, err := decodeOptionalNullableConfigValue[string](payload, objectName, "nextCursor")
	if err != nil {
		return err
	}
	*r = AppsListResponse{Data: apps, NextCursor: nextCursor}
	return nil
}

var (
	_ json.Marshaler   = AppsListParams{}
	_ json.Unmarshaler = (*AppsListParams)(nil)
	_ json.Marshaler   = AppsListResponse{}
	_ json.Unmarshaler = (*AppsListResponse)(nil)
)
