package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ThreadApproveGuardianDeniedActionParams is the standalone public guardian
// denied-action approval request. Gollem's deferred stub retains its legacy
// compatibility path and does not bind this source success contract.
type ThreadApproveGuardianDeniedActionParams struct {
	ThreadID string    `json:"threadId"`
	Event    JsonValue `json:"event"`
}

func (p *ThreadApproveGuardianDeniedActionParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-approve-guardian-denied-action params into nil receiver")
	}
	const objectName = "thread-approve-guardian-denied-action params"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "event")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	event, err := decodeRequiredGuardianDeniedActionEvent(payload, objectName)
	if err != nil {
		return err
	}
	*p = ThreadApproveGuardianDeniedActionParams{ThreadID: threadID, Event: event}
	return nil
}

func decodeRequiredGuardianDeniedActionEvent(payload map[string]json.RawMessage, objectName string) (JsonValue, error) {
	raw, ok := payload["event"]
	if !ok {
		return JsonValue{}, fmt.Errorf("%s requires event", objectName)
	}
	var event JsonValue
	if err := json.Unmarshal(raw, &event); err != nil {
		return JsonValue{}, fmt.Errorf("decode %s event: %w", objectName, err)
	}
	return event, nil
}

// ThreadApproveGuardianDeniedActionResponse is the exact empty source response.
// Gollem's deferred-stub runtime never returns it as a successful method result.
type ThreadApproveGuardianDeniedActionResponse struct{}

func (r *ThreadApproveGuardianDeniedActionResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-approve-guardian-denied-action response into nil receiver")
	}
	if _, err := decodeRustSerdeObject(data, "thread-approve-guardian-denied-action response"); err != nil {
		return err
	}
	*r = ThreadApproveGuardianDeniedActionResponse{}
	return nil
}

func threadApproveGuardianDeniedActionSchemas() map[string]Schema {
	return map[string]Schema{
		"ThreadApproveGuardianDeniedActionParams": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"event":    Schema{"description": "Serialized `codex_protocol::protocol::GuardianAssessmentEvent`."},
				"threadId": Schema{"type": "string"},
			},
			"required": []string{"event", "threadId"},
			"title":    "ThreadApproveGuardianDeniedActionParams",
			"type":     "object",
		},
		"ThreadApproveGuardianDeniedActionResponse": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"title":   "ThreadApproveGuardianDeniedActionResponse",
			"type":    "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*ThreadApproveGuardianDeniedActionParams)(nil)
	_ json.Unmarshaler = (*ThreadApproveGuardianDeniedActionResponse)(nil)
)
