package protocol

import (
	"encoding/json"
	"errors"
)

// ExternalAgentConfigImportHistoryRecordSuccessParams is a title-aware
// standalone import success record. Its values describe past results only.
type ExternalAgentConfigImportHistoryRecordSuccessParams struct {
	ItemType ExternalAgentConfigMigrationItemType `json:"itemType"`
	CWD      *string                              `json:"cwd"`
	Source   *string                              `json:"source"`
	Target   *string                              `json:"target"`
	Title    *string                              `json:"title"`
}

func (r ExternalAgentConfigImportHistoryRecordSuccessParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ItemType ExternalAgentConfigMigrationItemType `json:"itemType"`
		CWD      *string                              `json:"cwd"`
		Source   *string                              `json:"source"`
		Target   *string                              `json:"target"`
		Title    *string                              `json:"title"`
	}{
		ItemType: r.ItemType, CWD: r.CWD, Source: r.Source, Target: r.Target, Title: r.Title,
	})
}

func (r *ExternalAgentConfigImportHistoryRecordSuccessParams) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import history-record success into nil receiver")
	}
	const objectName = "external-agent config import history-record success"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "itemType", "cwd", "source", "target", "title")
	if err != nil {
		return err
	}
	itemType, err := decodeRequiredThreadItemValue[ExternalAgentConfigMigrationItemType](payload, objectName, "itemType")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	source, err := decodeOptionalNullableConfigValue[string](payload, objectName, "source")
	if err != nil {
		return err
	}
	target, err := decodeOptionalNullableConfigValue[string](payload, objectName, "target")
	if err != nil {
		return err
	}
	title, err := decodeOptionalNullableConfigValue[string](payload, objectName, "title")
	if err != nil {
		return err
	}
	*r = ExternalAgentConfigImportHistoryRecordSuccessParams{
		ItemType: itemType, CWD: cwd, Source: source, Target: target, Title: title,
	}
	return nil
}

// ExternalAgentConfigImportHistoryRecordTypeResultParams groups completed
// history records by source item type without claiming an import was run.
type ExternalAgentConfigImportHistoryRecordTypeResultParams struct {
	ItemType  ExternalAgentConfigMigrationItemType                  `json:"itemType"`
	Successes []ExternalAgentConfigImportHistoryRecordSuccessParams `json:"successes"`
	Failures  []ExternalAgentConfigImportItemTypeFailure            `json:"failures"`
}

func (r ExternalAgentConfigImportHistoryRecordTypeResultParams) MarshalJSON() ([]byte, error) {
	successes := r.Successes
	if successes == nil {
		successes = []ExternalAgentConfigImportHistoryRecordSuccessParams{}
	}
	failures := r.Failures
	if failures == nil {
		failures = []ExternalAgentConfigImportItemTypeFailure{}
	}
	return json.Marshal(struct {
		ItemType  ExternalAgentConfigMigrationItemType                  `json:"itemType"`
		Successes []ExternalAgentConfigImportHistoryRecordSuccessParams `json:"successes"`
		Failures  []ExternalAgentConfigImportItemTypeFailure            `json:"failures"`
	}{ItemType: r.ItemType, Successes: successes, Failures: failures})
}

func (r *ExternalAgentConfigImportHistoryRecordTypeResultParams) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import history-record type result into nil receiver")
	}
	const objectName = "external-agent config import history-record type result"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "itemType", "successes", "failures")
	if err != nil {
		return err
	}
	itemType, err := decodeRequiredThreadItemValue[ExternalAgentConfigMigrationItemType](payload, objectName, "itemType")
	if err != nil {
		return err
	}
	successes, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportHistoryRecordSuccessParams](payload, objectName, "successes")
	if err != nil {
		return err
	}
	failures, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportItemTypeFailure](payload, objectName, "failures")
	if err != nil {
		return err
	}
	*r = ExternalAgentConfigImportHistoryRecordTypeResultParams{
		ItemType: itemType, Successes: successes, Failures: failures,
	}
	return nil
}

// ExternalAgentConfigImportHistoryRecordParams is the exact standalone source
// record for an externally completed import history write.
type ExternalAgentConfigImportHistoryRecordParams struct {
	ProviderID      string                                                   `json:"providerId"`
	ItemTypeResults []ExternalAgentConfigImportHistoryRecordTypeResultParams `json:"itemTypeResults"`
}

func (p ExternalAgentConfigImportHistoryRecordParams) MarshalJSON() ([]byte, error) {
	results := p.ItemTypeResults
	if results == nil {
		results = []ExternalAgentConfigImportHistoryRecordTypeResultParams{}
	}
	return json.Marshal(struct {
		ProviderID      string                                                   `json:"providerId"`
		ItemTypeResults []ExternalAgentConfigImportHistoryRecordTypeResultParams `json:"itemTypeResults"`
	}{ProviderID: p.ProviderID, ItemTypeResults: results})
}

func (p *ExternalAgentConfigImportHistoryRecordParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode external-agent config import history-record params into nil receiver")
	}
	const objectName = "external-agent config import history-record params"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "providerId", "itemTypeResults")
	if err != nil {
		return err
	}
	providerID, err := decodeRequiredThreadItemValue[string](payload, objectName, "providerId")
	if err != nil {
		return err
	}
	results, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportHistoryRecordTypeResultParams](payload, objectName, "itemTypeResults")
	if err != nil {
		return err
	}
	*p = ExternalAgentConfigImportHistoryRecordParams{ProviderID: providerID, ItemTypeResults: results}
	return nil
}

func externalAgentConfigImportHistoryRecordSuccessParamsSchema() Schema {
	return Schema{"properties": Schema{
		"cwd":      Schema{"type": []any{"string", "null"}},
		"itemType": Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItemType"},
		"source":   Schema{"type": []any{"string", "null"}},
		"target":   Schema{"type": []any{"string", "null"}},
		"title": Schema{
			"default":     nil,
			"description": "Original title for an imported session, when available.",
			"type":        []any{"string", "null"},
		},
	}, "required": []string{"itemType"}, "type": "object"}
}

func externalAgentConfigImportHistoryRecordTypeResultParamsSchema() Schema {
	return Schema{"properties": Schema{
		"failures":  Schema{"items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportItemTypeFailure"}, "type": "array"},
		"itemType":  Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItemType"},
		"successes": Schema{"items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportHistoryRecordSuccessParams"}, "type": "array"},
	}, "required": []string{"failures", "itemType", "successes"}, "type": "object"}
}

func externalAgentConfigImportHistoryRecordParamsSchema() Schema {
	return Schema{"properties": Schema{
		"itemTypeResults": Schema{
			"description": "Completed results grouped by imported item type.",
			"items":       Schema{"$ref": "#/$defs/ExternalAgentConfigImportHistoryRecordTypeResultParams"},
			"type":        "array",
		},
		"providerId": Schema{
			"description": "Opaque provider identifier for the externally completed import.",
			"type":        "string",
		},
	}, "required": []string{"itemTypeResults", "providerId"}, "type": "object"}
}

var (
	_ json.Marshaler   = ExternalAgentConfigImportHistoryRecordSuccessParams{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportHistoryRecordSuccessParams)(nil)
	_ json.Marshaler   = ExternalAgentConfigImportHistoryRecordTypeResultParams{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportHistoryRecordTypeResultParams)(nil)
	_ json.Marshaler   = ExternalAgentConfigImportHistoryRecordParams{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportHistoryRecordParams)(nil)
)
