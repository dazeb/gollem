package protocol

import (
	"encoding/json"
	"errors"
)

// ItemGuardianApprovalReviewStartedNotification is the exact unstable public
// description of a Guardian review start. It does not produce or authorize a review.
type ItemGuardianApprovalReviewStartedNotification struct {
	ThreadID     string                       `json:"threadId"`
	TurnID       string                       `json:"turnId"`
	StartedAtMS  int64                        `json:"startedAtMs"`
	ReviewID     string                       `json:"reviewId"`
	TargetItemID *string                      `json:"targetItemId"`
	Review       GuardianApprovalReview       `json:"review"`
	Action       GuardianApprovalReviewAction `json:"action"`
}

func (n ItemGuardianApprovalReviewStartedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadID     string                       `json:"threadId"`
		TurnID       string                       `json:"turnId"`
		StartedAtMS  int64                        `json:"startedAtMs"`
		ReviewID     string                       `json:"reviewId"`
		TargetItemID *string                      `json:"targetItemId"`
		Review       GuardianApprovalReview       `json:"review"`
		Action       GuardianApprovalReviewAction `json:"action"`
	}{
		ThreadID: n.ThreadID, TurnID: n.TurnID, StartedAtMS: n.StartedAtMS,
		ReviewID: n.ReviewID, TargetItemID: n.TargetItemID,
		Review: n.Review, Action: n.Action,
	})
}

func (n *ItemGuardianApprovalReviewStartedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode Guardian approval-review-started notification into nil receiver")
	}
	const objectName = "Guardian approval-review-started notification"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"threadId", "turnId", "startedAtMs", "reviewId", "targetItemId", "review", "action",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	turnID, err := decodeRequiredThreadItemValue[string](payload, objectName, "turnId")
	if err != nil {
		return err
	}
	startedAtMS, err := decodeRequiredThreadItemValue[int64](payload, objectName, "startedAtMs")
	if err != nil {
		return err
	}
	reviewID, err := decodeRequiredThreadItemValue[string](payload, objectName, "reviewId")
	if err != nil {
		return err
	}
	targetItemID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "targetItemId")
	if err != nil {
		return err
	}
	review, err := decodeRequiredThreadItemValue[GuardianApprovalReview](payload, objectName, "review")
	if err != nil {
		return err
	}
	action, err := decodeRequiredThreadItemValue[GuardianApprovalReviewAction](payload, objectName, "action")
	if err != nil {
		return err
	}
	*n = ItemGuardianApprovalReviewStartedNotification{
		ThreadID: threadID, TurnID: turnID, StartedAtMS: startedAtMS,
		ReviewID: reviewID, TargetItemID: targetItemID, Review: review, Action: action,
	}
	return nil
}

func guardianApprovalReviewStartedNotificationSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"threadId":     Schema{"type": "string"},
		"turnId":       Schema{"type": "string"},
		"startedAtMs":  Schema{"type": "integer"},
		"reviewId":     Schema{"type": "string"},
		"targetItemId": Schema{"type": []any{"string", "null"}},
		"review":       Schema{"$ref": "#/$defs/GuardianApprovalReview"},
		"action":       Schema{"$ref": "#/$defs/GuardianApprovalReviewAction"},
	}, []string{"threadId", "turnId", "startedAtMs", "reviewId", "review", "action"})
}

var (
	_ json.Marshaler   = ItemGuardianApprovalReviewStartedNotification{}
	_ json.Unmarshaler = (*ItemGuardianApprovalReviewStartedNotification)(nil)
)
