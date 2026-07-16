package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ReviewDecisionApproved                    = "approved"
	ReviewDecisionApprovedExecPolicyAmendment = "approved_execpolicy_amendment"
	ReviewDecisionApprovedForSession          = "approved_for_session"
	ReviewDecisionNetworkPolicyAmendment      = "network_policy_amendment"
	ReviewDecisionDenied                      = "denied"
	ReviewDecisionTimedOut                    = "timed_out"
	ReviewDecisionAbort                       = "abort"
)

// ReviewDecision is the exact legacy public approval decision. It remains
// separate from Gollem's live command-execution approval decision.
type ReviewDecision struct {
	raw json.RawMessage
}

func (d ReviewDecision) MarshalJSON() ([]byte, error) {
	if len(d.raw) == 0 {
		return nil, errors.New("review decision has no value")
	}
	return validateReviewDecisionJSON(d.raw)
}

func (d *ReviewDecision) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("decode review decision into nil receiver")
	}
	canonical, err := validateReviewDecisionJSON(data)
	if err != nil {
		return err
	}
	d.raw = canonical
	return nil
}

func validateReviewDecisionJSON(data []byte) (json.RawMessage, error) {
	var literal string
	if err := json.Unmarshal(data, &literal); err == nil {
		switch literal {
		case ReviewDecisionApproved,
			ReviewDecisionApprovedForSession,
			ReviewDecisionDenied,
			ReviewDecisionTimedOut,
			ReviewDecisionAbort:
			return json.Marshal(literal)
		default:
			return nil, fmt.Errorf("unknown review decision %q", literal)
		}
	}

	variant, payload, err := decodeReviewDecisionVariant(data)
	if err != nil {
		return nil, err
	}
	if variant == ReviewDecisionApprovedExecPolicyAmendment {
		return canonicalReviewExecPolicyAmendment(payload)
	}
	return canonicalReviewNetworkPolicyAmendment(payload)
}

func decodeReviewDecisionVariant(data []byte) (string, json.RawMessage, error) {
	const objectName = "review decision"
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return "", nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return "", nil, fmt.Errorf("%s must be a string or object", objectName)
	}

	var variant string
	var payload json.RawMessage
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", nil, fmt.Errorf("decode %s variant: %w", objectName, err)
		}
		name := token.(string)
		switch name {
		case ReviewDecisionApprovedExecPolicyAmendment,
			ReviewDecisionNetworkPolicyAmendment:
		default:
			return "", nil, fmt.Errorf("unknown %s variant %q", objectName, name)
		}
		if variant != "" {
			return "", nil, errors.New("review decision requires exactly one variant")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return "", nil, fmt.Errorf("decode %s variant %q: %w", objectName, name, err)
		}
		variant = name
		payload = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return "", nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return "", nil, errors.New("review decision must contain one JSON value")
		}
		return "", nil, fmt.Errorf("decode review decision trailing value: %w", err)
	}
	if variant == "" {
		return "", nil, errors.New("review decision requires exactly one variant")
	}
	return variant, payload, nil
}

func canonicalReviewExecPolicyAmendment(payloadRaw json.RawMessage) (json.RawMessage, error) {
	const objectName = "approved exec-policy amendment review decision"
	payload, err := decodeRustSerdeObject(
		payloadRaw,
		objectName,
		"proposed_execpolicy_amendment",
	)
	if err != nil {
		return nil, err
	}
	amendment, err := decodeRequiredThreadItemArray[string](
		payload,
		objectName,
		"proposed_execpolicy_amendment",
	)
	if err != nil {
		return nil, err
	}
	type payloadWire struct {
		ProposedExecPolicyAmendment ExecPolicyAmendment `json:"proposed_execpolicy_amendment"`
	}
	return json.Marshal(struct {
		ApprovedExecPolicyAmendment payloadWire `json:"approved_execpolicy_amendment"`
	}{
		ApprovedExecPolicyAmendment: payloadWire{
			ProposedExecPolicyAmendment: ExecPolicyAmendment(amendment),
		},
	})
}

func canonicalReviewNetworkPolicyAmendment(payloadRaw json.RawMessage) (json.RawMessage, error) {
	const objectName = "network-policy amendment review decision"
	payload, err := decodeRustSerdeObject(
		payloadRaw,
		objectName,
		"network_policy_amendment",
	)
	if err != nil {
		return nil, err
	}
	amendmentRaw, ok := payload["network_policy_amendment"]
	if !ok || isJSONNull(amendmentRaw) {
		return nil, errors.New("network-policy amendment review decision requires network_policy_amendment")
	}
	amendmentObject, err := decodeRustSerdeObject(
		amendmentRaw,
		"network policy amendment",
		"host",
		"action",
	)
	if err != nil {
		return nil, err
	}
	host, err := decodeRequiredThreadItemValue[string](
		amendmentObject,
		"network policy amendment",
		"host",
	)
	if err != nil {
		return nil, err
	}
	action, err := decodeRequiredThreadItemValue[NetworkPolicyRuleAction](
		amendmentObject,
		"network policy amendment",
		"action",
	)
	if err != nil {
		return nil, err
	}
	if action != NetworkPolicyRuleAllow && action != NetworkPolicyRuleDeny {
		return nil, fmt.Errorf("unknown network-policy action %q", action)
	}
	type payloadWire struct {
		Amendment NetworkPolicyAmendment `json:"network_policy_amendment"`
	}
	return json.Marshal(struct {
		NetworkPolicyAmendment payloadWire `json:"network_policy_amendment"`
	}{
		NetworkPolicyAmendment: payloadWire{
			Amendment: NetworkPolicyAmendment{Host: host, Action: action},
		},
	})
}

func reviewDecisionSchema() Schema {
	return Schema{
		"description": "User's decision in response to an ExecApprovalRequest.",
		"oneOf": []any{
			reviewDecisionStringSchema(
				ReviewDecisionApproved,
				"User has approved this command and the agent should execute it.",
			),
			reviewDecisionObjectSchema(
				ReviewDecisionApprovedExecPolicyAmendment,
				"ApprovedExecpolicyAmendmentReviewDecision",
				"User has approved this command and wants to apply the proposed execpolicy amendment so future matching commands are permitted.",
				"proposed_execpolicy_amendment",
				"ExecPolicyAmendment",
			),
			reviewDecisionStringSchema(
				ReviewDecisionApprovedForSession,
				"User has approved this request and wants future prompts in the same session-scoped approval cache to be automatically approved for the remainder of the session.",
			),
			reviewDecisionObjectSchema(
				ReviewDecisionNetworkPolicyAmendment,
				"NetworkPolicyAmendmentReviewDecision",
				"User chose to persist a network policy rule (allow/deny) for future requests to the same host.",
				"network_policy_amendment",
				"NetworkPolicyAmendment",
			),
			reviewDecisionStringSchema(
				ReviewDecisionDenied,
				"User has denied this command and the agent should not execute it, but it should continue the session and try something else.",
			),
			reviewDecisionStringSchema(
				ReviewDecisionTimedOut,
				"Automatic approval review timed out before reaching a decision.",
			),
			reviewDecisionStringSchema(
				ReviewDecisionAbort,
				"User has denied this command and the agent should not do anything until the user's next command.",
			),
		},
	}
}

func reviewDecisionStringSchema(value, description string) Schema {
	return Schema{
		"type":        "string",
		"enum":        []any{value},
		"description": description,
	}
}

func reviewDecisionObjectSchema(
	variant,
	title,
	description,
	payloadField,
	payloadType string,
) Schema {
	return Schema{
		"type":                 "object",
		"title":                title,
		"description":          description,
		"additionalProperties": false,
		"properties": Schema{
			variant: Schema{
				"type":                 "object",
				"additionalProperties": false,
				"properties": Schema{
					payloadField: Schema{"$ref": "#/$defs/" + payloadType},
				},
				"required": []string{payloadField},
			},
		},
		"required": []string{variant},
	}
}

var (
	_ json.Marshaler   = ReviewDecision{}
	_ json.Unmarshaler = (*ReviewDecision)(nil)
)
