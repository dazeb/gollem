package protocol

import (
	"encoding/json"
	"errors"
)

// RawResponseItemCompletedNotification is the exact public raw model-response
// record. It remains unbound until Gollem has a truthful runtime producer.
type RawResponseItemCompletedNotification struct {
	ThreadID string       `json:"threadId"`
	TurnID   string       `json:"turnId"`
	Item     ResponseItem `json:"item"`
}

func (n RawResponseItemCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadID string       `json:"threadId"`
		TurnID   string       `json:"turnId"`
		Item     ResponseItem `json:"item"`
	}{
		ThreadID: n.ThreadID,
		TurnID:   n.TurnID,
		Item:     n.Item,
	})
}

func (n *RawResponseItemCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode raw-response-item-completed notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data, "raw-response-item-completed notification", "threadId", "turnId", "item",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](
		payload, "raw-response-item-completed notification", "threadId",
	)
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](
		payload, "raw-response-item-completed notification", "turnId",
	)
	if err != nil {
		return err
	}
	item, err := decodeRequiredThreadItemValue[ResponseItem](
		payload, "raw-response-item-completed notification", "item",
	)
	if err != nil {
		return err
	}
	*n = RawResponseItemCompletedNotification{ThreadID: threadID, TurnID: turnID, Item: item}
	return nil
}
