package protocol

import (
	"encoding/json"
	"errors"
)

// W3cTraceContext is the standalone public distributed-tracing context used by
// JSON-RPC request envelopes. It does not alter Gollem's transport behavior.
type W3cTraceContext struct {
	Traceparent *string `json:"traceparent,omitempty"`
	Tracestate  *string `json:"tracestate,omitempty"`
}

func (c *W3cTraceContext) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode W3C trace context into nil receiver")
	}
	const objectName = "W3C trace context"
	payload, err := decodeRustSerdeObject(data, objectName, "traceparent", "tracestate")
	if err != nil {
		return err
	}
	traceparent, err := decodeOptionalNullableConfigValue[string](payload, objectName, "traceparent")
	if err != nil {
		return err
	}
	tracestate, err := decodeOptionalNullableConfigValue[string](payload, objectName, "tracestate")
	if err != nil {
		return err
	}
	*c = W3cTraceContext{Traceparent: traceparent, Tracestate: tracestate}
	return nil
}

func w3cTraceContextSchema() Schema {
	return Schema{
		"properties": Schema{
			"traceparent": Schema{"type": []any{"string", "null"}},
			"tracestate":  Schema{"type": []any{"string", "null"}},
		},
		"type": "object",
	}
}

var _ json.Unmarshaler = (*W3cTraceContext)(nil)
