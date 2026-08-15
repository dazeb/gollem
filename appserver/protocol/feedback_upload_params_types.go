package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// FeedbackUploadParams is the exact standalone source contract for
// feedback/upload. It intentionally does not bind Gollem log collection or
// feedback submission behavior.
type FeedbackUploadParams struct {
	Classification string             `json:"classification"`
	Reason         *string            `json:"reason"`
	ThreadID       *string            `json:"threadId"`
	IncludeLogs    bool               `json:"includeLogs,omitempty"`
	ExtraLogFiles  *[]string          `json:"extraLogFiles"`
	Tags           *map[string]string `json:"tags"`
}

func (p *FeedbackUploadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode feedback upload params into nil receiver")
	}
	const objectName = "feedback upload params"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"classification",
		"reason",
		"threadId",
		"includeLogs",
		"extraLogFiles",
		"tags",
	)
	if err != nil {
		return err
	}
	classification, err := decodeRequiredThreadItemValue[string](payload, objectName, "classification")
	if err != nil {
		return err
	}
	reason, err := decodeOptionalFeedbackValue[string](payload, objectName, "reason")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalFeedbackValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	includeLogs, err := decodeOptionalFeedbackBool(payload, objectName, "includeLogs")
	if err != nil {
		return err
	}
	extraLogFiles, err := decodeOptionalFeedbackValue[[]string](payload, objectName, "extraLogFiles")
	if err != nil {
		return err
	}
	tags, err := decodeOptionalFeedbackValue[map[string]string](payload, objectName, "tags")
	if err != nil {
		return err
	}
	*p = FeedbackUploadParams{
		Classification: classification,
		Reason:         reason,
		ThreadID:       threadID,
		IncludeLogs:    includeLogs,
		ExtraLogFiles:  extraLogFiles,
		Tags:           tags,
	}
	return nil
}

func decodeOptionalFeedbackValue[T any](
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

func decodeOptionalFeedbackBool(
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

func feedbackUploadParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"classification": Schema{"type": "string"},
			"extraLogFiles": Schema{
				"items": Schema{"type": "string"},
				"type":  []any{"array", "null"},
			},
			"includeLogs": Schema{"type": "boolean"},
			"reason":      Schema{"type": []any{"string", "null"}},
			"tags": Schema{
				"additionalProperties": Schema{"type": "string"},
				"type":                 []any{"object", "null"},
			},
			"threadId": Schema{"type": []any{"string", "null"}},
		},
		"required": []string{"classification"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*FeedbackUploadParams)(nil)
