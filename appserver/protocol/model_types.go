package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Model is the exact public model-catalog entry. It remains separate from the
// provider-neutral runtime catalog until the list boundary has an explicit
// projection between the two contracts.
type Model struct {
	ID                        string                  `json:"id"`
	Model                     string                  `json:"model"`
	Upgrade                   *string                 `json:"upgrade"`
	UpgradeInfo               *ModelUpgradeInfo       `json:"upgradeInfo"`
	AvailabilityNux           *ModelAvailabilityNux   `json:"availabilityNux"`
	DisplayName               string                  `json:"displayName"`
	Description               string                  `json:"description"`
	Hidden                    bool                    `json:"hidden"`
	SupportedReasoningEfforts []ReasoningEffortOption `json:"supportedReasoningEfforts" jsonschema:"nonnullable=true"`
	DefaultReasoningEffort    ReasoningEffort         `json:"defaultReasoningEffort"`
	InputModalities           []InputModality         `json:"inputModalities" jsonschema:"nonnullable=true"`
	SupportsPersonality       bool                    `json:"supportsPersonality"`
	AdditionalSpeedTiers      []string                `json:"additionalSpeedTiers" jsonschema:"nonnullable=true"`
	ServiceTiers              []ModelServiceTier      `json:"serviceTiers" jsonschema:"nonnullable=true"`
	DefaultServiceTier        *string                 `json:"defaultServiceTier"`
	IsDefault                 bool                    `json:"isDefault"`
}

func (m Model) MarshalJSON() ([]byte, error) {
	type wire Model
	canonical := wire(m)
	canonical.SupportedReasoningEfforts = nonNilModelSlice(canonical.SupportedReasoningEfforts)
	canonical.InputModalities = nonNilModelSlice(canonical.InputModalities)
	canonical.AdditionalSpeedTiers = nonNilModelSlice(canonical.AdditionalSpeedTiers)
	canonical.ServiceTiers = nonNilModelSlice(canonical.ServiceTiers)
	return json.Marshal(canonical)
}

func (m *Model) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode model into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model",
		"id",
		"model",
		"upgrade",
		"upgradeInfo",
		"availabilityNux",
		"displayName",
		"description",
		"hidden",
		"supportedReasoningEfforts",
		"defaultReasoningEffort",
		"inputModalities",
		"supportsPersonality",
		"additionalSpeedTiers",
		"serviceTiers",
		"defaultServiceTier",
		"isDefault",
	)
	if err != nil {
		return err
	}

	id, err := decodeRequiredThreadItemValue[string](payload, "model", "id")
	if err != nil {
		return err
	}
	model, err := decodeRequiredThreadItemValue[string](payload, "model", "model")
	if err != nil {
		return err
	}
	upgrade, err := decodeOptionalNullableModelValue[string](payload, "upgrade")
	if err != nil {
		return err
	}
	upgradeInfo, err := decodeOptionalNullableModelValue[ModelUpgradeInfo](payload, "upgradeInfo")
	if err != nil {
		return err
	}
	availabilityNux, err := decodeOptionalNullableModelValue[ModelAvailabilityNux](payload, "availabilityNux")
	if err != nil {
		return err
	}
	displayName, err := decodeRequiredThreadItemValue[string](payload, "model", "displayName")
	if err != nil {
		return err
	}
	description, err := decodeRequiredThreadItemValue[string](payload, "model", "description")
	if err != nil {
		return err
	}
	hidden, err := decodeRequiredThreadItemValue[bool](payload, "model", "hidden")
	if err != nil {
		return err
	}
	supportedReasoningEfforts, err := decodeRequiredThreadItemArray[ReasoningEffortOption](
		payload,
		"model",
		"supportedReasoningEfforts",
	)
	if err != nil {
		return err
	}
	defaultReasoningEffort, err := decodeRequiredThreadItemValue[ReasoningEffort](
		payload,
		"model",
		"defaultReasoningEffort",
	)
	if err != nil {
		return err
	}
	inputModalities, err := decodeDefaultedModelArray(
		payload,
		"inputModalities",
		[]InputModality{InputModalityText, InputModalityImage},
	)
	if err != nil {
		return err
	}
	supportsPersonality, err := decodeDefaultedModelBool(payload, "supportsPersonality")
	if err != nil {
		return err
	}
	additionalSpeedTiers, err := decodeDefaultedModelArray[string](payload, "additionalSpeedTiers", nil)
	if err != nil {
		return err
	}
	serviceTiers, err := decodeDefaultedModelArray[ModelServiceTier](payload, "serviceTiers", nil)
	if err != nil {
		return err
	}
	defaultServiceTier, err := decodeOptionalNullableModelValue[string](payload, "defaultServiceTier")
	if err != nil {
		return err
	}
	isDefault, err := decodeRequiredThreadItemValue[bool](payload, "model", "isDefault")
	if err != nil {
		return err
	}

	*m = Model{
		ID:                        id,
		Model:                     model,
		Upgrade:                   upgrade,
		UpgradeInfo:               upgradeInfo,
		AvailabilityNux:           availabilityNux,
		DisplayName:               displayName,
		Description:               description,
		Hidden:                    hidden,
		SupportedReasoningEfforts: supportedReasoningEfforts,
		DefaultReasoningEffort:    defaultReasoningEffort,
		InputModalities:           inputModalities,
		SupportsPersonality:       supportsPersonality,
		AdditionalSpeedTiers:      additionalSpeedTiers,
		ServiceTiers:              serviceTiers,
		DefaultServiceTier:        defaultServiceTier,
		IsDefault:                 isDefault,
	}
	return nil
}

func decodeOptionalNullableModelValue[T any](payload map[string]json.RawMessage, fieldName string) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode model %s: %w", fieldName, err)
	}
	return &value, nil
}

func decodeDefaultedModelArray[T any](
	payload map[string]json.RawMessage,
	fieldName string,
	defaults []T,
) ([]T, error) {
	if _, ok := payload[fieldName]; !ok {
		return nonNilModelSlice(defaults), nil
	}
	return decodeRequiredThreadItemArray[T](payload, "model", fieldName)
}

func decodeDefaultedModelBool(payload map[string]json.RawMessage, fieldName string) (bool, error) {
	if _, ok := payload[fieldName]; !ok {
		return false, nil
	}
	return decodeRequiredThreadItemValue[bool](payload, "model", fieldName)
}

func nonNilModelSlice[T any](values []T) []T {
	result := make([]T, len(values))
	copy(result, values)
	return result
}

var (
	_ json.Marshaler   = Model{}
	_ json.Unmarshaler = (*Model)(nil)
)
