package protocol

import (
	"encoding/json"
	"errors"
)

// TerminalInteractionNotification is the public terminal-input event shape.
// The source notification method remains blocked in Gollem because no runtime
// producer emits this exact event yet.
type TerminalInteractionNotification struct {
	ThreadID  string `json:"threadId"`
	TurnID    string `json:"turnId"`
	ItemID    string `json:"itemId"`
	ProcessID string `json:"processId"`
	Stdin     string `json:"stdin"`
}

func (n *TerminalInteractionNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode terminal interaction notification into nil receiver")
	}
	const objectName = "terminal interaction notification"
	payload, err := decodeRustSerdeObject(
		data, objectName, "threadId", "turnId", "itemId", "processId", "stdin",
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
	processID, err := decodeRequiredThreadItemValue[string](payload, objectName, "processId")
	if err != nil {
		return err
	}
	stdin, err := decodeRequiredThreadItemValue[string](payload, objectName, "stdin")
	if err != nil {
		return err
	}
	*n = TerminalInteractionNotification{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, ProcessID: processID, Stdin: stdin,
	}
	return nil
}

func terminalInteractionNotificationSchema() Schema {
	return Schema{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"additionalProperties": true,
		"x-gollem-typescript-ignore-additional-properties": true,
		"properties": Schema{
			"itemId":    Schema{"type": "string"},
			"processId": Schema{"type": "string"},
			"stdin":     Schema{"type": "string"},
			"threadId":  Schema{"type": "string"},
			"turnId":    Schema{"type": "string"},
		},
		"required": []string{"itemId", "processId", "stdin", "threadId", "turnId"},
		"title":    "TerminalInteractionNotification",
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*TerminalInteractionNotification)(nil)
