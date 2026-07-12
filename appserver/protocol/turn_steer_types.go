package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// TurnSteerParams is the exact fixed public turn-steer request. It remains
// standalone from Gollem's prompt-alias live handler contract.
type TurnSteerParams struct {
	ThreadID            string      `json:"threadId"`
	ClientUserMessageID *string     `json:"clientUserMessageId,omitempty"`
	Input               []UserInput `json:"input" jsonschema:"nonnullable=true"`
	ExpectedTurnID      string      `json:"expectedTurnId"`
}

func (p TurnSteerParams) MarshalJSON() ([]byte, error) {
	if p.Input == nil {
		return nil, errors.New("turn-steer params input cannot be null")
	}
	type wire TurnSteerParams
	encoded, err := json.Marshal(wire(p))
	if err != nil {
		return nil, fmt.Errorf("encode turn-steer params: %w", err)
	}
	return encoded, nil
}

func (p *TurnSteerParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode turn-steer params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"turn-steer params",
		"threadId",
		"clientUserMessageId",
		"input",
		"expectedTurnId",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "turn-steer params", "threadId")
	if err != nil {
		return err
	}
	input, err := decodeRequiredThreadItemArray[UserInput](payload, "turn-steer params", "input")
	if err != nil {
		return err
	}
	expectedTurnID, err := decodeRequiredThreadItemValue[string](payload, "turn-steer params", "expectedTurnId")
	if err != nil {
		return err
	}
	clientUserMessageID, err := decodeOptionalNullableTurnSteerParam[string](payload, "clientUserMessageId")
	if err != nil {
		return err
	}
	*p = TurnSteerParams{
		ThreadID:            threadID,
		ClientUserMessageID: clientUserMessageID,
		Input:               input,
		ExpectedTurnID:      expectedTurnID,
	}
	return nil
}

type TurnSteerResponse struct {
	TurnID string `json:"turnId"`
}

func (r *TurnSteerResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode turn-steer response into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "turn-steer response", "turnId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "turn-steer response", "turnId")
	if err != nil {
		return err
	}
	*r = TurnSteerResponse{TurnID: turnID}
	return nil
}

func decodeOptionalNullableTurnSteerParam[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode turn-steer params %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = TurnSteerParams{}
	_ json.Unmarshaler = (*TurnSteerParams)(nil)
	_ json.Unmarshaler = (*TurnSteerResponse)(nil)
)
