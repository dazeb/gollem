package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// WarningNotification is the exact fixed public warning value. It remains
// standalone until Gollem has an exact live producer.
type WarningNotification struct {
	ThreadID *string `json:"threadId"`
	Message  string  `json:"message"`
}

func (n *WarningNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode warning notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "warning notification", "threadId", "message")
	if err != nil {
		return err
	}
	threadID, err := decodeOptionalNullableWarningThreadID(payload)
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, "warning notification", "message")
	if err != nil {
		return err
	}
	*n = WarningNotification{ThreadID: threadID, Message: message}
	return nil
}

func decodeOptionalNullableWarningThreadID(payload map[string]json.RawMessage) (*string, error) {
	raw, ok := payload["threadId"]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var threadID string
	if err := json.Unmarshal(raw, &threadID); err != nil {
		return nil, fmt.Errorf("decode warning notification threadId: %w", err)
	}
	return &threadID, nil
}

// GuardianWarningNotification is the exact fixed public guardian warning
// value. It remains standalone until Gollem has an exact live producer.
type GuardianWarningNotification struct {
	ThreadID string `json:"threadId"`
	Message  string `json:"message"`
}

func (n *GuardianWarningNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode guardian warning notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "guardian warning notification", "threadId", "message")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "guardian warning notification", "threadId")
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, "guardian warning notification", "message")
	if err != nil {
		return err
	}
	*n = GuardianWarningNotification{ThreadID: threadID, Message: message}
	return nil
}

// ErrorNotification is the exact fixed public turn-error notification. It
// remains standalone while Gollem's live runtime error envelope is
// wire-incompatible.
type ErrorNotification struct {
	Error     TurnError `json:"error"`
	WillRetry bool      `json:"willRetry"`
	ThreadID  string    `json:"threadId"`
	TurnID    string    `json:"turnId"`
}

func (n *ErrorNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode error notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"error notification",
		"error",
		"willRetry",
		"threadId",
		"turnId",
	)
	if err != nil {
		return err
	}
	turnError, err := decodeRequiredThreadItemValue[TurnError](payload, "error notification", "error")
	if err != nil {
		return err
	}
	willRetry, err := decodeRequiredThreadItemValue[bool](payload, "error notification", "willRetry")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "error notification", "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, "error notification", "turnId")
	if err != nil {
		return err
	}
	*n = ErrorNotification{
		Error: turnError, WillRetry: willRetry, ThreadID: threadID, TurnID: turnID,
	}
	return nil
}

var (
	_ json.Unmarshaler = (*WarningNotification)(nil)
	_ json.Unmarshaler = (*GuardianWarningNotification)(nil)
	_ json.Unmarshaler = (*ErrorNotification)(nil)
)
