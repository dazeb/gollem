package protocol

import (
	"encoding/json"
	"errors"
)

// ThreadRollbackParams is the exact public deprecated rollback request. It
// remains standalone because Gollem's executable history rollback accepts a
// legacy id alias and has stricter operational validation.
type ThreadRollbackParams struct {
	ThreadID string `json:"threadId"`
	NumTurns uint32 `json:"numTurns"`
}

func (p *ThreadRollbackParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-rollback params into nil receiver")
	}
	const objectName = "thread-rollback params"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "numTurns")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	numTurns, err := decodeRequiredThreadItemValue[uint32](payload, objectName, "numTurns")
	if err != nil {
		return err
	}
	*p = ThreadRollbackParams{ThreadID: threadID, NumTurns: numTurns}
	return nil
}

// ThreadRollbackResponse is the exact public deprecated rollback response. It
// remains standalone because Gollem returns an additional durable-history
// receipt rather than the public Thread projection.
type ThreadRollbackResponse struct {
	Thread Thread `json:"thread"`
}

func (r *ThreadRollbackResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-rollback response into nil receiver")
	}
	const objectName = "thread-rollback response"
	payload, err := decodeRustSerdeObject(data, objectName, "thread")
	if err != nil {
		return err
	}
	thread, err := decodeRequiredThreadItemValue[Thread](payload, objectName, "thread")
	if err != nil {
		return err
	}
	*r = ThreadRollbackResponse{Thread: thread}
	return nil
}

func threadRollbackSchemas() map[string]Schema {
	return map[string]Schema{
		"ThreadRollbackParams": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "DEPRECATED: `thread/rollback` will be removed soon.",
			"properties": Schema{
				"numTurns": Schema{
					"description": "The number of turns to drop from the end of the thread. Must be >= 1.\n\nThis only modifies the thread's history and does not revert local file changes that have been made by the agent. Clients are responsible for reverting these changes.",
					"format":      "uint32",
					"minimum":     0.0,
					"type":        "integer",
				},
				"threadId": Schema{"type": "string"},
			},
			"required": []string{"numTurns", "threadId"},
			"title":    "ThreadRollbackParams",
			"type":     "object",
		},
		"ThreadRollbackResponse": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"thread": Schema{
					"allOf":       []any{Schema{"$ref": "#/$defs/Thread"}},
					"description": "The updated thread after applying the rollback, with `turns` populated.\n\nThe ThreadItems stored in each Turn are lossy since we explicitly do not persist all agent interactions, such as command executions. This is the same behavior as `thread/resume`.",
				},
			},
			"required": []string{"thread"},
			"title":    "ThreadRollbackResponse",
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*ThreadRollbackParams)(nil)
	_ json.Unmarshaler = (*ThreadRollbackResponse)(nil)
)
