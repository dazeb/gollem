package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Turn is the exact public v2 turn projection. It remains separate from
// Gollem's durable TurnRecord and is not bound to methods until its public
// parent contracts are exported.
type Turn struct {
	ID          string        `json:"id"`
	Items       []ThreadItem  `json:"items" jsonschema:"nonnullable=true"`
	ItemsView   TurnItemsView `json:"itemsView"`
	Status      TurnStatus    `json:"status"`
	Error       *TurnError    `json:"error"`
	StartedAt   *int64        `json:"startedAt"`
	CompletedAt *int64        `json:"completedAt"`
	DurationMS  *int64        `json:"durationMs"`
}

func (t Turn) MarshalJSON() ([]byte, error) {
	if t.Items == nil {
		return nil, errors.New("turn items cannot be null")
	}
	type wire Turn
	encoded, err := json.Marshal(wire(t))
	if err != nil {
		return nil, fmt.Errorf("encode turn: %w", err)
	}
	return encoded, nil
}

func (t *Turn) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode turn into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"turn",
		"id",
		"items",
		"itemsView",
		"status",
		"error",
		"startedAt",
		"completedAt",
		"durationMs",
	)
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, "turn", "id")
	if err != nil {
		return err
	}
	items, err := decodeRequiredThreadItemArray[ThreadItem](payload, "turn", "items")
	if err != nil {
		return err
	}
	itemsView, err := decodeRequiredThreadItemValue[TurnItemsView](payload, "turn", "itemsView")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[TurnStatus](payload, "turn", "status")
	if err != nil {
		return err
	}
	turnError, err := decodeRequiredNullableThreadItemValue[TurnError](payload, "turn", "error")
	if err != nil {
		return err
	}
	startedAt, err := decodeRequiredNullableThreadItemValue[int64](payload, "turn", "startedAt")
	if err != nil {
		return err
	}
	completedAt, err := decodeRequiredNullableThreadItemValue[int64](payload, "turn", "completedAt")
	if err != nil {
		return err
	}
	durationMS, err := decodeRequiredNullableThreadItemValue[int64](payload, "turn", "durationMs")
	if err != nil {
		return err
	}
	*t = Turn{
		ID:          id,
		Items:       items,
		ItemsView:   itemsView,
		Status:      status,
		Error:       turnError,
		StartedAt:   startedAt,
		CompletedAt: completedAt,
		DurationMS:  durationMS,
	}
	return nil
}
