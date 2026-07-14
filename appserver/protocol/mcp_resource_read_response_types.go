package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// McpResourceReadResponse is the exact public MCP resource-content envelope.
// It remains separate from Gollem's broader live result until an adapter exists.
type McpResourceReadResponse struct {
	Contents []ResourceContent `json:"contents" jsonschema:"nonnullable=true"`
}

func (r McpResourceReadResponse) MarshalJSON() ([]byte, error) {
	contents := r.Contents
	if contents == nil {
		contents = []ResourceContent{}
	}
	return json.Marshal(struct {
		Contents []ResourceContent `json:"contents"`
	}{Contents: contents})
}

func (r *McpResourceReadResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP resource-read response into nil receiver")
	}
	payload, err := decodeMcpResourceReadResponseObject(data)
	if err != nil {
		return err
	}
	contents, err := decodeRequiredThreadItemValue[[]ResourceContent](
		payload,
		"MCP resource-read response",
		"contents",
	)
	if err != nil {
		return err
	}
	*r = McpResourceReadResponse{Contents: contents}
	return nil
}

func decodeMcpResourceReadResponseObject(data []byte) (map[string]json.RawMessage, error) {
	const objectName = "MCP resource-read response"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	payload := make(map[string]json.RawMessage, 1)
	seenContents := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode %s field name: %w", objectName, err)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode %s field %q: %w", objectName, name, err)
		}
		if name != "contents" {
			continue
		}
		if seenContents {
			return nil, fmt.Errorf("duplicate %s field %q", objectName, name)
		}
		seenContents = true
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%s must contain one JSON value", objectName)
		}
		return nil, fmt.Errorf("decode %s trailing value: %w", objectName, err)
	}
	return payload, nil
}

var (
	_ json.Marshaler   = McpResourceReadResponse{}
	_ json.Unmarshaler = (*McpResourceReadResponse)(nil)
)
