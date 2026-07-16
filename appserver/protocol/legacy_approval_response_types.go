package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ApplyPatchApprovalResponse is the legacy apply-patch callback response. It
// remains separate from the live file-change approval response.
type ApplyPatchApprovalResponse struct {
	Decision ReviewDecision `json:"decision"`
}

// ExecCommandApprovalResponse is the legacy exec-command callback response. It
// remains separate from the live command-execution approval response.
type ExecCommandApprovalResponse struct {
	Decision ReviewDecision `json:"decision"`
}

func (r ApplyPatchApprovalResponse) MarshalJSON() ([]byte, error) {
	return marshalLegacyApprovalResponse(r.Decision)
}

func (r *ApplyPatchApprovalResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode apply-patch approval response into nil receiver")
	}
	decision, err := decodeLegacyApprovalResponse(data, "apply-patch approval response")
	if err != nil {
		return err
	}
	r.Decision = decision
	return nil
}

func (r ExecCommandApprovalResponse) MarshalJSON() ([]byte, error) {
	return marshalLegacyApprovalResponse(r.Decision)
}

func (r *ExecCommandApprovalResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode exec-command approval response into nil receiver")
	}
	decision, err := decodeLegacyApprovalResponse(data, "exec-command approval response")
	if err != nil {
		return err
	}
	r.Decision = decision
	return nil
}

func marshalLegacyApprovalResponse(decision ReviewDecision) ([]byte, error) {
	encoded, err := json.Marshal(decision)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Decision json.RawMessage `json:"decision"`
	}{Decision: encoded})
}

func decodeLegacyApprovalResponse(data []byte, objectName string) (ReviewDecision, error) {
	payload, err := decodeRustSerdeObject(data, objectName, "decision")
	if err != nil {
		return ReviewDecision{}, err
	}
	decision, err := decodeRequiredThreadItemValue[ReviewDecision](payload, objectName, "decision")
	if err != nil {
		return ReviewDecision{}, fmt.Errorf("decode %s: %w", objectName, err)
	}
	return decision, nil
}
