package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSONRPCError is the exact standalone public error response envelope. It is
// distinct from Gollem's runtime Error and Response implementation types.
type JSONRPCError struct {
	Error JSONRPCErrorError `json:"error"`
	ID    RequestId         `json:"id"`
}

// JSONRPCErrorError is the public error payload within JSONRPCError.
type JSONRPCErrorError struct {
	Code    int64      `json:"code"`
	Data    *JsonValue `json:"data,omitempty"`
	Message string     `json:"message"`
}

func (e *JSONRPCError) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode JSON-RPC error into nil receiver")
	}
	const objectName = "JSON-RPC error"
	payload, err := decodeRustSerdeObject(data, objectName, "error", "id")
	if err != nil {
		return err
	}
	errorPayload, err := decodeRequiredThreadItemValue[JSONRPCErrorError](payload, objectName, "error")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[RequestId](payload, objectName, "id")
	if err != nil {
		return err
	}
	*e = JSONRPCError{Error: errorPayload, ID: id}
	return nil
}

func (e *JSONRPCErrorError) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode JSON-RPC error payload into nil receiver")
	}
	const objectName = "JSON-RPC error payload"
	payload, err := decodeRustSerdeObject(data, objectName, "code", "data", "message")
	if err != nil {
		return err
	}
	code, err := decodeRequiredThreadItemValue[int64](payload, objectName, "code")
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, objectName, "message")
	if err != nil {
		return err
	}
	dataValue, err := decodeOptionalJSONRPCErrorData(payload, objectName)
	if err != nil {
		return err
	}
	*e = JSONRPCErrorError{Code: code, Data: dataValue, Message: message}
	return nil
}

func decodeOptionalJSONRPCErrorData(payload map[string]json.RawMessage, objectName string) (*JsonValue, error) {
	raw, ok := payload["data"]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value JsonValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s data: %w", objectName, err)
	}
	return &value, nil
}

func jsonRPCErrorSchemas() map[string]Schema {
	return map[string]Schema{
		"JSONRPCError": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "A response to a request that indicates an error occurred.",
			"properties": Schema{
				"error": Schema{"$ref": "#/$defs/JSONRPCErrorError"},
				"id":    Schema{"$ref": "#/$defs/RequestId"},
			},
			"required": []string{"error", "id"},
			"title":    "JSONRPCError",
			"type":     "object",
		},
		"JSONRPCErrorError": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"code":    Schema{"format": "int64", "type": "integer"},
				"data":    Schema{},
				"message": Schema{"type": "string"},
			},
			"required": []string{"code", "message"},
			"title":    "JSONRPCErrorError",
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*JSONRPCError)(nil)
	_ json.Unmarshaler = (*JSONRPCErrorError)(nil)
)
