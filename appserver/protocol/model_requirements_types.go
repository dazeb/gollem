package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// NewThreadModelDefaults is the exact public model-default configuration for
// new threads. Nullable fields stay explicit in canonical output.
type NewThreadModelDefaults struct {
	Model                *string          `json:"model"`
	ModelReasoningEffort *ReasoningEffort `json:"modelReasoningEffort"`
	ServiceTier          *string          `json:"serviceTier"`
}

func (d *NewThreadModelDefaults) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("decode new-thread model defaults into nil receiver")
	}
	const objectName = "new-thread model defaults"
	payload, err := decodeExactThreadItemObject(data, objectName, "model", "modelReasoningEffort", "serviceTier")
	if err != nil {
		return err
	}
	model, err := decodeOptionalNullableModelRequirementValue[string](payload, objectName, "model")
	if err != nil {
		return err
	}
	modelReasoningEffort, err := decodeOptionalNullableModelRequirementValue[ReasoningEffort](
		payload,
		objectName,
		"modelReasoningEffort",
	)
	if err != nil {
		return err
	}
	serviceTier, err := decodeOptionalNullableModelRequirementValue[string](payload, objectName, "serviceTier")
	if err != nil {
		return err
	}
	*d = NewThreadModelDefaults{
		Model:                model,
		ModelReasoningEffort: modelReasoningEffort,
		ServiceTier:          serviceTier,
	}
	return nil
}

// ModelsRequirements is the exact public model-requirements configuration.
// The live Gollem configuration-requirements response remains separate.
type ModelsRequirements struct {
	NewThread *NewThreadModelDefaults `json:"newThread"`
}

func (r *ModelsRequirements) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode models requirements into nil receiver")
	}
	const objectName = "models requirements"
	payload, err := decodeExactThreadItemObject(data, objectName, "newThread")
	if err != nil {
		return err
	}
	newThread, err := decodeOptionalNullableModelRequirementValue[NewThreadModelDefaults](
		payload,
		objectName,
		"newThread",
	)
	if err != nil {
		return err
	}
	*r = ModelsRequirements{NewThread: newThread}
	return nil
}

func decodeOptionalNullableModelRequirementValue[T any](
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

var (
	_ json.Unmarshaler = (*NewThreadModelDefaults)(nil)
	_ json.Unmarshaler = (*ModelsRequirements)(nil)
)
