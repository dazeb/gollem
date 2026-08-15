package protocol

import (
	"encoding/json"
	"errors"
)

// JSONRPCMessage is the exact standalone public untagged JSON-RPC message
// union. It stays separate from Gollem's live transport and dispatch paths.
type JSONRPCMessage struct {
	Request      *JSONRPCRequest      `json:"-"`
	Notification *JSONRPCNotification `json:"-"`
	Response     *JSONRPCResponse     `json:"-"`
	Error        *JSONRPCError        `json:"-"`
}

func (m JSONRPCMessage) MarshalJSON() ([]byte, error) {
	variants := 0
	if m.Request != nil {
		variants++
	}
	if m.Notification != nil {
		variants++
	}
	if m.Response != nil {
		variants++
	}
	if m.Error != nil {
		variants++
	}
	if variants != 1 {
		return nil, errors.New("JSON-RPC message requires exactly one variant")
	}
	switch {
	case m.Request != nil:
		return json.Marshal(m.Request)
	case m.Notification != nil:
		return json.Marshal(m.Notification)
	case m.Response != nil:
		return json.Marshal(m.Response)
	default:
		return json.Marshal(m.Error)
	}
}

func (m *JSONRPCMessage) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode JSON-RPC message into nil receiver")
	}

	// serde's untagged enum resolves compatible variants in declaration order.
	var request JSONRPCRequest
	if err := json.Unmarshal(data, &request); err == nil {
		*m = JSONRPCMessage{Request: &request}
		return nil
	}
	var notification JSONRPCNotification
	if err := json.Unmarshal(data, &notification); err == nil {
		*m = JSONRPCMessage{Notification: &notification}
		return nil
	}
	var response JSONRPCResponse
	if err := json.Unmarshal(data, &response); err == nil {
		*m = JSONRPCMessage{Response: &response}
		return nil
	}
	var responseError JSONRPCError
	if err := json.Unmarshal(data, &responseError); err == nil {
		*m = JSONRPCMessage{Error: &responseError}
		return nil
	}
	return errors.New("decode JSON-RPC message: no compatible variant")
}

func jsonRPCMessageSchema() Schema {
	return Schema{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"anyOf": []any{
			Schema{"$ref": "#/$defs/JSONRPCRequest"},
			Schema{"$ref": "#/$defs/JSONRPCNotification"},
			Schema{"$ref": "#/$defs/JSONRPCResponse"},
			Schema{"$ref": "#/$defs/JSONRPCError"},
		},
		"description": "Refers to any valid JSON-RPC object that can be decoded off the wire, or encoded to be sent.",
		"title":       "JSONRPCMessage",
	}
}

var (
	_ json.Marshaler   = JSONRPCMessage{}
	_ json.Unmarshaler = (*JSONRPCMessage)(nil)
)
