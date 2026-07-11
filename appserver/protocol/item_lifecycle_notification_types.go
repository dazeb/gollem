package protocol

import (
	"encoding/json"
	"errors"
)

// ItemStartedNotification is the exact public lifecycle-start payload. Gollem's
// existing runtime-specific payloads remain separate compatibility variants.
type ItemStartedNotification struct {
	Item        ThreadItem `json:"item"`
	ThreadID    string     `json:"threadId"`
	TurnID      string     `json:"turnId"`
	StartedAtMS int64      `json:"startedAtMs"`
}

func (n ItemStartedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Item        ThreadItem `json:"item"`
		ThreadID    string     `json:"threadId"`
		TurnID      string     `json:"turnId"`
		StartedAtMS int64      `json:"startedAtMs"`
	}{
		Item:        n.Item,
		ThreadID:    n.ThreadID,
		TurnID:      n.TurnID,
		StartedAtMS: n.StartedAtMS,
	})
}

func (n *ItemStartedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode item-started notification into nil receiver")
	}
	item, threadID, turnID, startedAtMS, err := decodeExactItemLifecycleNotification(
		data, "item-started notification", "startedAtMs",
	)
	if err != nil {
		return err
	}
	*n = ItemStartedNotification{
		Item:        item,
		ThreadID:    threadID,
		TurnID:      turnID,
		StartedAtMS: startedAtMS,
	}
	return nil
}

// ItemCompletedNotification is the exact public lifecycle-completion payload.
type ItemCompletedNotification struct {
	Item          ThreadItem `json:"item"`
	ThreadID      string     `json:"threadId"`
	TurnID        string     `json:"turnId"`
	CompletedAtMS int64      `json:"completedAtMs"`
}

func (n ItemCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Item          ThreadItem `json:"item"`
		ThreadID      string     `json:"threadId"`
		TurnID        string     `json:"turnId"`
		CompletedAtMS int64      `json:"completedAtMs"`
	}{
		Item:          n.Item,
		ThreadID:      n.ThreadID,
		TurnID:        n.TurnID,
		CompletedAtMS: n.CompletedAtMS,
	})
}

func (n *ItemCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode item-completed notification into nil receiver")
	}
	item, threadID, turnID, completedAtMS, err := decodeExactItemLifecycleNotification(
		data, "item-completed notification", "completedAtMs",
	)
	if err != nil {
		return err
	}
	*n = ItemCompletedNotification{
		Item:          item,
		ThreadID:      threadID,
		TurnID:        turnID,
		CompletedAtMS: completedAtMS,
	}
	return nil
}

func decodeExactItemLifecycleNotification(
	data []byte,
	objectName string,
	timestampField string,
) (ThreadItem, string, string, int64, error) {
	payload, err := decodeExactThreadItemObject(
		data, objectName, "item", "threadId", "turnId", timestampField,
	)
	if err != nil {
		return ThreadItem{}, "", "", 0, err
	}
	item, err := decodeRequiredThreadItemValue[ThreadItem](payload, objectName, "item")
	if err != nil {
		return ThreadItem{}, "", "", 0, err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return ThreadItem{}, "", "", 0, err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, objectName, "turnId")
	if err != nil {
		return ThreadItem{}, "", "", 0, err
	}
	timestamp, err := decodeRequiredThreadItemValue[int64](payload, objectName, timestampField)
	if err != nil {
		return ThreadItem{}, "", "", 0, err
	}
	return item, threadID, turnID, timestamp, nil
}
