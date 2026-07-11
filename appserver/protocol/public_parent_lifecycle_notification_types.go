package protocol

import (
	"encoding/json"
	"errors"
)

type ThreadStartedNotification struct {
	Thread Thread `json:"thread"`
}

func (n *ThreadStartedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread-started notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "thread-started notification", "thread")
	if err != nil {
		return err
	}
	thread, err := decodeRequiredThreadItemValue[Thread](payload, "thread-started notification", "thread")
	if err != nil {
		return err
	}
	*n = ThreadStartedNotification{Thread: thread}
	return nil
}

type ThreadStatusChangedNotification struct {
	ThreadID string       `json:"threadId"`
	Status   ThreadStatus `json:"status"`
}

func (n *ThreadStatusChangedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread-status-changed notification into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"thread-status-changed notification",
		"threadId",
		"status",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "thread-status-changed notification", "threadId")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[ThreadStatus](payload, "thread-status-changed notification", "status")
	if err != nil {
		return err
	}
	*n = ThreadStatusChangedNotification{ThreadID: threadID, Status: status}
	return nil
}

type TurnStartedNotification struct {
	ThreadID string `json:"threadId"`
	Turn     Turn   `json:"turn"`
}

func (n *TurnStartedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode turn-started notification into nil receiver")
	}
	threadID, turn, err := decodePublicTurnLifecycleNotification(data, "turn-started notification")
	if err != nil {
		return err
	}
	*n = TurnStartedNotification{ThreadID: threadID, Turn: turn}
	return nil
}

type TurnCompletedNotification struct {
	ThreadID string `json:"threadId"`
	Turn     Turn   `json:"turn"`
}

func (n *TurnCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode turn-completed notification into nil receiver")
	}
	threadID, turn, err := decodePublicTurnLifecycleNotification(data, "turn-completed notification")
	if err != nil {
		return err
	}
	*n = TurnCompletedNotification{ThreadID: threadID, Turn: turn}
	return nil
}

func decodePublicTurnLifecycleNotification(data []byte, objectName string) (string, Turn, error) {
	payload, err := decodeExactThreadItemObject(data, objectName, "threadId", "turn")
	if err != nil {
		return "", Turn{}, err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return "", Turn{}, err
	}
	turn, err := decodeRequiredThreadItemValue[Turn](payload, objectName, "turn")
	if err != nil {
		return "", Turn{}, err
	}
	return threadID, turn, nil
}

var (
	_ json.Unmarshaler = (*ThreadStartedNotification)(nil)
	_ json.Unmarshaler = (*ThreadStatusChangedNotification)(nil)
	_ json.Unmarshaler = (*TurnStartedNotification)(nil)
	_ json.Unmarshaler = (*TurnCompletedNotification)(nil)
)
