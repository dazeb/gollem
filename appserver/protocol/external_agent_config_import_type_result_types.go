package protocol

import (
	"encoding/json"
	"errors"
)

// ExternalAgentConfigImportTypeResult is an exact standalone result summary.
// Its nested records describe outcomes without proving that an import ran.
type ExternalAgentConfigImportTypeResult struct {
	ItemType  ExternalAgentConfigMigrationItemType       `json:"itemType"`
	Successes []ExternalAgentConfigImportItemTypeSuccess `json:"successes"`
	Failures  []ExternalAgentConfigImportItemTypeFailure `json:"failures"`
}

func (r ExternalAgentConfigImportTypeResult) MarshalJSON() ([]byte, error) {
	successes := r.Successes
	if successes == nil {
		successes = []ExternalAgentConfigImportItemTypeSuccess{}
	}
	failures := r.Failures
	if failures == nil {
		failures = []ExternalAgentConfigImportItemTypeFailure{}
	}
	return json.Marshal(struct {
		ItemType  ExternalAgentConfigMigrationItemType       `json:"itemType"`
		Successes []ExternalAgentConfigImportItemTypeSuccess `json:"successes"`
		Failures  []ExternalAgentConfigImportItemTypeFailure `json:"failures"`
	}{ItemType: r.ItemType, Successes: successes, Failures: failures})
}

func (r *ExternalAgentConfigImportTypeResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import type result into nil receiver")
	}
	const objectName = "external-agent config import type result"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "itemType", "successes", "failures")
	if err != nil {
		return err
	}
	itemType, err := decodeRequiredThreadItemValue[ExternalAgentConfigMigrationItemType](payload, objectName, "itemType")
	if err != nil {
		return err
	}
	successes, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportItemTypeSuccess](
		payload, objectName, "successes",
	)
	if err != nil {
		return err
	}
	failures, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportItemTypeFailure](
		payload, objectName, "failures",
	)
	if err != nil {
		return err
	}
	*r = ExternalAgentConfigImportTypeResult{
		ItemType: itemType, Successes: successes, Failures: failures,
	}
	return nil
}

func externalAgentConfigImportTypeResultSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"itemType": Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItemType"},
		"successes": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportItemTypeSuccess"},
		},
		"failures": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportItemTypeFailure"},
		},
	}, []string{"itemType", "successes", "failures"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigImportTypeResult{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportTypeResult)(nil)
)
