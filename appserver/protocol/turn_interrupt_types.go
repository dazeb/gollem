package protocol

import (
	"encoding/json"
	"errors"
)

// TurnInterruptParams is the exact fixed public turn-interrupt request. It
// remains standalone from Gollem's alias-compatible live handler contract.
type TurnInterruptParams struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
}

func (p *TurnInterruptParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode turn-interrupt params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "turn-interrupt params", "threadId", "turnId")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "turn-interrupt params", "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "turn-interrupt params", "turnId")
	if err != nil {
		return err
	}
	*p = TurnInterruptParams{ThreadID: threadID, TurnID: turnID}
	return nil
}

// TurnInterruptResponse is the exact fixed empty public response. Gollem's
// live handler continues to return its richer durable interrupt result.
type TurnInterruptResponse struct{}

func (r *TurnInterruptResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode turn-interrupt response into nil receiver")
	}
	if _, err := decodeExactThreadItemObject(data, "turn-interrupt response"); err != nil {
		return err
	}
	*r = TurnInterruptResponse{}
	return nil
}

var (
	_ json.Unmarshaler = (*TurnInterruptParams)(nil)
	_ json.Unmarshaler = (*TurnInterruptResponse)(nil)
)
