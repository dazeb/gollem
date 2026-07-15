package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

const externalAgentConfigImportSourceDescription = "Source product that produced the migration items. Missing means unspecified."

// ExternalAgentConfigImportParams is the exact standalone public import
// description. It does not grant path access or authorize import execution.
type ExternalAgentConfigImportParams struct {
	MigrationItems []ExternalAgentConfigMigrationItem `json:"migrationItems"`
	Source         *string                            `json:"source"`
}

func (p ExternalAgentConfigImportParams) MarshalJSON() ([]byte, error) {
	items := p.MigrationItems
	if items == nil {
		items = []ExternalAgentConfigMigrationItem{}
	}
	return json.Marshal(struct {
		MigrationItems []ExternalAgentConfigMigrationItem `json:"migrationItems"`
		Source         *string                            `json:"source"`
	}{MigrationItems: items, Source: p.Source})
}

func (p *ExternalAgentConfigImportParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode external-agent config import params into nil receiver")
	}
	const objectName = "external-agent config import params"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "migrationItems", "source")
	if err != nil {
		return err
	}
	items, err := decodeRequiredThreadItemArray[ExternalAgentConfigMigrationItem](
		payload, objectName, "migrationItems",
	)
	if err != nil {
		return err
	}
	source, err := decodeOptionalNullableExternalAgentConfigImportSource(payload, objectName)
	if err != nil {
		return err
	}
	*p = ExternalAgentConfigImportParams{MigrationItems: items, Source: source}
	return nil
}

func decodeOptionalNullableExternalAgentConfigImportSource(
	payload map[string]json.RawMessage,
	objectName string,
) (*string, error) {
	raw, ok := payload["source"]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var source string
	if err := json.Unmarshal(raw, &source); err != nil {
		return nil, fmt.Errorf("decode %s source: %w", objectName, err)
	}
	return &source, nil
}

// ExternalAgentConfigImportResponse is the exact standalone public import ID.
// It does not imply that an import has run or persisted any configuration.
type ExternalAgentConfigImportResponse struct {
	ImportID string `json:"importId"`
}

func (r ExternalAgentConfigImportResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ImportID string `json:"importId"`
	}{ImportID: r.ImportID})
}

func (r *ExternalAgentConfigImportResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import response into nil receiver")
	}
	const objectName = "external-agent config import response"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "importId")
	if err != nil {
		return err
	}
	importID, err := decodeRequiredThreadItemValue[string](payload, objectName, "importId")
	if err != nil {
		return err
	}
	*r = ExternalAgentConfigImportResponse{ImportID: importID}
	return nil
}

func externalAgentConfigImportParamsSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"migrationItems": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItem"},
		},
		"source": Schema{
			"anyOf":       []any{Schema{"type": "string"}, Schema{"type": "null"}},
			"description": externalAgentConfigImportSourceDescription,
		},
	}, []string{"migrationItems"})
}

func externalAgentConfigImportResponseSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"importId": Schema{"type": "string"},
	}, []string{"importId"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigImportParams{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportParams)(nil)
	_ json.Marshaler   = ExternalAgentConfigImportResponse{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportResponse)(nil)
)
