package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ClientNotification is the exact standalone public client notification union.
// Its only source variant is the parameterless initialized lifecycle signal.
type ClientNotification struct{}

func (ClientNotification) MarshalJSON() ([]byte, error) {
	return []byte(`{"method":"initialized"}`), nil
}

func (n *ClientNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode client notification into nil receiver")
	}
	const objectName = "client notification"
	payload, err := decodeRustSerdeObject(data, objectName, "method")
	if err != nil {
		return err
	}
	method, err := decodeRequiredThreadItemValue[string](payload, objectName, "method")
	if err != nil {
		return err
	}
	if method != "initialized" {
		return fmt.Errorf("decode %s method %q: want initialized", objectName, method)
	}
	*n = ClientNotification{}
	return nil
}

func clientNotificationSchema() Schema {
	return Schema{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"oneOf": []any{
			Schema{
				"properties": Schema{
					"method": Schema{
						"enum":  []any{"initialized"},
						"title": "InitializedNotificationMethod",
						"type":  "string",
					},
				},
				"required": []string{"method"},
				"title":    "InitializedNotification",
				"type":     "object",
			},
		},
		"title": "ClientNotification",
	}
}

var (
	_ json.Marshaler   = ClientNotification{}
	_ json.Unmarshaler = (*ClientNotification)(nil)
)
