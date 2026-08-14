package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// JSONRPCNotification is the exact standalone public notification envelope.
// It is distinct from Gollem's method-specific notification dispatch.
type JSONRPCNotification struct {
	Method string     `json:"method"`
	Params *JsonValue `json:"params,omitempty"`
}

// JSONRPCRequest is the exact standalone public request envelope. It does not
// alter Gollem's existing transport or request dispatch behavior.
type JSONRPCRequest struct {
	ID     RequestId        `json:"id"`
	Method string           `json:"method"`
	Params *JsonValue       `json:"params,omitempty"`
	Trace  *W3cTraceContext `json:"trace,omitempty"`
}

// JSONRPCResponse is the exact standalone public successful response envelope.
// It remains distinct from Gollem's runtime response implementation type.
type JSONRPCResponse struct {
	ID     RequestId `json:"id"`
	Result JsonValue `json:"result"`
}

func (n *JSONRPCNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode JSON-RPC notification into nil receiver")
	}
	const objectName = "JSON-RPC notification"
	payload, err := decodeRustSerdeObject(data, objectName, "method", "params")
	if err != nil {
		return err
	}
	method, err := decodeRequiredThreadItemValue[string](payload, objectName, "method")
	if err != nil {
		return err
	}
	params, err := decodeOptionalJSONRPCEnvelopeValue(payload, objectName, "params")
	if err != nil {
		return err
	}
	*n = JSONRPCNotification{Method: method, Params: params}
	return nil
}

func (r *JSONRPCRequest) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode JSON-RPC request into nil receiver")
	}
	const objectName = "JSON-RPC request"
	payload, err := decodeRustSerdeObject(data, objectName, "id", "method", "params", "trace")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[RequestId](payload, objectName, "id")
	if err != nil {
		return err
	}
	method, err := decodeRequiredThreadItemValue[string](payload, objectName, "method")
	if err != nil {
		return err
	}
	params, err := decodeOptionalJSONRPCEnvelopeValue(payload, objectName, "params")
	if err != nil {
		return err
	}
	trace, err := decodeOptionalNullableConfigValue[W3cTraceContext](payload, objectName, "trace")
	if err != nil {
		return err
	}
	*r = JSONRPCRequest{ID: id, Method: method, Params: params, Trace: trace}
	return nil
}

func (r *JSONRPCResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode JSON-RPC response into nil receiver")
	}
	const objectName = "JSON-RPC response"
	payload, err := decodeRustSerdeObject(data, objectName, "id", "result")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[RequestId](payload, objectName, "id")
	if err != nil {
		return err
	}
	result, err := decodeRequiredThreadItemJSONValue(payload, objectName, "result")
	if err != nil {
		return err
	}
	*r = JSONRPCResponse{ID: id, Result: result}
	return nil
}

func decodeOptionalJSONRPCEnvelopeValue(
	payload map[string]json.RawMessage,
	objectName, fieldName string,
) (*JsonValue, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value JsonValue
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func jsonRPCEnvelopeSchemas() map[string]Schema {
	return map[string]Schema{
		"JSONRPCNotification": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "A notification which does not expect a response.",
			"properties": Schema{
				"method": Schema{"type": "string"},
				"params": Schema{},
			},
			"required": []string{"method"},
			"title":    "JSONRPCNotification",
			"type":     "object",
		},
		"JSONRPCRequest": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "A request that expects a response.",
			"properties": Schema{
				"id":     Schema{"$ref": "#/$defs/RequestId"},
				"method": Schema{"type": "string"},
				"params": Schema{},
				"trace": Schema{
					"anyOf": []any{
						Schema{"$ref": "#/$defs/W3cTraceContext"},
						Schema{"type": "null"},
					},
					"description": "Optional W3C Trace Context for distributed tracing.",
				},
			},
			"required": []string{"id", "method"},
			"title":    "JSONRPCRequest",
			"type":     "object",
		},
		"JSONRPCResponse": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "A successful (non-error) response to a request.",
			"properties": Schema{
				"id":     Schema{"$ref": "#/$defs/RequestId"},
				"result": Schema{},
			},
			"required": []string{"id", "result"},
			"title":    "JSONRPCResponse",
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*JSONRPCNotification)(nil)
	_ json.Unmarshaler = (*JSONRPCRequest)(nil)
	_ json.Unmarshaler = (*JSONRPCResponse)(nil)
)
