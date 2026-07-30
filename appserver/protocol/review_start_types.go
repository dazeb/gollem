package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ReviewDelivery is the exact public placement for a requested review. It is
// standalone from Gollem because review execution has no compatible runtime.
type ReviewDelivery string

const (
	ReviewDeliveryInline   ReviewDelivery = "inline"
	ReviewDeliveryDetached ReviewDelivery = "detached"
)

func (d ReviewDelivery) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(d, "review delivery", ReviewDelivery.valid)
}

func (d *ReviewDelivery) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, d, "review delivery", ReviewDelivery.valid)
}

func (d ReviewDelivery) valid() bool {
	return d == ReviewDeliveryInline || d == ReviewDeliveryDetached
}

// ReviewTarget is the exact tagged public union used by review/start. Keeping
// the raw canonical form prevents the standalone contract from implying that
// Gollem can execute a review.
type ReviewTarget struct {
	raw json.RawMessage
}

func (t ReviewTarget) MarshalJSON() ([]byte, error) {
	if len(t.raw) == 0 {
		return nil, errors.New("review target has no value")
	}
	return canonicalReviewTargetJSON(t.raw)
}

func (t *ReviewTarget) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode review target into nil receiver")
	}
	canonical, err := canonicalReviewTargetJSON(data)
	if err != nil {
		return err
	}
	t.raw = canonical
	return nil
}

func canonicalReviewTargetJSON(data []byte) (json.RawMessage, error) {
	const objectName = "review target"
	tagPayload, err := decodeRustSerdeObject(data, objectName, "type")
	if err != nil {
		return nil, err
	}
	targetType, err := decodeRequiredThreadItemValue[string](tagPayload, objectName, "type")
	if err != nil {
		return nil, err
	}

	switch targetType {
	case "uncommittedChanges":
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: targetType})
	case "baseBranch":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "branch")
		if err != nil {
			return nil, err
		}
		branch, err := decodeRequiredThreadItemValue[string](payload, objectName, "branch")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type   string `json:"type"`
			Branch string `json:"branch"`
		}{Type: targetType, Branch: branch})
	case "commit":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "sha", "title")
		if err != nil {
			return nil, err
		}
		sha, err := decodeRequiredThreadItemValue[string](payload, objectName, "sha")
		if err != nil {
			return nil, err
		}
		title, err := decodeOptionalNullableConfigValue[string](payload, objectName, "title")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type  string  `json:"type"`
			SHA   string  `json:"sha"`
			Title *string `json:"title"`
		}{Type: targetType, SHA: sha, Title: title})
	case "custom":
		payload, err := decodeRustSerdeObject(data, objectName, "type", "instructions")
		if err != nil {
			return nil, err
		}
		instructions, err := decodeRequiredThreadItemValue[string](payload, objectName, "instructions")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type         string `json:"type"`
			Instructions string `json:"instructions"`
		}{Type: targetType, Instructions: instructions})
	default:
		return nil, fmt.Errorf("unknown review target type %q", targetType)
	}
}

// ReviewStartParams is the exact public review-start request. It does not bind
// to Gollem's protocol registry until an execution path can honor this shape.
type ReviewStartParams struct {
	ThreadID string          `json:"threadId"`
	Target   ReviewTarget    `json:"target"`
	Delivery *ReviewDelivery `json:"delivery"`
}

func (p ReviewStartParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadID string          `json:"threadId"`
		Target   ReviewTarget    `json:"target"`
		Delivery *ReviewDelivery `json:"delivery"`
	}{ThreadID: p.ThreadID, Target: p.Target, Delivery: p.Delivery})
}

func (p *ReviewStartParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode review-start params into nil receiver")
	}
	const objectName = "review-start params"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "target", "delivery")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	target, err := decodeRequiredThreadItemValue[ReviewTarget](payload, objectName, "target")
	if err != nil {
		return err
	}
	delivery, err := decodeOptionalNullableConfigValue[ReviewDelivery](payload, objectName, "delivery")
	if err != nil {
		return err
	}
	*p = ReviewStartParams{ThreadID: threadID, Target: target, Delivery: delivery}
	return nil
}

// ReviewStartResponse is the exact public review-start response. It does not
// imply a running review or change any existing live result binding.
type ReviewStartResponse struct {
	Turn           Turn   `json:"turn"`
	ReviewThreadID string `json:"reviewThreadId"`
}

func (r *ReviewStartResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode review-start response into nil receiver")
	}
	const objectName = "review-start response"
	payload, err := decodeRustSerdeObject(data, objectName, "turn", "reviewThreadId")
	if err != nil {
		return err
	}
	turn, err := decodeRequiredThreadItemValue[Turn](payload, objectName, "turn")
	if err != nil {
		return err
	}
	reviewThreadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "reviewThreadId")
	if err != nil {
		return err
	}
	*r = ReviewStartResponse{Turn: turn, ReviewThreadID: reviewThreadID}
	return nil
}

func reviewStartSchemas() map[string]Schema {
	return map[string]Schema{
		"ReviewDelivery": stringEnumSchema("inline", "detached"),
		"ReviewTarget": {
			"oneOf": []any{
				Schema{
					"type": "object",
					"properties": Schema{
						"type": Schema{"type": "string", "enum": []any{"uncommittedChanges"}},
					},
					"required": []string{"type"},
				},
				Schema{
					"type": "object",
					"properties": Schema{
						"branch": Schema{"type": "string"},
						"type":   Schema{"type": "string", "enum": []any{"baseBranch"}},
					},
					"required": []string{"branch", "type"},
				},
				Schema{
					"type": "object",
					"properties": Schema{
						"sha":   Schema{"type": "string"},
						"title": Schema{"type": []any{"string", "null"}},
						"type":  Schema{"type": "string", "enum": []any{"commit"}},
					},
					"required": []string{"sha", "type"},
				},
				Schema{
					"type": "object",
					"properties": Schema{
						"instructions": Schema{"type": "string"},
						"type":         Schema{"type": "string", "enum": []any{"custom"}},
					},
					"required": []string{"instructions", "type"},
				},
			},
		},
		"ReviewStartParams": {
			"type": "object",
			"properties": Schema{
				"delivery": Schema{"anyOf": []any{
					Schema{"$ref": "#/$defs/ReviewDelivery"}, Schema{"type": "null"},
				}},
				"target":   Schema{"$ref": "#/$defs/ReviewTarget"},
				"threadId": Schema{"type": "string"},
			},
			"required": []string{"target", "threadId"},
		},
		"ReviewStartResponse": {
			"type": "object",
			"properties": Schema{
				"reviewThreadId": Schema{"type": "string"},
				"turn":           Schema{"$ref": "#/$defs/Turn"},
			},
			"required": []string{"reviewThreadId", "turn"},
		},
	}
}

var (
	_ json.Marshaler   = ReviewDelivery("")
	_ json.Unmarshaler = (*ReviewDelivery)(nil)
	_ json.Marshaler   = ReviewTarget{}
	_ json.Unmarshaler = (*ReviewTarget)(nil)
	_ json.Marshaler   = ReviewStartParams{}
	_ json.Unmarshaler = (*ReviewStartParams)(nil)
	_ json.Unmarshaler = (*ReviewStartResponse)(nil)
)
