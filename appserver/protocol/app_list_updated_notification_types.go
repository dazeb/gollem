package protocol

import (
	"encoding/json"
	"errors"
)

// AppListUpdatedNotification is an exact standalone app-list snapshot value.
// Production, delivery, and app discovery remain deferred.
type AppListUpdatedNotification struct {
	Data []AppInfo `json:"data" jsonschema:"nonnullable=true"`
}

func (n AppListUpdatedNotification) MarshalJSON() ([]byte, error) {
	if n.Data == nil {
		return nil, errors.New("app-list updated notification data cannot be null")
	}
	type wire AppListUpdatedNotification
	return json.Marshal(wire(n))
}

func (n *AppListUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode app-list updated notification into nil receiver")
	}
	const objectName = "app-list updated notification"
	payload, err := decodeRustSerdeObject(data, objectName, "data")
	if err != nil {
		return err
	}
	apps, err := decodeRequiredThreadItemArray[AppInfo](payload, objectName, "data")
	if err != nil {
		return err
	}
	*n = AppListUpdatedNotification{Data: apps}
	return nil
}

var (
	_ json.Marshaler   = AppListUpdatedNotification{}
	_ json.Unmarshaler = (*AppListUpdatedNotification)(nil)
)
