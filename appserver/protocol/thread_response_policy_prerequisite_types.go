package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

const (
	ApprovalsReviewerUser             ApprovalsReviewer = "user"
	ApprovalsReviewerAutoReview       ApprovalsReviewer = "auto_review"
	ApprovalsReviewerGuardianSubagent ApprovalsReviewer = "guardian_subagent"

	NetworkAccessRestricted NetworkAccess = "restricted"
	NetworkAccessEnabled    NetworkAccess = "enabled"
)

type ApprovalsReviewer string
type NetworkAccess string

// AskForApproval is the exact public approval-policy union used by thread
// responses. Gollem's runtime approval policy remains separately owned.
type AskForApproval struct{ raw json.RawMessage }

// SandboxPolicy is the exact public sandbox-policy union used by thread
// responses. It does not replace Gollem's runtime workspace policy.
type SandboxPolicy struct{ raw json.RawMessage }

type granularApprovalPolicy struct {
	SandboxApproval    bool `json:"sandbox_approval"`
	Rules              bool `json:"rules"`
	SkillApproval      bool `json:"skill_approval"`
	RequestPermissions bool `json:"request_permissions"`
	MCPElicitations    bool `json:"mcp_elicitations"`
}

type granularAskForApproval struct {
	Granular granularApprovalPolicy `json:"granular"`
}

func (r ApprovalsReviewer) MarshalJSON() ([]byte, error) {
	if !validApprovalsReviewer(r) {
		return nil, fmt.Errorf("unsupported approvals reviewer %q", r)
	}
	return json.Marshal(string(r))
}

func (r *ApprovalsReviewer) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode approvals reviewer into nil receiver")
	}
	var value string
	if err := decodePermissionValue(data, &value); err != nil {
		return fmt.Errorf("decode approvals reviewer: %w", err)
	}
	parsed := ApprovalsReviewer(value)
	if !validApprovalsReviewer(parsed) {
		return fmt.Errorf("unsupported approvals reviewer %q", value)
	}
	*r = parsed
	return nil
}

func validApprovalsReviewer(value ApprovalsReviewer) bool {
	return value == ApprovalsReviewerUser ||
		value == ApprovalsReviewerAutoReview ||
		value == ApprovalsReviewerGuardianSubagent
}

func (a NetworkAccess) MarshalJSON() ([]byte, error) {
	if !validNetworkAccess(a) {
		return nil, fmt.Errorf("unsupported network access %q", a)
	}
	return json.Marshal(string(a))
}

func (a *NetworkAccess) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode network access into nil receiver")
	}
	var value string
	if err := decodePermissionValue(data, &value); err != nil {
		return fmt.Errorf("decode network access: %w", err)
	}
	parsed := NetworkAccess(value)
	if !validNetworkAccess(parsed) {
		return fmt.Errorf("unsupported network access %q", value)
	}
	*a = parsed
	return nil
}

func validNetworkAccess(value NetworkAccess) bool {
	return value == NetworkAccessRestricted || value == NetworkAccessEnabled
}

func (a AskForApproval) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("ask-for-approval value is empty")
	}
	return validateAskForApproval(a.raw)
}

func (a *AskForApproval) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode ask-for-approval into nil receiver")
	}
	canonical, err := validateAskForApproval(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

func (a AskForApproval) Kind() string {
	if len(a.raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(a.raw, &value); err == nil {
		return value
	}
	return "granular"
}

func (p SandboxPolicy) MarshalJSON() ([]byte, error) {
	if len(p.raw) == 0 {
		return nil, errors.New("sandbox policy is empty")
	}
	return validateSandboxPolicy(p.raw)
}

func (p *SandboxPolicy) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode sandbox policy into nil receiver")
	}
	canonical, err := validateSandboxPolicy(data)
	if err != nil {
		return err
	}
	p.raw = canonical
	return nil
}

func (p SandboxPolicy) Type() string {
	return permissionUnionDiscriminant(p.raw, "type")
}

func validateAskForApproval(data []byte) ([]byte, error) {
	var simple string
	if err := decodePermissionValue(data, &simple); err == nil {
		switch simple {
		case "untrusted", "on-request", "never":
			return json.Marshal(simple)
		default:
			return nil, fmt.Errorf("unsupported ask-for-approval value %q", simple)
		}
	}

	payload, err := decodeExactThreadItemObject(data, "ask-for-approval value", "granular")
	if err != nil {
		return nil, err
	}
	granularRaw, ok := payload["granular"]
	if !ok || isJSONNull(granularRaw) {
		return nil, errors.New("ask-for-approval value requires granular")
	}
	granular, err := decodeExactThreadItemObject(
		granularRaw,
		"granular approval policy",
		"sandbox_approval",
		"rules",
		"skill_approval",
		"request_permissions",
		"mcp_elicitations",
	)
	if err != nil {
		return nil, err
	}
	sandboxApproval, err := decodeRequiredThreadItemValue[bool](granular, "granular approval policy", "sandbox_approval")
	if err != nil {
		return nil, err
	}
	rules, err := decodeRequiredThreadItemValue[bool](granular, "granular approval policy", "rules")
	if err != nil {
		return nil, err
	}
	skillApproval, err := decodeRequiredThreadItemValue[bool](granular, "granular approval policy", "skill_approval")
	if err != nil {
		return nil, err
	}
	requestPermissions, err := decodeRequiredThreadItemValue[bool](granular, "granular approval policy", "request_permissions")
	if err != nil {
		return nil, err
	}
	mcpElicitations, err := decodeRequiredThreadItemValue[bool](granular, "granular approval policy", "mcp_elicitations")
	if err != nil {
		return nil, err
	}
	return json.Marshal(granularAskForApproval{
		Granular: granularApprovalPolicy{
			SandboxApproval: sandboxApproval, Rules: rules, SkillApproval: skillApproval,
			RequestPermissions: requestPermissions, MCPElicitations: mcpElicitations,
		},
	})
}

func validateSandboxPolicy(data []byte) ([]byte, error) {
	payload, err := decodeExactThreadItemObject(
		data,
		"sandbox policy",
		"type",
		"writableRoots",
		"networkAccess",
		"excludeTmpdirEnvVar",
		"excludeSlashTmp",
	)
	if err != nil {
		return nil, err
	}
	typeName, err := decodeRequiredThreadItemValue[string](payload, "sandbox policy", "type")
	if err != nil {
		return nil, err
	}
	switch typeName {
	case "dangerFullAccess":
		if err := requirePermissionFields(payload, []string{"type"}, "type"); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
		}{Type: typeName})
	case "readOnly":
		if err := requirePermissionFields(payload, []string{"type", "networkAccess"}, "type", "networkAccess"); err != nil {
			return nil, err
		}
		networkAccess, err := decodeRequiredThreadItemValue[bool](payload, "read-only sandbox policy", "networkAccess")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type          string `json:"type"`
			NetworkAccess bool   `json:"networkAccess"`
		}{Type: typeName, NetworkAccess: networkAccess})
	case "externalSandbox":
		if err := requirePermissionFields(payload, []string{"type", "networkAccess"}, "type", "networkAccess"); err != nil {
			return nil, err
		}
		networkAccess, err := decodeRequiredThreadItemValue[NetworkAccess](payload, "external sandbox policy", "networkAccess")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type          string        `json:"type"`
			NetworkAccess NetworkAccess `json:"networkAccess"`
		}{Type: typeName, NetworkAccess: networkAccess})
	case "workspaceWrite":
		allowed := []string{"type", "writableRoots", "networkAccess", "excludeTmpdirEnvVar", "excludeSlashTmp"}
		if err := requirePermissionFields(payload, allowed, allowed...); err != nil {
			return nil, err
		}
		writableRoots, err := decodeRequiredThreadItemValue[[]AbsolutePathBuf](payload, "workspace-write sandbox policy", "writableRoots")
		if err != nil {
			return nil, err
		}
		networkAccess, err := decodeRequiredThreadItemValue[bool](payload, "workspace-write sandbox policy", "networkAccess")
		if err != nil {
			return nil, err
		}
		excludeTmpdirEnvVar, err := decodeRequiredThreadItemValue[bool](payload, "workspace-write sandbox policy", "excludeTmpdirEnvVar")
		if err != nil {
			return nil, err
		}
		excludeSlashTmp, err := decodeRequiredThreadItemValue[bool](payload, "workspace-write sandbox policy", "excludeSlashTmp")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type                string            `json:"type"`
			WritableRoots       []AbsolutePathBuf `json:"writableRoots"`
			NetworkAccess       bool              `json:"networkAccess"`
			ExcludeTmpdirEnvVar bool              `json:"excludeTmpdirEnvVar"`
			ExcludeSlashTmp     bool              `json:"excludeSlashTmp"`
		}{
			Type: typeName, WritableRoots: writableRoots, NetworkAccess: networkAccess,
			ExcludeTmpdirEnvVar: excludeTmpdirEnvVar, ExcludeSlashTmp: excludeSlashTmp,
		})
	default:
		return nil, fmt.Errorf("unsupported sandbox policy type %q", typeName)
	}
}

var (
	_ json.Marshaler   = ApprovalsReviewer("")
	_ json.Unmarshaler = (*ApprovalsReviewer)(nil)
	_ json.Marshaler   = NetworkAccess("")
	_ json.Unmarshaler = (*NetworkAccess)(nil)
	_ json.Marshaler   = AskForApproval{}
	_ json.Unmarshaler = (*AskForApproval)(nil)
	_ json.Marshaler   = SandboxPolicy{}
	_ json.Unmarshaler = (*SandboxPolicy)(nil)
)
