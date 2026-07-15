package protocol

import (
	"encoding/json"
	"errors"
)

// ExternalAgentConfigImportItemTypeSuccess is an exact standalone import
// result. Its paths and strings are descriptive data, not evidence or authority.
type ExternalAgentConfigImportItemTypeSuccess struct {
	ItemType ExternalAgentConfigMigrationItemType `json:"itemType"`
	CWD      *string                              `json:"cwd"`
	Source   *string                              `json:"source"`
	Target   *string                              `json:"target"`
}

func (r ExternalAgentConfigImportItemTypeSuccess) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ItemType ExternalAgentConfigMigrationItemType `json:"itemType"`
		CWD      *string                              `json:"cwd"`
		Source   *string                              `json:"source"`
		Target   *string                              `json:"target"`
	}{ItemType: r.ItemType, CWD: r.CWD, Source: r.Source, Target: r.Target})
}

func (r *ExternalAgentConfigImportItemTypeSuccess) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import item success into nil receiver")
	}
	const objectName = "external-agent config import item success"
	payload, err := decodeExternalAgentConfigObject(data, objectName, "itemType", "cwd", "source", "target")
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
	*r = ExternalAgentConfigImportItemTypeSuccess{
		ItemType: itemType, CWD: cwd, Source: source, Target: target,
	}
	return nil
}

// ExternalAgentConfigImportItemTypeFailure is an exact standalone import
// failure description. It does not imply that import execution occurred.
type ExternalAgentConfigImportItemTypeFailure struct {
	ItemType     ExternalAgentConfigMigrationItemType `json:"itemType"`
	ErrorType    *string                              `json:"errorType"`
	FailureStage string                               `json:"failureStage"`
	Message      string                               `json:"message"`
	CWD          *string                              `json:"cwd"`
	Source       *string                              `json:"source"`
}

func (r ExternalAgentConfigImportItemTypeFailure) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ItemType     ExternalAgentConfigMigrationItemType `json:"itemType"`
		ErrorType    *string                              `json:"errorType"`
		FailureStage string                               `json:"failureStage"`
		Message      string                               `json:"message"`
		CWD          *string                              `json:"cwd"`
		Source       *string                              `json:"source"`
	}{
		ItemType: r.ItemType, ErrorType: r.ErrorType, FailureStage: r.FailureStage,
		Message: r.Message, CWD: r.CWD, Source: r.Source,
	})
}

func (r *ExternalAgentConfigImportItemTypeFailure) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode external-agent config import item failure into nil receiver")
	}
	const objectName = "external-agent config import item failure"
	payload, err := decodeExternalAgentConfigObject(
		data, objectName, "itemType", "errorType", "failureStage", "message", "cwd", "source",
	)
	if err != nil {
		return err
	}
	itemType, err := decodeRequiredThreadItemValue[ExternalAgentConfigMigrationItemType](payload, objectName, "itemType")
	if err != nil {
		return err
	}
	errorType, err := decodeOptionalNullableConfigValue[string](payload, objectName, "errorType")
	if err != nil {
		return err
	}
	failureStage, err := decodeRequiredThreadItemValue[string](payload, objectName, "failureStage")
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, objectName, "message")
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
	*r = ExternalAgentConfigImportItemTypeFailure{
		ItemType: itemType, ErrorType: errorType, FailureStage: failureStage,
		Message: message, CWD: cwd, Source: source,
	}
	return nil
}

func externalAgentConfigImportItemTypeSuccessSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"itemType": Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItemType"},
		"cwd":      Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
		"source":   Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
		"target":   Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
	}, []string{"itemType"})
}

func externalAgentConfigImportItemTypeFailureSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"itemType":     Schema{"$ref": "#/$defs/ExternalAgentConfigMigrationItemType"},
		"errorType":    Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
		"failureStage": Schema{"type": "string"},
		"message":      Schema{"type": "string"},
		"cwd":          Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
		"source":       Schema{"anyOf": []any{Schema{"type": "string"}, Schema{"type": "null"}}},
	}, []string{"itemType", "failureStage", "message"})
}

var (
	_ json.Marshaler   = ExternalAgentConfigImportItemTypeSuccess{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportItemTypeSuccess)(nil)
	_ json.Marshaler   = ExternalAgentConfigImportItemTypeFailure{}
	_ json.Unmarshaler = (*ExternalAgentConfigImportItemTypeFailure)(nil)
)
