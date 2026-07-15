package protocol

import (
	"encoding/json"
	"errors"
)

// ExternalAgentConfigImportHistory is an exact standalone import-history
// record. Its contents are descriptive data and do not prove an import ran.
type ExternalAgentConfigImportHistory struct {
	ImportID      string                                     `json:"importId"`
	CompletedAtMS int64                                      `json:"completedAtMs"`
	Successes     []ExternalAgentConfigImportItemTypeSuccess `json:"successes"`
	Failures      []ExternalAgentConfigImportItemTypeFailure `json:"failures"`
}

func (h ExternalAgentConfigImportHistory) MarshalJSON() ([]byte, error) {
	successes := h.Successes
	if successes == nil {
		successes = []ExternalAgentConfigImportItemTypeSuccess{}
	}
	failures := h.Failures
	if failures == nil {
		failures = []ExternalAgentConfigImportItemTypeFailure{}
	}
	return json.Marshal(struct {
		ImportID      string                                     `json:"importId"`
		CompletedAtMS int64                                      `json:"completedAtMs"`
		Successes     []ExternalAgentConfigImportItemTypeSuccess `json:"successes"`
		Failures      []ExternalAgentConfigImportItemTypeFailure `json:"failures"`
	}{
		ImportID: h.ImportID, CompletedAtMS: h.CompletedAtMS,
		Successes: successes, Failures: failures,
	})
}

func (h *ExternalAgentConfigImportHistory) UnmarshalJSON(data []byte) error {
	if h == nil {
		return errors.New("decode external-agent config import history into nil receiver")
	}
	const objectName = "external-agent config import history"
	payload, err := decodeExternalAgentConfigObject(
		data, objectName, "importId", "completedAtMs", "successes", "failures",
	)
	if err != nil {
		return err
	}
	importID, err := decodeRequiredThreadItemValue[string](payload, objectName, "importId")
	if err != nil {
		return err
	}
	completedAtMS, err := decodeRequiredThreadItemValue[int64](payload, objectName, "completedAtMs")
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
	*h = ExternalAgentConfigImportHistory{
		ImportID: importID, CompletedAtMS: completedAtMS,
		Successes: successes, Failures: failures,
	}
	return nil
}

// ExternalAgentConfigImportHistoriesReadResponse is an exact standalone
// history-read response. Gollem does not bind it to a method or persist it.
type ExternalAgentConfigImportHistoriesReadResponse struct {
	Data []ExternalAgentConfigImportHistory `json:"data"`
}

func (r ExternalAgentConfigImportHistoriesReadResponse) MarshalJSON() ([]byte, error) {
	data := r.Data
	if data == nil {
		data = []ExternalAgentConfigImportHistory{}
	}
	return json.Marshal(struct {
		Data []ExternalAgentConfigImportHistory `json:"data"`
	}{Data: data})
}

func (r *ExternalAgentConfigImportHistoriesReadResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import histories response into nil receiver")
	}
	const objectName = "external-agent config import histories response"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "data")
	if err != nil {
		return err
	}
	histories, err := decodeRequiredThreadItemArray[ExternalAgentConfigImportHistory](
		payload, objectName, "data",
	)
	if err != nil {
		return err
	}
	*r = ExternalAgentConfigImportHistoriesReadResponse{Data: histories}
	return nil
}

func externalAgentConfigImportHistorySchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"importId":      Schema{"type": "string"},
		"completedAtMs": Schema{"type": "integer", "format": "int64"},
		"successes": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportItemTypeSuccess"},
		},
		"failures": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportItemTypeFailure"},
		},
	}, []string{"importId", "completedAtMs", "successes", "failures"})
}

func externalAgentConfigImportHistoriesReadResponseSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"data": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/ExternalAgentConfigImportHistory"},
		},
	}, []string{"data"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigImportHistory{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportHistory)(nil)
	_ json.Marshaler   = ExternalAgentConfigImportHistoriesReadResponse{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportHistoriesReadResponse)(nil)
)
