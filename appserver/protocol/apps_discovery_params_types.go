package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AppsInstalledParams is the exact standalone source contract for
// app/installed. It intentionally does not bind Gollem's runtime discovery.
type AppsInstalledParams struct {
	ThreadID     *string `json:"threadId"`
	ForceRefresh bool    `json:"forceRefresh,omitempty"`
}

func (p *AppsInstalledParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode apps installed params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "apps installed params", "threadId", "forceRefresh")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalAppsDiscoveryValue[string](payload, "apps installed params", "threadId")
	if err != nil {
		return err
	}
	forceRefresh, err := decodeOptionalAppsDiscoveryBool(payload, "apps installed params", "forceRefresh")
	if err != nil {
		return err
	}
	*p = AppsInstalledParams{ThreadID: threadID, ForceRefresh: forceRefresh}
	return nil
}

// AppsReadParams is the exact standalone source contract for app/read. It
// intentionally does not bind Gollem's runtime discovery.
type AppsReadParams struct {
	AppIDs       []string `json:"appIds"`
	ThreadID     *string  `json:"threadId"`
	IncludeTools bool     `json:"includeTools,omitempty"`
}

func (p *AppsReadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode apps read params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "apps read params", "appIds", "threadId", "includeTools")
	if err != nil {
		return err
	}
	appIDs, err := decodeRequiredThreadItemValue[[]string](payload, "apps read params", "appIds")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalAppsDiscoveryValue[string](payload, "apps read params", "threadId")
	if err != nil {
		return err
	}
	includeTools, err := decodeOptionalAppsDiscoveryBool(payload, "apps read params", "includeTools")
	if err != nil {
		return err
	}
	*p = AppsReadParams{AppIDs: appIDs, ThreadID: threadID, IncludeTools: includeTools}
	return nil
}

func decodeOptionalAppsDiscoveryValue[T any](
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

func decodeOptionalAppsDiscoveryBool(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (bool, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return false, nil
	}
	if isJSONNull(raw) {
		return false, fmt.Errorf("decode %s %s: value cannot be null", objectName, fieldName)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func appsDiscoveryParamSchemas() map[string]Schema {
	threadID := Schema{
		"description": "Optional loaded thread id used to evaluate effective app configuration.",
		"type":        []any{"string", "null"},
	}
	return map[string]Schema{
		"AppsInstalledParams": {
			"description": "Read the committed installed connector runtime snapshot.",
			"properties": Schema{
				"forceRefresh": Schema{
					"description": "When true and Apps are permitted, refresh and publish the hosted connector runtime tool snapshot first.",
					"type":        "boolean",
				},
				"threadId": threadID,
			},
			"type": "object",
		},
		"AppsReadParams": {
			"description": "EXPERIMENTAL - read metadata for specific apps/connectors.",
			"properties": Schema{
				"appIds": Schema{
					"description": "App ids to read. The server accepts at most 100 ids and deduplicates repeated ids while preserving their first-request order.",
					"items":       Schema{"type": "string"},
					"type":        "array",
				},
				"includeTools": Schema{
					"description": "When true, include display-only public tool summaries in the returned metadata.",
					"type":        "boolean",
				},
				"threadId": threadID,
			},
			"required": []string{"appIds"},
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*AppsInstalledParams)(nil)
	_ json.Unmarshaler = (*AppsReadParams)(nil)
)
