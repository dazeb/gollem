package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

type ReasoningSummary string

const (
	ReasoningSummaryAuto     ReasoningSummary = "auto"
	ReasoningSummaryConcise  ReasoningSummary = "concise"
	ReasoningSummaryDetailed ReasoningSummary = "detailed"
	ReasoningSummaryNone     ReasoningSummary = "none"
)

func (s ReasoningSummary) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "reasoning summary", ReasoningSummary.valid)
}

func (s *ReasoningSummary) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "reasoning summary", ReasoningSummary.valid)
}

func (s ReasoningSummary) valid() bool {
	switch s {
	case ReasoningSummaryAuto, ReasoningSummaryConcise, ReasoningSummaryDetailed, ReasoningSummaryNone:
		return true
	default:
		return false
	}
}

// TurnStartParams is the exact fixed public turn-start request. It remains
// standalone from Gollem's broader prompt-driven live handler contract.
type TurnStartParams struct {
	ThreadID            string             `json:"threadId"`
	ClientUserMessageID *string            `json:"clientUserMessageId,omitempty"`
	Input               []UserInput        `json:"input" jsonschema:"nonnullable=true"`
	CWD                 *string            `json:"cwd,omitempty"`
	ApprovalPolicy      *AskForApproval    `json:"approvalPolicy,omitempty"`
	ApprovalsReviewer   *ApprovalsReviewer `json:"approvalsReviewer,omitempty"`
	SandboxPolicy       *SandboxPolicy     `json:"sandboxPolicy,omitempty"`
	Model               *string            `json:"model,omitempty"`
	ServiceTier         *string            `json:"serviceTier,omitempty"`
	Effort              *ReasoningEffort   `json:"effort,omitempty"`
	Summary             *ReasoningSummary  `json:"summary,omitempty"`
	Personality         *Personality       `json:"personality,omitempty"`
	OutputSchema        *JsonValue         `json:"outputSchema,omitempty"`
}

func (p TurnStartParams) MarshalJSON() ([]byte, error) {
	if p.Input == nil {
		return nil, errors.New("turn-start params input cannot be null")
	}
	type wire TurnStartParams
	encoded, err := json.Marshal(wire(p))
	if err != nil {
		return nil, fmt.Errorf("encode turn-start params: %w", err)
	}
	return encoded, nil
}

func (p *TurnStartParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode turn-start params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"turn-start params",
		"threadId",
		"clientUserMessageId",
		"input",
		"cwd",
		"approvalPolicy",
		"approvalsReviewer",
		"sandboxPolicy",
		"model",
		"serviceTier",
		"effort",
		"summary",
		"personality",
		"outputSchema",
	)
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, "turn-start params", "threadId")
	if err != nil {
		return err
	}
	input, err := decodeRequiredThreadItemArray[UserInput](payload, "turn-start params", "input")
	if err != nil {
		return err
	}
	clientUserMessageID, err := decodeOptionalNullableTurnStartParam[string](payload, "clientUserMessageId")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableTurnStartParam[string](payload, "cwd")
	if err != nil {
		return err
	}
	approvalPolicy, err := decodeOptionalNullableTurnStartParam[AskForApproval](payload, "approvalPolicy")
	if err != nil {
		return err
	}
	approvalsReviewer, err := decodeOptionalNullableTurnStartParam[ApprovalsReviewer](payload, "approvalsReviewer")
	if err != nil {
		return err
	}
	sandboxPolicy, err := decodeOptionalNullableTurnStartParam[SandboxPolicy](payload, "sandboxPolicy")
	if err != nil {
		return err
	}
	model, err := decodeOptionalNullableTurnStartParam[string](payload, "model")
	if err != nil {
		return err
	}
	serviceTier, err := decodeOptionalNullableTurnStartParam[string](payload, "serviceTier")
	if err != nil {
		return err
	}
	effort, err := decodeOptionalNullableTurnStartParam[ReasoningEffort](payload, "effort")
	if err != nil {
		return err
	}
	summary, err := decodeOptionalNullableTurnStartParam[ReasoningSummary](payload, "summary")
	if err != nil {
		return err
	}
	personality, err := decodeOptionalNullableTurnStartParam[Personality](payload, "personality")
	if err != nil {
		return err
	}
	outputSchema := decodeOptionalNullableTurnStartJSONValue(payload, "outputSchema")
	*p = TurnStartParams{
		ThreadID: threadID, ClientUserMessageID: clientUserMessageID, Input: input, CWD: cwd,
		ApprovalPolicy: approvalPolicy, ApprovalsReviewer: approvalsReviewer, SandboxPolicy: sandboxPolicy,
		Model: model, ServiceTier: serviceTier, Effort: effort, Summary: summary,
		Personality: personality, OutputSchema: outputSchema,
	}
	return nil
}

type TurnStartResponse struct {
	Turn Turn `json:"turn"`
}

func (r TurnStartResponse) MarshalJSON() ([]byte, error) {
	type wire TurnStartResponse
	encoded, err := json.Marshal(wire(r))
	if err != nil {
		return nil, fmt.Errorf("encode turn-start response: %w", err)
	}
	return encoded, nil
}

func (r *TurnStartResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode turn-start response into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "turn-start response", "turn")
	if err != nil {
		return err
	}
	turn, err := decodeRequiredThreadItemValue[Turn](payload, "turn-start response", "turn")
	if err != nil {
		return err
	}
	*r = TurnStartResponse{Turn: turn}
	return nil
}

func decodeOptionalNullableTurnStartParam[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode turn-start params %s: %w", fieldName, err)
	}
	return &value, nil
}

func decodeOptionalNullableTurnStartJSONValue(
	payload map[string]json.RawMessage,
	fieldName string,
) *JsonValue {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil
	}
	// decodeExactThreadItemObject has already validated one complete JSON value.
	return &JsonValue{raw: append(json.RawMessage(nil), raw...)}
}

var (
	_ json.Marshaler   = ReasoningSummary("")
	_ json.Unmarshaler = (*ReasoningSummary)(nil)
	_ json.Marshaler   = TurnStartParams{}
	_ json.Unmarshaler = (*TurnStartParams)(nil)
	_ json.Marshaler   = TurnStartResponse{}
	_ json.Unmarshaler = (*TurnStartResponse)(nil)
)
