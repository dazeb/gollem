package protocol

import (
	"encoding/json"
	"errors"
)

// ItemGuardianApprovalReviewCompletedNotification is the exact unstable public
// description of a Guardian review completion. It does not produce or authorize a review.
type ItemGuardianApprovalReviewCompletedNotification struct {
	ThreadID       string                       `json:"threadId"`
	TurnID         string                       `json:"turnId"`
	StartedAtMS    int64                        `json:"startedAtMs"`
	CompletedAtMS  int64                        `json:"completedAtMs"`
	ReviewID       string                       `json:"reviewId"`
	TargetItemID   *string                      `json:"targetItemId"`
	DecisionSource AutoReviewDecisionSource     `json:"decisionSource"`
	Review         GuardianApprovalReview       `json:"review"`
	Action         GuardianApprovalReviewAction `json:"action"`
}

func (n ItemGuardianApprovalReviewCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ThreadID       string                       `json:"threadId"`
		TurnID         string                       `json:"turnId"`
		StartedAtMS    int64                        `json:"startedAtMs"`
		CompletedAtMS  int64                        `json:"completedAtMs"`
		ReviewID       string                       `json:"reviewId"`
		TargetItemID   *string                      `json:"targetItemId"`
		DecisionSource AutoReviewDecisionSource     `json:"decisionSource"`
		Review         GuardianApprovalReview       `json:"review"`
		Action         GuardianApprovalReviewAction `json:"action"`
	}{
		ThreadID: n.ThreadID, TurnID: n.TurnID,
		StartedAtMS: n.StartedAtMS, CompletedAtMS: n.CompletedAtMS,
		ReviewID: n.ReviewID, TargetItemID: n.TargetItemID,
		DecisionSource: n.DecisionSource, Review: n.Review, Action: n.Action,
	})
}

func (n *ItemGuardianApprovalReviewCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode Guardian approval-review-completed notification into nil receiver")
	}
	const objectName = "Guardian approval-review-completed notification"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"threadId", "turnId", "startedAtMs", "completedAtMs", "reviewId",
		"targetItemId", "decisionSource", "review", "action",
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
	completedAtMS, err := decodeRequiredThreadItemValue[int64](payload, objectName, "completedAtMs")
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
	decisionSource, err := decodeRequiredThreadItemValue[AutoReviewDecisionSource](
		payload, objectName, "decisionSource",
	)
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
	*n = ItemGuardianApprovalReviewCompletedNotification{
		ThreadID: threadID, TurnID: turnID,
		StartedAtMS: startedAtMS, CompletedAtMS: completedAtMS,
		ReviewID: reviewID, TargetItemID: targetItemID,
		DecisionSource: decisionSource, Review: review, Action: action,
	}
	return nil
}

func guardianApprovalReviewCompletedNotificationSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"threadId":       Schema{"type": "string"},
		"turnId":         Schema{"type": "string"},
		"startedAtMs":    Schema{"type": "integer"},
		"completedAtMs":  Schema{"type": "integer"},
		"reviewId":       Schema{"type": "string"},
		"targetItemId":   Schema{"type": []any{"string", "null"}},
		"decisionSource": Schema{"$ref": "#/$defs/AutoReviewDecisionSource"},
		"review":         Schema{"$ref": "#/$defs/GuardianApprovalReview"},
		"action":         Schema{"$ref": "#/$defs/GuardianApprovalReviewAction"},
	}, []string{
		"threadId", "turnId", "startedAtMs", "completedAtMs", "reviewId",
		"decisionSource", "review", "action",
	})
}

var (
	_ json.Marshaler   = ItemGuardianApprovalReviewCompletedNotification{}
	_ json.Unmarshaler = (*ItemGuardianApprovalReviewCompletedNotification)(nil)
)
