package protocol

import (
	"encoding/json"
	"errors"
)

// ExternalAgentConfigImportProgressNotification is the exact standalone
// progress value. It does not imply that Gollem produces import progress.
type ExternalAgentConfigImportProgressNotification struct {
	ImportID        string                                `json:"importId"`
	ItemTypeResults []ExternalAgentConfigImportTypeResult `json:"itemTypeResults"`
}

func (n ExternalAgentConfigImportProgressNotification) MarshalJSON() ([]byte, error) {
	return marshalExternalAgentConfigImportNotification(n.ImportID, n.ItemTypeResults)
}

func (n *ExternalAgentConfigImportProgressNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode external-agent config import progress notification into nil receiver")
	}
	importID, results, err := unmarshalExternalAgentConfigImportNotification(
		data,
		"external-agent config import progress notification",
	)
	if err != nil {
		return err
	}
	*n = ExternalAgentConfigImportProgressNotification{ImportID: importID, ItemTypeResults: results}
	return nil
}

// ExternalAgentConfigImportCompletedNotification is the exact standalone
// completion value. It does not imply that Gollem executes imports.
type ExternalAgentConfigImportCompletedNotification struct {
	ImportID        string                                `json:"importId"`
	ItemTypeResults []ExternalAgentConfigImportTypeResult `json:"itemTypeResults"`
}

func (n ExternalAgentConfigImportCompletedNotification) MarshalJSON() ([]byte, error) {
	return marshalExternalAgentConfigImportNotification(n.ImportID, n.ItemTypeResults)
}

func (n *ExternalAgentConfigImportCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode external-agent config import completed notification into nil receiver")
	}
	importID, results, err := unmarshalExternalAgentConfigImportNotification(
		data,
		"external-agent config import completed notification",
	)
	if err != nil {
		return err
	}
	*n = ExternalAgentConfigImportCompletedNotification{ImportID: importID, ItemTypeResults: results}
	return nil
}

func marshalExternalAgentConfigImportNotification(
	importID string,
	results []ExternalAgentConfigImportTypeResult,
) ([]byte, error) {
	if results == nil {
		results = []ExternalAgentConfigImportTypeResult{}
	}
	return json.Marshal(struct {
		ImportID        string                                `json:"importId"`
		ItemTypeResults []ExternalAgentConfigImportTypeResult `json:"itemTypeResults"`
	}{ImportID: importID, ItemTypeResults: results})
}

func unmarshalExternalAgentConfigImportNotification(
	data []byte,
	objectName string,
) (string, []ExternalAgentConfigImportTypeResult, error) {
	payload, err := decodeExternalAgentConfigObject(data, objectName, "importId", "itemTypeResults")
	if err != nil {
		return "", nil, err
	}
	importID, err := decodeRequiredThreadItemValue[string](payload, objectName, "importId")
	if err != nil {
		return "", nil, err
	}
	results, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportTypeResult](
		payload,
		objectName,
		"itemTypeResults",
	)
	if err != nil {
		return "", nil, err
	}
	return importID, results, nil
}

func externalAgentConfigImportNotificationSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"importId": Schema{"type": "string"},
		"itemTypeResults": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportTypeResult"},
		},
	}, []string{"importId", "itemTypeResults"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigImportProgressNotification{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportProgressNotification)(nil)
	_ json.Marshaler   = ExternalAgentConfigImportCompletedNotification{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportCompletedNotification)(nil)
)
