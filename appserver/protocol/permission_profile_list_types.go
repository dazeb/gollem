package protocol

import (
	"encoding/json"
	"errors"
)

// PermissionProfileListParams is the exact public permission-profile page
// selector. It remains separate from Gollem's broader live catalog request.
type PermissionProfileListParams struct {
	Cursor *string `json:"cursor"`
	Limit  *uint32 `json:"limit"`
	CWD    *string `json:"cwd"`
}

func (p PermissionProfileListParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Cursor *string `json:"cursor"`
		Limit  *uint32 `json:"limit"`
		CWD    *string `json:"cwd"`
	}{Cursor: p.Cursor, Limit: p.Limit, CWD: p.CWD})
}

func (p *PermissionProfileListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode permission-profile list params into nil receiver")
	}
	const objectName = "permission-profile list params"
	payload, err := decodeRustSerdeObject(data, objectName, "cursor", "limit", "cwd")
	if err != nil {
		return err
	}
	cursor, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cursor")
	if err != nil {
		return err
	}
	limit, err := decodeOptionalNullableConfigValue[uint32](payload, objectName, "limit")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	*p = PermissionProfileListParams{Cursor: cursor, Limit: limit, CWD: cwd}
	return nil
}

// PermissionProfileSummary is exact public display and selectability data. It
// does not grant permissions or prove that a profile is enforced.
type PermissionProfileSummary struct {
	ID          string  `json:"id"`
	Description *string `json:"description"`
	Allowed     bool    `json:"allowed"`
}

func (s PermissionProfileSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ID          string  `json:"id"`
		Description *string `json:"description"`
		Allowed     bool    `json:"allowed"`
	}{ID: s.ID, Description: s.Description, Allowed: s.Allowed})
}

func (s *PermissionProfileSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode permission-profile summary into nil receiver")
	}
	const objectName = "permission-profile summary"
	payload, err := decodeRustSerdeObject(data, objectName, "id", "description", "allowed")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableConfigValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	allowed, err := decodeRequiredThreadItemValue[bool](payload, objectName, "allowed")
	if err != nil {
		return err
	}
	*s = PermissionProfileSummary{ID: id, Description: description, Allowed: allowed}
	return nil
}

// PermissionProfileListResponse is one exact public ordered profile-summary
// page. The records remain descriptive and carry no permission authority.
type PermissionProfileListResponse struct {
	Data       []PermissionProfileSummary `json:"data"`
	NextCursor *string                    `json:"nextCursor"`
}

func (r PermissionProfileListResponse) MarshalJSON() ([]byte, error) {
	if r.Data == nil {
		return nil, errors.New("permission-profile list response data cannot be null")
	}
	return json.Marshal(struct {
		Data       []PermissionProfileSummary `json:"data"`
		NextCursor *string                    `json:"nextCursor"`
	}{Data: r.Data, NextCursor: r.NextCursor})
}

func (r *PermissionProfileListResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode permission-profile list response into nil receiver")
	}
	const objectName = "permission-profile list response"
	payload, err := decodeRustSerdeObject(data, objectName, "data", "nextCursor")
	if err != nil {
		return err
	}
	profiles, err := decodeRequiredThreadItemArray[PermissionProfileSummary](
		payload, objectName, "data",
	)
	if err != nil {
		return err
	}
	nextCursor, err := decodeOptionalNullableConfigValue[string](payload, objectName, "nextCursor")
	if err != nil {
		return err
	}
	*r = PermissionProfileListResponse{Data: profiles, NextCursor: nextCursor}
	return nil
}

func permissionProfileListSchemas() map[string]Schema {
	return map[string]Schema{
		"PermissionProfileListParams": {
			"type": "object",
			"properties": Schema{
				"cursor": Schema{
					"description": "Opaque pagination cursor returned by a previous call.",
					"type":        []any{"string", "null"},
				},
				"cwd": Schema{
					"description": "Optional working directory to resolve project config layers.",
					"type":        []any{"string", "null"},
				},
				"limit": Schema{
					"description": "Optional page size; defaults to the full result set.",
					"format":      "uint32",
					"minimum":     float64(0),
					"type":        []any{"integer", "null"},
				},
			},
		},
		"PermissionProfileSummary": {
			"type": "object",
			"properties": Schema{
				"allowed": Schema{
					"description": "Whether the effective requirements allow selecting this profile.",
					"type":        "boolean",
				},
				"description": Schema{
					"description": "Optional user-facing description for display in clients.",
					"type":        []any{"string", "null"},
				},
				"id": Schema{
					"description": "Available permission profile identifier.",
					"type":        "string",
				},
			},
			"required": []string{"allowed", "id"},
		},
		"PermissionProfileListResponse": {
			"type": "object",
			"properties": Schema{
				"data": Schema{
					"items": Schema{"$ref": "#/$defs/PermissionProfileSummary"},
					"type":  "array",
				},
				"nextCursor": Schema{
					"description": "Opaque cursor to pass to the next call to continue after the last item. If None, there are no more items to return.",
					"type":        []any{"string", "null"},
				},
			},
			"required": []string{"data"},
		},
	}
}

var (
	_ json.Marshaler   = PermissionProfileListParams{}
	_ json.Unmarshaler = (*PermissionProfileListParams)(nil)
	_ json.Marshaler   = PermissionProfileSummary{}
	_ json.Unmarshaler = (*PermissionProfileSummary)(nil)
	_ json.Marshaler   = PermissionProfileListResponse{}
	_ json.Unmarshaler = (*PermissionProfileListResponse)(nil)
)
