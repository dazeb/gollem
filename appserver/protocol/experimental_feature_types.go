package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ExperimentalFeatureStage is the exact closed public feature lifecycle.
type ExperimentalFeatureStage string

const (
	ExperimentalFeatureStageBeta             ExperimentalFeatureStage = "beta"
	ExperimentalFeatureStageUnderDevelopment ExperimentalFeatureStage = "underDevelopment"
	ExperimentalFeatureStageStable           ExperimentalFeatureStage = "stable"
	ExperimentalFeatureStageDeprecated       ExperimentalFeatureStage = "deprecated"
	ExperimentalFeatureStageRemoved          ExperimentalFeatureStage = "removed"
)

func (s ExperimentalFeatureStage) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "experimental feature stage", ExperimentalFeatureStage.valid)
}

func (s *ExperimentalFeatureStage) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "experimental feature stage", ExperimentalFeatureStage.valid)
}

func (s ExperimentalFeatureStage) valid() bool {
	return s == ExperimentalFeatureStageBeta ||
		s == ExperimentalFeatureStageUnderDevelopment ||
		s == ExperimentalFeatureStageStable ||
		s == ExperimentalFeatureStageDeprecated ||
		s == ExperimentalFeatureStageRemoved
}

// ExperimentalFeature is exact standalone feature metadata.
type ExperimentalFeature struct {
	Name           string                   `json:"name"`
	Stage          ExperimentalFeatureStage `json:"stage"`
	DisplayName    *string                  `json:"displayName,omitempty"`
	Description    *string                  `json:"description,omitempty"`
	Announcement   *string                  `json:"announcement,omitempty"`
	Enabled        bool                     `json:"enabled"`
	DefaultEnabled bool                     `json:"defaultEnabled"`
}

func (f ExperimentalFeature) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name           string                   `json:"name"`
		Stage          ExperimentalFeatureStage `json:"stage"`
		DisplayName    *string                  `json:"displayName"`
		Description    *string                  `json:"description"`
		Announcement   *string                  `json:"announcement"`
		Enabled        bool                     `json:"enabled"`
		DefaultEnabled bool                     `json:"defaultEnabled"`
	}{
		Name: f.Name, Stage: f.Stage, DisplayName: f.DisplayName,
		Description: f.Description, Announcement: f.Announcement,
		Enabled: f.Enabled, DefaultEnabled: f.DefaultEnabled,
	})
}

func (f *ExperimentalFeature) UnmarshalJSON(data []byte) error {
	if f == nil {
		return errors.New("decode experimental feature into nil receiver")
	}
	const objectName = "experimental feature"
	payload, err := decodeRustSerdeObject(
		data, objectName, "name", "stage", "displayName", "description", "announcement", "enabled", "defaultEnabled",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	stage, err := decodeRequiredThreadItemValue[ExperimentalFeatureStage](payload, objectName, "stage")
	if err != nil {
		return err
	}
	displayName, err := decodeOptionalNullableConfigValue[string](payload, objectName, "displayName")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableConfigValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	announcement, err := decodeOptionalNullableConfigValue[string](payload, objectName, "announcement")
	if err != nil {
		return err
	}
	enabled, err := decodeRequiredThreadItemValue[bool](payload, objectName, "enabled")
	if err != nil {
		return err
	}
	defaultEnabled, err := decodeRequiredThreadItemValue[bool](payload, objectName, "defaultEnabled")
	if err != nil {
		return err
	}
	*f = ExperimentalFeature{
		Name: name, Stage: stage, DisplayName: displayName, Description: description,
		Announcement: announcement, Enabled: enabled, DefaultEnabled: defaultEnabled,
	}
	return nil
}

// ExperimentalFeatureListParams is exact optional pagination/thread context.
type ExperimentalFeatureListParams struct {
	Cursor   *string `json:"cursor,omitempty"`
	Limit    *uint32 `json:"limit,omitempty"`
	ThreadID *string `json:"threadId,omitempty"`
}

func (p *ExperimentalFeatureListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode experimental feature list params into nil receiver")
	}
	const objectName = "experimental feature list params"
	payload, err := decodeRustSerdeObject(data, objectName, "cursor", "limit", "threadId")
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
	threadID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	*p = ExperimentalFeatureListParams{Cursor: cursor, Limit: limit, ThreadID: threadID}
	return nil
}

// ExperimentalFeatureListResponse is exact standalone paginated feature data.
type ExperimentalFeatureListResponse struct {
	Data       []ExperimentalFeature `json:"data"`
	NextCursor *string               `json:"nextCursor,omitempty"`
}

func (r ExperimentalFeatureListResponse) MarshalJSON() ([]byte, error) {
	if r.Data == nil {
		return nil, errors.New("experimental feature list response data cannot be null")
	}
	return json.Marshal(struct {
		Data       []ExperimentalFeature `json:"data"`
		NextCursor *string               `json:"nextCursor"`
	}{Data: r.Data, NextCursor: r.NextCursor})
}

func (r *ExperimentalFeatureListResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode experimental feature list response into nil receiver")
	}
	const objectName = "experimental feature list response"
	payload, err := decodeRustSerdeObject(data, objectName, "data", "nextCursor")
	if err != nil {
		return err
	}
	values, err := decodeRequiredThreadItemArray[ExperimentalFeature](payload, objectName, "data")
	if err != nil {
		return err
	}
	nextCursor, err := decodeOptionalNullableConfigValue[string](payload, objectName, "nextCursor")
	if err != nil {
		return err
	}
	*r = ExperimentalFeatureListResponse{Data: values, NextCursor: nextCursor}
	return nil
}

// ExperimentalFeatureEnablementSetParams is exact standalone enablement input.
type ExperimentalFeatureEnablementSetParams struct {
	Enablement map[string]bool `json:"enablement"`
}

func (p ExperimentalFeatureEnablementSetParams) MarshalJSON() ([]byte, error) {
	return marshalExperimentalFeatureEnablement("params", p.Enablement)
}

func (p *ExperimentalFeatureEnablementSetParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode experimental feature enablement params into nil receiver")
	}
	enablement, err := unmarshalExperimentalFeatureEnablement(data, "experimental feature enablement params")
	if err != nil {
		return err
	}
	*p = ExperimentalFeatureEnablementSetParams{Enablement: enablement}
	return nil
}

// ExperimentalFeatureEnablementSetResponse is exact standalone enablement output.
type ExperimentalFeatureEnablementSetResponse struct {
	Enablement map[string]bool `json:"enablement"`
}

func (r ExperimentalFeatureEnablementSetResponse) MarshalJSON() ([]byte, error) {
	return marshalExperimentalFeatureEnablement("response", r.Enablement)
}

func (r *ExperimentalFeatureEnablementSetResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode experimental feature enablement response into nil receiver")
	}
	enablement, err := unmarshalExperimentalFeatureEnablement(data, "experimental feature enablement response")
	if err != nil {
		return err
	}
	*r = ExperimentalFeatureEnablementSetResponse{Enablement: enablement}
	return nil
}

func marshalExperimentalFeatureEnablement(kind string, enablement map[string]bool) ([]byte, error) {
	if enablement == nil {
		return nil, fmt.Errorf("experimental feature enablement %s map cannot be null", kind)
	}
	return json.Marshal(struct {
		Enablement map[string]bool `json:"enablement"`
	}{Enablement: enablement})
}

func unmarshalExperimentalFeatureEnablement(data []byte, objectName string) (map[string]bool, error) {
	payload, err := decodeRustSerdeObject(data, objectName, "enablement")
	if err != nil {
		return nil, err
	}
	enablement, err := decodeRequiredThreadItemValue[map[string]bool](payload, objectName, "enablement")
	if err != nil {
		return nil, err
	}
	return enablement, nil
}

func experimentalFeatureSchemas() map[string]Schema {
	stage := func(description, value string) Schema {
		return Schema{"description": description, "enum": []any{value}, "type": "string"}
	}
	mapSchema := func(description string) Schema {
		return Schema{
			"type": "object",
			"properties": Schema{"enablement": Schema{
				"additionalProperties": Schema{"type": "boolean"},
				"description":          description,
				"type":                 "object",
			}},
			"required": []string{"enablement"},
		}
	}
	return map[string]Schema{
		"ExperimentalFeatureStage": {"oneOf": []any{
			stage("Feature is available for user testing and feedback.", "beta"),
			stage("Feature is still being built and not ready for broad use.", "underDevelopment"),
			stage("Feature is production-ready.", "stable"),
			stage("Feature is deprecated and should be avoided.", "deprecated"),
			stage("Feature flag is retained only for backwards compatibility.", "removed"),
		}},
		"ExperimentalFeature": {
			"type": "object",
			"properties": Schema{
				"announcement":   Schema{"description": "Announcement copy shown to users when the feature is introduced. Null when this feature is not in beta.", "type": []any{"string", "null"}},
				"defaultEnabled": Schema{"description": "Whether this feature is enabled by default.", "type": "boolean"},
				"description":    Schema{"description": "Short summary describing what the feature does. Null when this feature is not in beta.", "type": []any{"string", "null"}},
				"displayName":    Schema{"description": "User-facing display name shown in the experimental features UI. Null when this feature is not in beta.", "type": []any{"string", "null"}},
				"enabled":        Schema{"description": "Whether this feature is currently enabled in the loaded config.", "type": "boolean"},
				"name":           Schema{"description": "Stable key used in config.toml and CLI flag toggles.", "type": "string"},
				"stage":          Schema{"allOf": []any{Schema{"$ref": "#/$defs/ExperimentalFeatureStage"}}, "description": "Lifecycle stage of this feature flag."},
			},
			"required": []string{"defaultEnabled", "enabled", "name", "stage"},
		},
		"ExperimentalFeatureListParams": {
			"type": "object",
			"properties": Schema{
				"cursor":   Schema{"description": "Opaque pagination cursor returned by a previous call.", "type": []any{"string", "null"}},
				"limit":    Schema{"description": "Optional page size; defaults to a reasonable server-side value.", "format": "uint32", "minimum": float64(0), "type": []any{"integer", "null"}},
				"threadId": Schema{"description": "Optional loaded thread id. Pass this when showing feature state for an existing thread so enablement is computed from that thread's refreshed config, including project-local config for the thread's cwd.", "type": []any{"string", "null"}},
			},
		},
		"ExperimentalFeatureListResponse": {
			"type": "object",
			"properties": Schema{
				"data":       Schema{"items": Schema{"$ref": "#/$defs/ExperimentalFeature"}, "type": "array"},
				"nextCursor": Schema{"description": "Opaque cursor to pass to the next call to continue after the last item. If None, there are no more items to return.", "type": []any{"string", "null"}},
			},
			"required": []string{"data"},
		},
		"ExperimentalFeatureEnablementSetParams":   mapSchema("Process-wide runtime feature enablement keyed by canonical feature name.\n\nOnly named features are updated. Omitted features are left unchanged. Send an empty map for a no-op."),
		"ExperimentalFeatureEnablementSetResponse": mapSchema("Feature enablement entries updated by this request."),
	}
}

var (
	_ json.Marshaler   = ExperimentalFeatureStage("")
	_ json.Unmarshaler = (*ExperimentalFeatureStage)(nil)
	_ json.Marshaler   = ExperimentalFeature{}
	_ json.Unmarshaler = (*ExperimentalFeature)(nil)
	_ json.Unmarshaler = (*ExperimentalFeatureListParams)(nil)
	_ json.Marshaler   = ExperimentalFeatureListResponse{}
	_ json.Unmarshaler = (*ExperimentalFeatureListResponse)(nil)
	_ json.Marshaler   = ExperimentalFeatureEnablementSetParams{}
	_ json.Unmarshaler = (*ExperimentalFeatureEnablementSetParams)(nil)
	_ json.Marshaler   = ExperimentalFeatureEnablementSetResponse{}
	_ json.Unmarshaler = (*ExperimentalFeatureEnablementSetResponse)(nil)
)
