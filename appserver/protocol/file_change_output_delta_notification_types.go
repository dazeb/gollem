package protocol

import (
	"encoding/json"
	"errors"
)

// FileChangeOutputDeltaNotification is the exact deprecated public textual
// apply-patch output value. It remains standalone because the server no longer
// emits item/fileChange/outputDelta.
type FileChangeOutputDeltaNotification struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	ItemID   string `json:"itemId"`
	Delta    string `json:"delta"`
}

func (n *FileChangeOutputDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode file-change output delta into nil receiver")
	}
	const objectName = "file-change output delta"
	payload, err := decodeRustSerdeObject(
		data, objectName, "threadId", "turnId", "itemId", "delta",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, objectName, "turnId")
	if err != nil {
		return err
	}
	itemID, err := decodeRequiredThreadItemValue[string](payload, objectName, "itemId")
	if err != nil {
		return err
	}
	delta, err := decodeRequiredThreadItemValue[string](payload, objectName, "delta")
	if err != nil {
		return err
	}
	*n = FileChangeOutputDeltaNotification{
		ThreadID: threadID,
		TurnID:   turnID,
		ItemID:   itemID,
		Delta:    delta,
	}
	return nil
}

var _ json.Unmarshaler = (*FileChangeOutputDeltaNotification)(nil)
