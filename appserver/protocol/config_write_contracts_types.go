package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MergeStrategy is the exact closed public config-write merge strategy.
type MergeStrategy string

const (
	MergeStrategyReplace MergeStrategy = "replace"
	MergeStrategyUpsert  MergeStrategy = "upsert"
)

func (s MergeStrategy) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "config merge strategy", MergeStrategy.valid)
}

func (s *MergeStrategy) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "config merge strategy", MergeStrategy.valid)
}

func (s MergeStrategy) valid() bool {
	return s == MergeStrategyReplace || s == MergeStrategyUpsert
}

// WriteStatus is the exact closed public config-write result status.
type WriteStatus string

const (
	WriteStatusOK           WriteStatus = "ok"
	WriteStatusOKOverridden WriteStatus = "okOverridden"
)

func (s WriteStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "config write status", WriteStatus.valid)
}

func (s *WriteStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "config write status", WriteStatus.valid)
}

func (s WriteStatus) valid() bool {
	return s == WriteStatusOK || s == WriteStatusOKOverridden
}

// ConfigEdit is one exact public key-path edit.
type ConfigEdit struct {
	KeyPath       string        `json:"keyPath"`
	Value         JsonValue     `json:"value"`
	MergeStrategy MergeStrategy `json:"mergeStrategy"`
}

func (e *ConfigEdit) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode config edit into nil receiver")
	}
	const objectName = "config edit"
	payload, err := decodeExactThreadItemObject(data, objectName, "keyPath", "value", "mergeStrategy")
	if err != nil {
		return err
	}
	keyPath, err := decodeRequiredThreadItemValue[string](payload, objectName, "keyPath")
	if err != nil {
		return err
	}
	value, err := decodeRequiredThreadItemJSONValue(payload, objectName, "value")
	if err != nil {
		return err
	}
	mergeStrategy, err := decodeRequiredThreadItemValue[MergeStrategy](payload, objectName, "mergeStrategy")
	if err != nil {
		return err
	}
	*e = ConfigEdit{KeyPath: keyPath, Value: value, MergeStrategy: mergeStrategy}
	return nil
}

// ConfigValueWriteParams is the exact standalone public single-value write
// request. Gollem's live memory-backed request remains a separate contract.
type ConfigValueWriteParams struct {
	KeyPath         string        `json:"keyPath"`
	Value           JsonValue     `json:"value"`
	MergeStrategy   MergeStrategy `json:"mergeStrategy"`
	FilePath        *string       `json:"filePath,omitempty"`
	ExpectedVersion *string       `json:"expectedVersion,omitempty"`
}

func (p ConfigValueWriteParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		KeyPath         string        `json:"keyPath"`
		Value           JsonValue     `json:"value"`
		MergeStrategy   MergeStrategy `json:"mergeStrategy"`
		FilePath        *string       `json:"filePath"`
		ExpectedVersion *string       `json:"expectedVersion"`
	}
	return json.Marshal(wire(p))
}

func (p *ConfigValueWriteParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode config value-write params into nil receiver")
	}
	const objectName = "config value-write params"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"keyPath",
		"value",
		"mergeStrategy",
		"filePath",
		"expectedVersion",
	)
	if err != nil {
		return err
	}
	keyPath, err := decodeRequiredThreadItemValue[string](payload, objectName, "keyPath")
	if err != nil {
		return err
	}
	value, err := decodeRequiredThreadItemJSONValue(payload, objectName, "value")
	if err != nil {
		return err
	}
	mergeStrategy, err := decodeRequiredThreadItemValue[MergeStrategy](payload, objectName, "mergeStrategy")
	if err != nil {
		return err
	}
	filePath, err := decodeOptionalNullableConfigWriteValue[string](payload, objectName, "filePath")
	if err != nil {
		return err
	}
	expectedVersion, err := decodeOptionalNullableConfigWriteValue[string](payload, objectName, "expectedVersion")
	if err != nil {
		return err
	}
	*p = ConfigValueWriteParams{
		KeyPath:         keyPath,
		Value:           value,
		MergeStrategy:   mergeStrategy,
		FilePath:        filePath,
		ExpectedVersion: expectedVersion,
	}
	return nil
}

// ConfigBatchWriteParams is the exact standalone public ordered edit batch.
type ConfigBatchWriteParams struct {
	Edits            []ConfigEdit `json:"edits" jsonschema:"nonnullable=true"`
	FilePath         *string      `json:"filePath,omitempty"`
	ExpectedVersion  *string      `json:"expectedVersion,omitempty"`
	ReloadUserConfig bool         `json:"reloadUserConfig,omitempty"`
}

func (p ConfigBatchWriteParams) MarshalJSON() ([]byte, error) {
	if p.Edits == nil {
		return nil, errors.New("config batch-write params edits cannot be null")
	}
	type wire struct {
		Edits            []ConfigEdit `json:"edits"`
		FilePath         *string      `json:"filePath"`
		ExpectedVersion  *string      `json:"expectedVersion"`
		ReloadUserConfig bool         `json:"reloadUserConfig,omitempty"`
	}
	return json.Marshal(wire(p))
}

func (p *ConfigBatchWriteParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode config batch-write params into nil receiver")
	}
	const objectName = "config batch-write params"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"edits",
		"filePath",
		"expectedVersion",
		"reloadUserConfig",
	)
	if err != nil {
		return err
	}
	edits, err := decodeRequiredThreadItemArray[ConfigEdit](payload, objectName, "edits")
	if err != nil {
		return err
	}
	filePath, err := decodeOptionalNullableConfigWriteValue[string](payload, objectName, "filePath")
	if err != nil {
		return err
	}
	expectedVersion, err := decodeOptionalNullableConfigWriteValue[string](payload, objectName, "expectedVersion")
	if err != nil {
		return err
	}
	reloadUserConfig, err := decodeOptionalConfigWriteBool(payload, objectName, "reloadUserConfig")
	if err != nil {
		return err
	}
	*p = ConfigBatchWriteParams{
		Edits:            edits,
		FilePath:         filePath,
		ExpectedVersion:  expectedVersion,
		ReloadUserConfig: reloadUserConfig,
	}
	return nil
}

// ConfigWriteResponse is the exact standalone public config-write result.
// Gollem's live value/entry snapshot remains a separate response contract.
type ConfigWriteResponse struct {
	Status             WriteStatus         `json:"status"`
	Version            string              `json:"version"`
	FilePath           AbsolutePathBuf     `json:"filePath"`
	OverriddenMetadata *OverriddenMetadata `json:"overriddenMetadata"`
}

func (r *ConfigWriteResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode config-write response into nil receiver")
	}
	const objectName = "config-write response"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"status",
		"version",
		"filePath",
		"overriddenMetadata",
	)
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[WriteStatus](payload, objectName, "status")
	if err != nil {
		return err
	}
	version, err := decodeRequiredThreadItemValue[string](payload, objectName, "version")
	if err != nil {
		return err
	}
	filePath, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "filePath")
	if err != nil {
		return err
	}
	overriddenMetadata, err := decodeOptionalNullableConfigWriteValue[OverriddenMetadata](
		payload,
		objectName,
		"overriddenMetadata",
	)
	if err != nil {
		return err
	}
	*r = ConfigWriteResponse{
		Status:             status,
		Version:            version,
		FilePath:           filePath,
		OverriddenMetadata: overriddenMetadata,
	}
	return nil
}

func decodeOptionalNullableConfigWriteValue[T any](
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

func decodeOptionalConfigWriteBool(
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

var (
	_ json.Marshaler   = MergeStrategy("")
	_ json.Unmarshaler = (*MergeStrategy)(nil)
	_ json.Marshaler   = WriteStatus("")
	_ json.Unmarshaler = (*WriteStatus)(nil)
	_ json.Unmarshaler = (*ConfigEdit)(nil)
	_ json.Marshaler   = ConfigValueWriteParams{}
	_ json.Unmarshaler = (*ConfigValueWriteParams)(nil)
	_ json.Marshaler   = ConfigBatchWriteParams{}
	_ json.Unmarshaler = (*ConfigBatchWriteParams)(nil)
	_ json.Unmarshaler = (*ConfigWriteResponse)(nil)
)
