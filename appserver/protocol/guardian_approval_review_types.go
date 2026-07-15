package protocol

import (
	"encoding/json"
	"errors"
)

// GuardianApprovalReview is the exact unstable public result of one approval
// auto-review. It is descriptive and does not perform or authorize a review.
type GuardianApprovalReview struct {
	Status            GuardianApprovalReviewStatus `json:"status"`
	RiskLevel         *GuardianRiskLevel           `json:"riskLevel"`
	UserAuthorization *GuardianUserAuthorization   `json:"userAuthorization"`
	Rationale         *string                      `json:"rationale"`
}

func (r GuardianApprovalReview) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Status            GuardianApprovalReviewStatus `json:"status"`
		RiskLevel         *GuardianRiskLevel           `json:"riskLevel"`
		UserAuthorization *GuardianUserAuthorization   `json:"userAuthorization"`
		Rationale         *string                      `json:"rationale"`
	}{
		Status: r.Status, RiskLevel: r.RiskLevel,
		UserAuthorization: r.UserAuthorization, Rationale: r.Rationale,
	})
}

func (r *GuardianApprovalReview) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode Guardian approval review into nil receiver")
	}
	const objectName = "Guardian approval review"
	payload, err := decodeRustSerdeObject(
		data, objectName, "status", "riskLevel", "userAuthorization", "rationale",
	)
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[GuardianApprovalReviewStatus](
		payload, objectName, "status",
	)
	if err != nil {
		return err
	}
	riskLevel, err := decodeOptionalNullableConfigValue[GuardianRiskLevel](
		payload, objectName, "riskLevel",
	)
	if err != nil {
		return err
	}
	userAuthorization, err := decodeOptionalNullableConfigValue[GuardianUserAuthorization](
		payload, objectName, "userAuthorization",
	)
	if err != nil {
		return err
	}
	rationale, err := decodeOptionalNullableConfigValue[string](payload, objectName, "rationale")
	if err != nil {
		return err
	}
	*r = GuardianApprovalReview{
		Status: status, RiskLevel: riskLevel,
		UserAuthorization: userAuthorization, Rationale: rationale,
	}
	return nil
}

func guardianApprovalReviewSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"status": Schema{"$ref": "#/$defs/GuardianApprovalReviewStatus"},
		"riskLevel": Schema{"anyOf": []any{
			Schema{"$ref": "#/$defs/GuardianRiskLevel"}, Schema{"type": "null"},
		}},
		"userAuthorization": Schema{"anyOf": []any{
			Schema{"$ref": "#/$defs/GuardianUserAuthorization"}, Schema{"type": "null"},
		}},
		"rationale": Schema{"type": []any{"string", "null"}},
	}, []string{"status"})
}

var (
	_ json.Marshaler   = GuardianApprovalReview{}
	_ json.Unmarshaler = (*GuardianApprovalReview)(nil)
)
