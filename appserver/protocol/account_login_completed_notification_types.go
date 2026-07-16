package protocol

import (
	"encoding/json"
	"errors"
)

// AccountLoginCompletedNotification is exact standalone public login-result
// data. The corresponding notification remains deferred and unbound.
type AccountLoginCompletedNotification struct {
	LoginID *string `json:"loginId,omitempty"`
	Success bool    `json:"success"`
	Error   *string `json:"error,omitempty"`
}

func (n AccountLoginCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"loginId": n.LoginID,
		"success": n.Success,
		"error":   n.Error,
	})
}

func (n *AccountLoginCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode account login completed notification into nil receiver")
	}
	const objectName = "account login completed notification"
	payload, err := decodeRustSerdeObject(data, objectName, "loginId", "success", "error")
	if err != nil {
		return err
	}
	loginID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "loginId")
	if err != nil {
		return err
	}
	success, err := decodeRequiredThreadItemValue[bool](payload, objectName, "success")
	if err != nil {
		return err
	}
	errorMessage, err := decodeOptionalNullableConfigValue[string](payload, objectName, "error")
	if err != nil {
		return err
	}
	*n = AccountLoginCompletedNotification{
		LoginID: loginID,
		Success: success,
		Error:   errorMessage,
	}
	return nil
}

var (
	_ json.Marshaler   = AccountLoginCompletedNotification{}
	_ json.Unmarshaler = (*AccountLoginCompletedNotification)(nil)
)
