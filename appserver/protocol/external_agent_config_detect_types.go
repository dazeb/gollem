package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	externalAgentConfigDetectIncludeHomeDescription = "If true, include detection under the user's home directory."
	externalAgentConfigDetectCWDsDescription        = "Zero or more working directories to include for repo-scoped detection."
)

// ExternalAgentConfigDetectParams is the exact standalone public request for
// describing detection scope. It does not grant filesystem access or authority.
type ExternalAgentConfigDetectParams struct {
	IncludeHome bool     `json:"includeHome,omitempty"`
	CWDs        []string `json:"cwds"`
}

func (p ExternalAgentConfigDetectParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		IncludeHome bool     `json:"includeHome,omitempty"`
		CWDs        []string `json:"cwds"`
	}{IncludeHome: p.IncludeHome, CWDs: p.CWDs})
}

func (p *ExternalAgentConfigDetectParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode external-agent config detect params into nil receiver")
	}
	const objectName = "external-agent config detect params"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "includeHome", "cwds")
	if err != nil {
		return err
	}
	includeHome, err := decodeOptionalConfigBool(payload, objectName, "includeHome")
	if err != nil {
		return err
	}
	cwds, err := decodeOptionalNullableExternalAgentConfigCWDs(payload, objectName)
	if err != nil {
		return err
	}
	*p = ExternalAgentConfigDetectParams{IncludeHome: includeHome, CWDs: cwds}
	return nil
}

func decodeOptionalNullableExternalAgentConfigCWDs(
	payload map[string]json.RawMessage,
	objectName string,
) ([]string, error) {
	raw, ok := payload["cwds"]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s cwds: %w", objectName, err)
	}
	values := make([]string, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s cwds[%d]: value cannot be null", objectName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s cwds[%d]: %w", objectName, index, err)
		}
	}
	return values, nil
}

// ExternalAgentConfigDetectResponse is the exact standalone public collection
// of migration items. It does not imply that detection has been implemented.
type ExternalAgentConfigDetectResponse struct {
	Items []ExternalAgentConfigMigrationItem `json:"items"`
}

func (r ExternalAgentConfigDetectResponse) MarshalJSON() ([]byte, error) {
	items := r.Items
	if items == nil {
		items = []ExternalAgentConfigMigrationItem{}
	}
	return json.Marshal(struct {
		Items []ExternalAgentConfigMigrationItem `json:"items"`
	}{Items: items})
}

func (r *ExternalAgentConfigDetectResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config detect response into nil receiver")
	}
	const objectName = "external-agent config detect response"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "items")
	if err != nil {
		return err
	}
	items, err := decodeRequiredThreadItemArray[ExternalAgentConfigMigrationItem](payload, objectName, "items")
	if err != nil {
		return err
	}
	*r = ExternalAgentConfigDetectResponse{Items: items}
	return nil
}

func decodeExternalAgentConfigObject(
	data []byte,
	objectName string,
	knownFields ...string,
) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	known := make(map[string]struct{}, len(knownFields))
	for _, name := range knownFields {
		known[name] = struct{}{}
	}
	payload := make(map[string]json.RawMessage, len(known))
	seen := make(map[string]bool, len(known))
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
		if _, ok := known[name]; !ok {
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

func externalAgentConfigDetectParamsSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"includeHome": Schema{
			"type": "boolean", "description": externalAgentConfigDetectIncludeHomeDescription,
		},
		"cwds": Schema{
			"anyOf": []any{
				Schema{"type": "array", "items": Schema{"type": "string"}},
				Schema{"type": "null"},
			},
			"description": externalAgentConfigDetectCWDsDescription,
		},
	}, nil)
}

func externalAgentConfigDetectResponseSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"items": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItem"},
		},
	}, []string{"items"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigDetectParams{}
	_ json.Unmarshaler = (*ExternalAgentConfigDetectParams)(nil)
	_ json.Marshaler   = ExternalAgentConfigDetectResponse{}
	_ json.Unmarshaler = (*ExternalAgentConfigDetectResponse)(nil)
)
