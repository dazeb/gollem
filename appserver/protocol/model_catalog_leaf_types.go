package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// InputModality is the exact closed public model input-modality value.
type InputModality string

const (
	InputModalityText  InputModality = "text"
	InputModalityImage InputModality = "image"
)

func (m InputModality) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "input modality", InputModality.valid)
}

func (m *InputModality) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "input modality", InputModality.valid)
}

func (m InputModality) valid() bool {
	return m == InputModalityText || m == InputModalityImage
}

// ReasoningEffortOption is an exact public model-catalog effort choice.
type ReasoningEffortOption struct {
	ReasoningEffort ReasoningEffort `json:"reasoningEffort"`
	Description     string          `json:"description"`
}

func (o *ReasoningEffortOption) UnmarshalJSON(data []byte) error {
	if o == nil {
		return errors.New("decode reasoning effort option into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "reasoning effort option", "reasoningEffort", "description")
	if err != nil {
		return err
	}
	reasoningEffort, err := decodeRequiredThreadItemValue[ReasoningEffort](payload, "reasoning effort option", "reasoningEffort")
	if err != nil {
		return err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, "reasoning effort option", "description")
	if err != nil {
		return err
	}
	*o = ReasoningEffortOption{ReasoningEffort: reasoningEffort, Description: description}
	return nil
}

// ModelAvailabilityNux is the exact public model-availability notice value.
type ModelAvailabilityNux struct {
	Message string `json:"message"`
}

func (n *ModelAvailabilityNux) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode model availability nux into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "model availability nux", "message")
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, "model availability nux", "message")
	if err != nil {
		return err
	}
	*n = ModelAvailabilityNux{Message: message}
	return nil
}

// ModelServiceTier is the exact public model service-tier descriptor.
type ModelServiceTier struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (t *ModelServiceTier) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode model service tier into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "model service tier", "id", "name", "description")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, "model service tier", "id")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, "model service tier", "name")
	if err != nil {
		return err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, "model service tier", "description")
	if err != nil {
		return err
	}
	*t = ModelServiceTier{ID: id, Name: name, Description: description}
	return nil
}

// ModelUpgradeInfo is the exact public model-upgrade descriptor. Nullable
// fields are canonical output fields, while decode accepts their Rust Option
// omission form.
type ModelUpgradeInfo struct {
	Model             string  `json:"model"`
	UpgradeCopy       *string `json:"upgradeCopy"`
	ModelLink         *string `json:"modelLink"`
	MigrationMarkdown *string `json:"migrationMarkdown"`
}

func (i *ModelUpgradeInfo) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode model upgrade info into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model upgrade info",
		"model",
		"upgradeCopy",
		"modelLink",
		"migrationMarkdown",
	)
	if err != nil {
		return err
	}
	model, err := decodeRequiredThreadItemValue[string](payload, "model upgrade info", "model")
	if err != nil {
		return err
	}
	upgradeCopy, err := decodeOptionalNullableModelCatalogString(payload, "upgradeCopy")
	if err != nil {
		return err
	}
	modelLink, err := decodeOptionalNullableModelCatalogString(payload, "modelLink")
	if err != nil {
		return err
	}
	migrationMarkdown, err := decodeOptionalNullableModelCatalogString(payload, "migrationMarkdown")
	if err != nil {
		return err
	}
	*i = ModelUpgradeInfo{
		Model: model, UpgradeCopy: upgradeCopy, ModelLink: modelLink, MigrationMarkdown: migrationMarkdown,
	}
	return nil
}

func decodeOptionalNullableModelCatalogString(payload map[string]json.RawMessage, fieldName string) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode model upgrade info %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = InputModality("")
	_ json.Unmarshaler = (*InputModality)(nil)
	_ json.Unmarshaler = (*ReasoningEffortOption)(nil)
	_ json.Unmarshaler = (*ModelAvailabilityNux)(nil)
	_ json.Unmarshaler = (*ModelServiceTier)(nil)
	_ json.Unmarshaler = (*ModelUpgradeInfo)(nil)
)
