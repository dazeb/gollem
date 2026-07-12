package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ModelRerouteReason string

const ModelRerouteReasonHighRiskCyberActivity ModelRerouteReason = "highRiskCyberActivity"

func (r ModelRerouteReason) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "model reroute reason", ModelRerouteReason.valid)
}

func (r *ModelRerouteReason) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "model reroute reason", ModelRerouteReason.valid)
}

func (r ModelRerouteReason) valid() bool {
	return r == ModelRerouteReasonHighRiskCyberActivity
}

type ModelVerification string

const ModelVerificationTrustedAccessForCyber ModelVerification = "trustedAccessForCyber"

func (v ModelVerification) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(v, "model verification", ModelVerification.valid)
}

func (v *ModelVerification) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, v, "model verification", ModelVerification.valid)
}

func (v ModelVerification) valid() bool {
	return v == ModelVerificationTrustedAccessForCyber
}

// ModelReroutedNotification is the exact fixed public model-reroute value. It
// remains standalone until Gollem has a model-safety producer.
type ModelReroutedNotification struct {
	ThreadID  string             `json:"threadId"`
	TurnID    string             `json:"turnId"`
	FromModel string             `json:"fromModel"`
	ToModel   string             `json:"toModel"`
	Reason    ModelRerouteReason `json:"reason"`
}

func (n *ModelReroutedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode model-rerouted notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model-rerouted notification",
		"threadId",
		"turnId",
		"fromModel",
		"toModel",
		"reason",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "model-rerouted notification", "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "model-rerouted notification", "turnId")
	if err != nil {
		return err
	}
	fromModel, err := decodeRequiredThreadItemValue[string](payload, "model-rerouted notification", "fromModel")
	if err != nil {
		return err
	}
	toModel, err := decodeRequiredThreadItemValue[string](payload, "model-rerouted notification", "toModel")
	if err != nil {
		return err
	}
	reason, err := decodeRequiredThreadItemValue[ModelRerouteReason](payload, "model-rerouted notification", "reason")
	if err != nil {
		return err
	}
	*n = ModelReroutedNotification{
		ThreadID: threadID, TurnID: turnID, FromModel: fromModel, ToModel: toModel, Reason: reason,
	}
	return nil
}

// ModelVerificationNotification is the exact fixed public model-verification
// value. It remains standalone until Gollem has a model-safety producer.
type ModelVerificationNotification struct {
	ThreadID      string              `json:"threadId"`
	TurnID        string              `json:"turnId"`
	Verifications []ModelVerification `json:"verifications" jsonschema:"nonnullable=true"`
}

func (n ModelVerificationNotification) MarshalJSON() ([]byte, error) {
	if n.Verifications == nil {
		return nil, errors.New("model-verification notification verifications cannot be null")
	}
	type wire ModelVerificationNotification
	return json.Marshal(wire(n))
}

func (n *ModelVerificationNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode model-verification notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model-verification notification",
		"threadId",
		"turnId",
		"verifications",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "model-verification notification", "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "model-verification notification", "turnId")
	if err != nil {
		return err
	}
	verifications, err := decodeRequiredThreadItemValue[[]ModelVerification](payload, "model-verification notification", "verifications")
	if err != nil {
		return err
	}
	*n = ModelVerificationNotification{ThreadID: threadID, TurnID: turnID, Verifications: verifications}
	return nil
}

// TurnModerationMetadataNotification is the exact fixed public moderation
// metadata value. It remains standalone until Gollem has a producer.
type TurnModerationMetadataNotification struct {
	ThreadID string    `json:"threadId"`
	TurnID   string    `json:"turnId"`
	Metadata JsonValue `json:"metadata"`
}

func (n *TurnModerationMetadataNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode turn-moderation metadata notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"turn-moderation metadata notification",
		"threadId",
		"turnId",
		"metadata",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "turn-moderation metadata notification", "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "turn-moderation metadata notification", "turnId")
	if err != nil {
		return err
	}
	metadata, err := decodeRequiredModelSafetyJSONValue(payload, "metadata")
	if err != nil {
		return err
	}
	*n = TurnModerationMetadataNotification{ThreadID: threadID, TurnID: turnID, Metadata: metadata}
	return nil
}

// ModelSafetyBufferingUpdatedNotification is the exact fixed public buffering
// state value. It remains standalone until Gollem has a model-safety producer.
type ModelSafetyBufferingUpdatedNotification struct {
	ThreadID        string   `json:"threadId"`
	TurnID          string   `json:"turnId"`
	Model           string   `json:"model"`
	UseCases        []string `json:"useCases" jsonschema:"nonnullable=true"`
	Reasons         []string `json:"reasons" jsonschema:"nonnullable=true"`
	ShowBufferingUI bool     `json:"showBufferingUi"`
	FasterModel     *string  `json:"fasterModel"`
}

func (n ModelSafetyBufferingUpdatedNotification) MarshalJSON() ([]byte, error) {
	if n.UseCases == nil {
		return nil, errors.New("model-safety buffering notification useCases cannot be null")
	}
	if n.Reasons == nil {
		return nil, errors.New("model-safety buffering notification reasons cannot be null")
	}
	type wire ModelSafetyBufferingUpdatedNotification
	return json.Marshal(wire(n))
}

func (n *ModelSafetyBufferingUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode model-safety buffering notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"model-safety buffering notification",
		"threadId",
		"turnId",
		"model",
		"useCases",
		"reasons",
		"showBufferingUi",
		"fasterModel",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "model-safety buffering notification", "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "model-safety buffering notification", "turnId")
	if err != nil {
		return err
	}
	model, err := decodeRequiredThreadItemValue[string](payload, "model-safety buffering notification", "model")
	if err != nil {
		return err
	}
	useCases, err := decodeRequiredNonNullStringArray(payload, "model-safety buffering notification", "useCases")
	if err != nil {
		return err
	}
	reasons, err := decodeRequiredNonNullStringArray(payload, "model-safety buffering notification", "reasons")
	if err != nil {
		return err
	}
	showBufferingUI, err := decodeRequiredThreadItemValue[bool](payload, "model-safety buffering notification", "showBufferingUi")
	if err != nil {
		return err
	}
	fasterModel, err := decodeOptionalNullableModelSafetyString(payload, "fasterModel")
	if err != nil {
		return err
	}
	*n = ModelSafetyBufferingUpdatedNotification{
		ThreadID: threadID, TurnID: turnID, Model: model, UseCases: useCases, Reasons: reasons,
		ShowBufferingUI: showBufferingUI, FasterModel: fasterModel,
	}
	return nil
}

func decodeOptionalNullableModelSafetyString(
	payload map[string]json.RawMessage,
	fieldName string,
) (*string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode model-safety buffering notification %s: %w", fieldName, err)
	}
	return &value, nil
}

func decodeRequiredModelSafetyJSONValue(
	payload map[string]json.RawMessage,
	fieldName string,
) (JsonValue, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return JsonValue{}, fmt.Errorf("turn-moderation metadata notification requires %s", fieldName)
	}
	var value JsonValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return JsonValue{}, fmt.Errorf("decode turn-moderation metadata notification %s: %w", fieldName, err)
	}
	return value, nil
}

var (
	_ json.Marshaler   = ModelRerouteReason("")
	_ json.Unmarshaler = (*ModelRerouteReason)(nil)
	_ json.Marshaler   = ModelVerification("")
	_ json.Unmarshaler = (*ModelVerification)(nil)
	_ json.Unmarshaler = (*ModelReroutedNotification)(nil)
	_ json.Marshaler   = ModelVerificationNotification{}
	_ json.Unmarshaler = (*ModelVerificationNotification)(nil)
	_ json.Unmarshaler = (*TurnModerationMetadataNotification)(nil)
	_ json.Marshaler   = ModelSafetyBufferingUpdatedNotification{}
	_ json.Unmarshaler = (*ModelSafetyBufferingUpdatedNotification)(nil)
)
