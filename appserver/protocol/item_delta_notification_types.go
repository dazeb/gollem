package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

type AgentMessageDeltaNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

func (n *AgentMessageDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode agent-message delta into nil receiver")
	}
	value, _, err := decodePublicItemDelta(data, "agent-message delta", "", true)
	if err != nil {
		return err
	}
	*n = AgentMessageDeltaNotification(value)
	return nil
}

// PlanDeltaNotification is the exact experimental plan-item delta value. It
// remains standalone until Gollem has an exact producer.
type PlanDeltaNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

func (n *PlanDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode plan delta into nil receiver")
	}
	value, _, err := decodePublicItemDelta(data, "plan delta", "", true)
	if err != nil {
		return err
	}
	*n = PlanDeltaNotification(value)
	return nil
}

type ReasoningSummaryPartAddedNotification struct {
	ThreadID     string `json:"threadId"`
	TurnID       string `json:"turnId"`
	ItemID       string `json:"itemId"`
	SummaryIndex int64  `json:"summaryIndex"`
}

func (n *ReasoningSummaryPartAddedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode reasoning summary part into nil receiver")
	}
	value, index, err := decodePublicItemDelta(data, "reasoning summary part", "summaryIndex", false)
	if err != nil {
		return err
	}
	*n = ReasoningSummaryPartAddedNotification{
		ThreadID: value.ThreadID, TurnID: value.TurnID, ItemID: value.ItemID, SummaryIndex: index,
	}
	return nil
}

type ReasoningSummaryTextDeltaNotification struct {
	ThreadID     string `json:"threadId"`
	TurnID       string `json:"turnId"`
	ItemID       string `json:"itemId"`
	Delta        string `json:"delta"`
	SummaryIndex int64  `json:"summaryIndex"`
}

func (n *ReasoningSummaryTextDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode reasoning summary text delta into nil receiver")
	}
	value, index, err := decodePublicItemDelta(data, "reasoning summary text delta", "summaryIndex", true)
	if err != nil {
		return err
	}
	*n = ReasoningSummaryTextDeltaNotification{
		ThreadID: value.ThreadID, TurnID: value.TurnID, ItemID: value.ItemID,
		Delta: value.Delta, SummaryIndex: index,
	}
	return nil
}

type ReasoningTextDeltaNotification struct {
	ThreadID     string `json:"threadId"`
	TurnID       string `json:"turnId"`
	ItemID       string `json:"itemId"`
	Delta        string `json:"delta"`
	ContentIndex int64  `json:"contentIndex"`
}

func (n *ReasoningTextDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode reasoning text delta into nil receiver")
	}
	value, index, err := decodePublicItemDelta(data, "reasoning text delta", "contentIndex", true)
	if err != nil {
		return err
	}
	*n = ReasoningTextDeltaNotification{
		ThreadID: value.ThreadID, TurnID: value.TurnID, ItemID: value.ItemID,
		Delta: value.Delta, ContentIndex: index,
	}
	return nil
}

type publicItemDeltaFields struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

func decodePublicItemDelta(
	data []byte,
	name string,
	indexField string,
	requireDelta bool,
) (publicItemDeltaFields, int64, error) {
	fields := []string{"threadId", "turnId", "itemId"}
	if requireDelta {
		fields = append(fields, "delta")
	}
	if indexField != "" {
		fields = append(fields, indexField)
	}
	payload, err := decodeExactThreadItemObject(data, name, fields...)
	if err != nil {
		return publicItemDeltaFields{}, 0, err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, name, "threadId")
	if err != nil {
		return publicItemDeltaFields{}, 0, err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, name, "turnId")
	if err != nil {
		return publicItemDeltaFields{}, 0, err
	}
	itemID, err := decodeRequiredThreadItemValue[string](payload, name, "itemId")
	if err != nil {
		return publicItemDeltaFields{}, 0, err
	}
	delta := ""
	if requireDelta {
		delta, err = decodeRequiredThreadItemValue[string](payload, name, "delta")
		if err != nil {
			return publicItemDeltaFields{}, 0, err
		}
	}
	var index int64
	if indexField != "" {
		index, err = decodeRequiredThreadItemValue[int64](payload, name, indexField)
		if err != nil {
			return publicItemDeltaFields{}, 0, fmt.Errorf("decode %s: %w", indexField, err)
		}
	}
	return publicItemDeltaFields{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, Delta: delta,
	}, index, nil
}

var (
	_ json.Unmarshaler = (*AgentMessageDeltaNotification)(nil)
	_ json.Unmarshaler = (*PlanDeltaNotification)(nil)
	_ json.Unmarshaler = (*ReasoningSummaryPartAddedNotification)(nil)
	_ json.Unmarshaler = (*ReasoningSummaryTextDeltaNotification)(nil)
	_ json.Unmarshaler = (*ReasoningTextDeltaNotification)(nil)
)
