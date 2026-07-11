package protocol

import (
	"encoding/json"
	"testing"
)

func TestCommandApprovalResponseSchemaAndBindingAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"CommandExecutionApprovalDecision",
		"CommandExecutionRequestApprovalResponse",
		"ExecPolicyAmendment",
		"NetworkPolicyAmendment",
		"NetworkPolicyRuleAction",
	} {
		if _, ok := defs[name]; !ok {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}
	decision := defs["CommandExecutionApprovalDecision"].(Schema)
	variants, ok := decision["oneOf"].([]any)
	if !ok || len(variants) != 6 {
		t.Fatalf("CommandExecutionApprovalDecision oneOf = %#v", decision["oneOf"])
	}
	assertStringEnum(t, defs["NetworkPolicyRuleAction"], "allow", "deny")
	assertSchemaRequired(t, defs["NetworkPolicyAmendment"].(Schema), "host")
	assertSchemaRequired(t, defs["NetworkPolicyAmendment"].(Schema), "action")
	assertSchemaRequired(t, defs["CommandExecutionRequestApprovalResponse"].(Schema), "decision")
	bindings := WireTypeBindings()
	assertBinding(t, bindings, "item/commandExecution/requestApproval", SurfaceServerRequest, "CommandExecutionApprovalRequestParams")
	assertBinding(t, bindings, "item/commandExecution/requestApproval", SurfaceServerRequest, "CommandExecutionRequestApprovalResponse")
}

func TestCommandExecutionApprovalDecisionValidatesEveryPublicVariant(t *testing.T) {
	valid := []string{
		`"accept"`,
		`"acceptForSession"`,
		`{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["git","status"]}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":{"host":"example.com","action":"allow"}}}`,
		`"decline"`,
		`"cancel"`,
	}
	wantActions := []string{
		CommandExecutionApprovalAccept,
		CommandExecutionApprovalAcceptForSession,
		CommandExecutionApprovalAcceptWithExecpolicyAmendment,
		CommandExecutionApprovalApplyNetworkPolicyAmendment,
		CommandExecutionApprovalDecline,
		CommandExecutionApprovalCancel,
	}
	for index, input := range valid {
		var decision CommandExecutionApprovalDecision
		if err := json.Unmarshal([]byte(input), &decision); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(decision)
		if err != nil {
			t.Errorf("Marshal(%s): %v", input, err)
			continue
		}
		var roundTrip CommandExecutionApprovalDecision
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Errorf("round-trip %s: %v", encoded, err)
		}
		if action := decision.Action(); action != wantActions[index] {
			t.Errorf("Action(%s) = %q, want %q", input, action, wantActions[index])
		}
	}
	invalid := []string{
		`[`,
		`null`,
		`"approve"`,
		`{}`,
		`{"accept":"extra"}`,
		`{"acceptWithExecpolicyAmendment":null}`,
		`{"acceptWithExecpolicyAmendment":{}}`,
		`{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":null}}`,
		`{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":[1]}}`,
		`{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":[],"extra":true}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":null}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":{}}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":{"host":1,"action":"allow"}}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":{"host":"example.com","action":1}}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":{"host":"example.com","action":"prompt"}}}`,
		`{"applyNetworkPolicyAmendment":{"network_policy_amendment":{"host":"example.com","action":"allow","extra":true}}}`,
		`{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":[]},"cancel":null}`,
	}
	for _, input := range invalid {
		var decision CommandExecutionApprovalDecision
		if err := json.Unmarshal([]byte(input), &decision); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	if _, err := json.Marshal(CommandExecutionApprovalDecision{}); err == nil {
		t.Error("zero decision marshal succeeded")
	}
	var decision *CommandExecutionApprovalDecision
	if err := decision.UnmarshalJSON([]byte(`"accept"`)); err == nil {
		t.Error("nil decision receiver succeeded")
	}
	decisions, err := NewCommandExecutionApprovalDecisions(
		CommandExecutionApprovalAccept,
		CommandExecutionApprovalDecline,
		CommandExecutionApprovalCancel,
	)
	if err != nil {
		t.Fatalf("NewCommandExecutionApprovalDecisions: %v", err)
	}
	if len(decisions) != 3 || decisions[0].Action() != CommandExecutionApprovalAccept ||
		decisions[1].Action() != CommandExecutionApprovalDecline || decisions[2].Action() != CommandExecutionApprovalCancel {
		t.Fatalf("literal decisions = %#v", decisions)
	}
	if _, err := NewCommandExecutionApprovalDecisions("approve"); err == nil {
		t.Error("unknown literal decision constructor succeeded")
	}
	if action := (CommandExecutionApprovalDecision{}).Action(); action != "" {
		t.Fatalf("zero decision action = %q", action)
	}
}

func TestCommandExecutionApprovalResponseIsStrict(t *testing.T) {
	for _, input := range []string{
		`{"decision":"accept"}`,
		`{"decision":"decline"}`,
		`{"decision":"cancel"}`,
		`{"decision":{"acceptWithExecpolicyAmendment":{"execpolicy_amendment":["git","status"]}}}`,
	} {
		var response CommandExecutionRequestApprovalResponse
		if err := json.Unmarshal([]byte(input), &response); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
		}
	}
	for _, input := range []string{
		`{}`,
		`{"decision":null}`,
		`{"decision":"approve"}`,
		`{"decision":"accept","extra":true}`,
	} {
		var response CommandExecutionRequestApprovalResponse
		if err := json.Unmarshal([]byte(input), &response); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var response *CommandExecutionRequestApprovalResponse
	if err := response.UnmarshalJSON([]byte(`{"decision":"accept"}`)); err == nil {
		t.Error("nil response receiver succeeded")
	}
	decisions, err := NewCommandExecutionApprovalDecisions(CommandExecutionApprovalAccept)
	if err != nil {
		t.Fatalf("NewCommandExecutionApprovalDecisions: %v", err)
	}
	encoded, err := json.Marshal(CommandExecutionRequestApprovalResponse{Decision: decisions[0]})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{"decision":"accept"}` {
		t.Fatalf("response = %s", encoded)
	}
	if _, err := json.Marshal(CommandExecutionRequestApprovalResponse{}); err == nil {
		t.Error("zero response marshal succeeded")
	}
}
