package protocol

import (
	"encoding/json"
	"errors"
)

// ThreadInjectItemsParams is the exact public request for appending raw
// Responses API items to a thread. It remains standalone while Gollem's live
// handler also accepts the legacy id alias.
type ThreadInjectItemsParams struct {
	ThreadID string            `json:"threadId"`
	Items    []json.RawMessage `json:"items"`
}

func (p ThreadInjectItemsParams) MarshalJSON() ([]byte, error) {
	items := p.Items
	if items == nil {
		items = []json.RawMessage{}
	}
	return json.Marshal(struct {
		ThreadID string            `json:"threadId"`
		Items    []json.RawMessage `json:"items"`
	}{ThreadID: p.ThreadID, Items: items})
}

func (p *ThreadInjectItemsParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-inject-items params into nil receiver")
	}
	const objectName = "thread-inject-items params"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "items")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	items, err := decodeRequiredThreadItemValue[[]json.RawMessage](payload, objectName, "items")
	if err != nil {
		return err
	}
	*p = ThreadInjectItemsParams{ThreadID: threadID, Items: items}
	return nil
}

// ThreadInjectItemsResponse is the exact empty public response. It remains
// standalone while Gollem's live request handler preserves its wider input
// compatibility contract.
type ThreadInjectItemsResponse struct{}

func (r *ThreadInjectItemsResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-inject-items response into nil receiver")
	}
	if _, err := decodeRustSerdeObject(data, "thread-inject-items response"); err != nil {
		return err
	}
	*r = ThreadInjectItemsResponse{}
	return nil
}

func threadInjectItemsSchemas() map[string]Schema {
	return map[string]Schema{
		"ThreadInjectItemsParams": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"items": Schema{
					"description": "Raw Responses API items to append to the thread's model-visible history.",
					"items":       true,
					"type":        "array",
				},
				"threadId": Schema{"type": "string"},
			},
			"required": []string{"items", "threadId"},
			"title":    "ThreadInjectItemsParams",
			"type":     "object",
		},
		"ThreadInjectItemsResponse": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"title":   "ThreadInjectItemsResponse",
			"type":    "object",
		},
	}
}

var (
	_ json.Marshaler   = ThreadInjectItemsParams{}
	_ json.Unmarshaler = (*ThreadInjectItemsParams)(nil)
	_ json.Unmarshaler = (*ThreadInjectItemsResponse)(nil)
)
