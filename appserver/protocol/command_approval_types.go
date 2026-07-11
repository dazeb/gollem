package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	CommandExecutionApprovalAccept                        = "accept"
	CommandExecutionApprovalAcceptForSession              = "acceptForSession"
	CommandExecutionApprovalAcceptWithExecpolicyAmendment = "acceptWithExecpolicyAmendment"
	CommandExecutionApprovalApplyNetworkPolicyAmendment   = "applyNetworkPolicyAmendment"
	CommandExecutionApprovalDecline                       = "decline"
	CommandExecutionApprovalCancel                        = "cancel"
)

type ExecPolicyAmendment []string

type NetworkPolicyRuleAction string

const (
	NetworkPolicyRuleAllow NetworkPolicyRuleAction = "allow"
	NetworkPolicyRuleDeny  NetworkPolicyRuleAction = "deny"
)

type NetworkPolicyAmendment struct {
	Host   string                  `json:"host"`
	Action NetworkPolicyRuleAction `json:"action"`
}

// CommandExecutionApprovalDecision retains the validated JSON for the public
// string-or-object union without flattening amendment variants into invalid Go
// states.
type CommandExecutionApprovalDecision struct {
	raw json.RawMessage
}

type CommandExecutionRequestApprovalResponse struct {
	Decision CommandExecutionApprovalDecision `json:"decision"`
}

func NewCommandExecutionApprovalDecisions(actions ...string) ([]CommandExecutionApprovalDecision, error) {
	decisions := make([]CommandExecutionApprovalDecision, 0, len(actions))
	for _, action := range actions {
		data, err := json.Marshal(action)
		if err != nil {
			return nil, err
		}
		canonical, err := validateCommandExecutionApprovalDecisionJSON(data)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, CommandExecutionApprovalDecision{raw: canonical})
	}
	return decisions, nil
}

func (d CommandExecutionApprovalDecision) Action() string {
	canonical, err := validateCommandExecutionApprovalDecisionJSON(d.raw)
	if err != nil {
		return ""
	}
	var literal string
	if json.Unmarshal(canonical, &literal) == nil {
		return literal
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(canonical, &object) != nil {
		return ""
	}
	for action := range object {
		return action
	}
	return ""
}

func (d CommandExecutionApprovalDecision) MarshalJSON() ([]byte, error) {
	if len(d.raw) == 0 {
		return nil, errors.New("command-execution approval decision has no value")
	}
	return validateCommandExecutionApprovalDecisionJSON(d.raw)
}

func (d *CommandExecutionApprovalDecision) UnmarshalJSON(data []byte) error {
	if d == nil {
		return errors.New("decode command-execution approval decision into nil receiver")
	}
	canonical, err := validateCommandExecutionApprovalDecisionJSON(data)
	if err != nil {
		return err
	}
	d.raw = canonical
	return nil
}

func (r CommandExecutionRequestApprovalResponse) MarshalJSON() ([]byte, error) {
	decision, err := json.Marshal(r.Decision)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Decision json.RawMessage `json:"decision"`
	}{Decision: decision})
}

func (r *CommandExecutionRequestApprovalResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode command-execution approval response into nil receiver")
	}
	object, err := decodeCommandApprovalObject(data, "decision")
	if err != nil {
		return fmt.Errorf("decode command-execution approval response: %w", err)
	}
	decisionRaw, ok := object["decision"]
	if !ok || bytes.Equal(bytes.TrimSpace(decisionRaw), []byte("null")) {
		return errors.New("command-execution approval response requires decision")
	}
	var decision CommandExecutionApprovalDecision
	if err := json.Unmarshal(decisionRaw, &decision); err != nil {
		return err
	}
	r.Decision = decision
	return nil
}

func validateCommandExecutionApprovalDecisionJSON(data []byte) (json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("command-execution approval decision is empty")
	}
	var literal string
	if err := json.Unmarshal(data, &literal); err == nil {
		switch literal {
		case CommandExecutionApprovalAccept,
			CommandExecutionApprovalAcceptForSession,
			CommandExecutionApprovalDecline,
			CommandExecutionApprovalCancel:
			canonical, _ := json.Marshal(literal)
			return canonical, nil
		default:
			return nil, fmt.Errorf("unknown command-execution approval decision %q", literal)
		}
	}
	object, err := decodeCommandApprovalObject(
		data,
		CommandExecutionApprovalAcceptWithExecpolicyAmendment,
		CommandExecutionApprovalApplyNetworkPolicyAmendment,
	)
	if err != nil {
		return nil, fmt.Errorf("decode command-execution approval decision: %w", err)
	}
	if len(object) != 1 {
		return nil, errors.New("command-execution approval decision requires exactly one variant")
	}
	if payload, ok := object[CommandExecutionApprovalAcceptWithExecpolicyAmendment]; ok {
		return canonicalExecPolicyApprovalDecision(payload)
	}
	return canonicalNetworkPolicyApprovalDecision(object[CommandExecutionApprovalApplyNetworkPolicyAmendment])
}

func canonicalExecPolicyApprovalDecision(payloadRaw json.RawMessage) (json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(payloadRaw), []byte("null")) {
		return nil, errors.New("exec-policy amendment decision requires a payload")
	}
	payload, err := decodeCommandApprovalObject(payloadRaw, "execpolicy_amendment")
	if err != nil {
		return nil, err
	}
	amendmentRaw, ok := payload["execpolicy_amendment"]
	if !ok || bytes.Equal(bytes.TrimSpace(amendmentRaw), []byte("null")) {
		return nil, errors.New("exec-policy amendment decision requires execpolicy_amendment")
	}
	var amendment ExecPolicyAmendment
	if err := json.Unmarshal(amendmentRaw, &amendment); err != nil || amendment == nil {
		return nil, errors.New("execpolicy_amendment must be a string array")
	}
	return json.Marshal(map[string]any{
		CommandExecutionApprovalAcceptWithExecpolicyAmendment: map[string]any{
			"execpolicy_amendment": amendment,
		},
	})
}

func canonicalNetworkPolicyApprovalDecision(payloadRaw json.RawMessage) (json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(payloadRaw), []byte("null")) {
		return nil, errors.New("network-policy amendment decision requires a payload")
	}
	payload, err := decodeCommandApprovalObject(payloadRaw, "network_policy_amendment")
	if err != nil {
		return nil, err
	}
	amendmentRaw, ok := payload["network_policy_amendment"]
	if !ok || bytes.Equal(bytes.TrimSpace(amendmentRaw), []byte("null")) {
		return nil, errors.New("network-policy amendment decision requires network_policy_amendment")
	}
	amendmentObject, err := decodeCommandApprovalObject(amendmentRaw, "host", "action")
	if err != nil {
		return nil, err
	}
	host, err := requiredCommandApprovalString(amendmentObject, "host")
	if err != nil {
		return nil, err
	}
	action, err := requiredCommandApprovalString(amendmentObject, "action")
	if err != nil {
		return nil, err
	}
	if action != string(NetworkPolicyRuleAllow) && action != string(NetworkPolicyRuleDeny) {
		return nil, fmt.Errorf("unknown network-policy action %q", action)
	}
	return json.Marshal(map[string]any{
		CommandExecutionApprovalApplyNetworkPolicyAmendment: map[string]any{
			"network_policy_amendment": NetworkPolicyAmendment{
				Host:   host,
				Action: NetworkPolicyRuleAction(action),
			},
		},
	})
}

func decodeCommandApprovalObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("value must be an object")
	}
	known := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		known[name] = struct{}{}
	}
	for name := range object {
		if _, ok := known[name]; !ok {
			return nil, fmt.Errorf("unknown command-approval field %q", name)
		}
	}
	return object, nil
}

func requiredCommandApprovalString(object map[string]json.RawMessage, name string) (string, error) {
	raw, ok := object[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("network-policy amendment requires %s", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("network-policy amendment %s must be a string", name)
	}
	return value, nil
}
