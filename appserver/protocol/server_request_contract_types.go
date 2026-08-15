package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// CommandExecutionRequestApprovalParams is the exact standalone public command
// approval request. It is separate from Gollem's live compatibility request.
type CommandExecutionRequestApprovalParams struct {
	ThreadID                        string                    `json:"threadId"`
	TurnID                          string                    `json:"turnId"`
	ItemID                          string                    `json:"itemId"`
	StartedAtMS                     int64                     `json:"startedAtMs"`
	ApprovalID                      *string                   `json:"approvalId,omitempty"`
	EnvironmentID                   *string                   `json:"environmentId"`
	Reason                          *string                   `json:"reason,omitempty"`
	NetworkApprovalContext          *NetworkApprovalContext   `json:"networkApprovalContext,omitempty"`
	Command                         *string                   `json:"command,omitempty"`
	CWD                             *LegacyAppPathString      `json:"cwd,omitempty"`
	CommandActions                  *[]CommandAction          `json:"commandActions,omitempty"`
	ProposedExecpolicyAmendment     *ExecPolicyAmendment      `json:"proposedExecpolicyAmendment,omitempty"`
	ProposedNetworkPolicyAmendments *[]NetworkPolicyAmendment `json:"proposedNetworkPolicyAmendments,omitempty"`
}

// FileChangeRequestApprovalParams is the exact standalone public file-change
// approval request. It does not alter Gollem's live approval workflow.
type FileChangeRequestApprovalParams struct {
	ThreadID    string  `json:"threadId"`
	TurnID      string  `json:"turnId"`
	ItemID      string  `json:"itemId"`
	StartedAtMS int64   `json:"startedAtMs"`
	Reason      *string `json:"reason"`
	GrantRoot   *string `json:"grantRoot"`
}

func (p *CommandExecutionRequestApprovalParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode command-execution approval params into nil receiver")
	}
	const objectName = "command-execution approval params"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"threadId", "turnId", "itemId", "startedAtMs", "approvalId", "environmentId",
		"reason", "networkApprovalContext", "command", "cwd", "commandActions",
		"proposedExecpolicyAmendment", "proposedNetworkPolicyAmendments",
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
	itemID, err := decodeRequiredThreadItemValue[string](payload, objectName, "itemId")
	if err != nil {
		return err
	}
	startedAtMS, err := decodeRequiredThreadItemValue[int64](payload, objectName, "startedAtMs")
	if err != nil {
		return err
	}
	approvalID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "approvalId")
	if err != nil {
		return err
	}
	environmentID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "environmentId")
	if err != nil {
		return err
	}
	reason, err := decodeOptionalNullableConfigValue[string](payload, objectName, "reason")
	if err != nil {
		return err
	}
	networkApprovalContext, err := decodeOptionalNullableConfigValue[NetworkApprovalContext](payload, objectName, "networkApprovalContext")
	if err != nil {
		return err
	}
	command, err := decodeOptionalNullableConfigValue[string](payload, objectName, "command")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableConfigValue[LegacyAppPathString](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	commandActions, err := decodeOptionalNullableConfigValue[[]CommandAction](payload, objectName, "commandActions")
	if err != nil {
		return err
	}
	proposedExecpolicyAmendment, err := decodeOptionalNullableConfigValue[ExecPolicyAmendment](payload, objectName, "proposedExecpolicyAmendment")
	if err != nil {
		return err
	}
	proposedNetworkPolicyAmendments, err := decodeOptionalNullableNetworkPolicyAmendments(payload, objectName, "proposedNetworkPolicyAmendments")
	if err != nil {
		return err
	}
	*p = CommandExecutionRequestApprovalParams{
		ThreadID:                        threadID,
		TurnID:                          turnID,
		ItemID:                          itemID,
		StartedAtMS:                     startedAtMS,
		ApprovalID:                      approvalID,
		EnvironmentID:                   environmentID,
		Reason:                          reason,
		NetworkApprovalContext:          networkApprovalContext,
		Command:                         command,
		CWD:                             cwd,
		CommandActions:                  commandActions,
		ProposedExecpolicyAmendment:     proposedExecpolicyAmendment,
		ProposedNetworkPolicyAmendments: proposedNetworkPolicyAmendments,
	}
	return nil
}

func decodeOptionalNullableNetworkPolicyAmendments(
	payload map[string]json.RawMessage,
	objectName, fieldName string,
) (*[]NetworkPolicyAmendment, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	amendments := make([]NetworkPolicyAmendment, 0, len(entries))
	for index, entry := range entries {
		entryName := fmt.Sprintf("%s %s[%d]", objectName, fieldName, index)
		amendment, err := decodeRustSerdeObject(entry, entryName, "host", "action")
		if err != nil {
			return nil, err
		}
		host, err := decodeRequiredThreadItemValue[string](amendment, entryName, "host")
		if err != nil {
			return nil, err
		}
		action, err := decodeRequiredThreadItemValue[NetworkPolicyRuleAction](amendment, entryName, "action")
		if err != nil {
			return nil, err
		}
		if action != NetworkPolicyRuleAllow && action != NetworkPolicyRuleDeny {
			return nil, fmt.Errorf("%s has unknown action %q", entryName, action)
		}
		amendments = append(amendments, NetworkPolicyAmendment{Host: host, Action: action})
	}
	return &amendments, nil
}

func (p *FileChangeRequestApprovalParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode file-change approval params into nil receiver")
	}
	const objectName = "file-change approval params"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "turnId", "itemId", "startedAtMs", "reason", "grantRoot")
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
	itemID, err := decodeRequiredThreadItemValue[string](payload, objectName, "itemId")
	if err != nil {
		return err
	}
	startedAtMS, err := decodeRequiredThreadItemValue[int64](payload, objectName, "startedAtMs")
	if err != nil {
		return err
	}
	reason, err := decodeOptionalNullableConfigValue[string](payload, objectName, "reason")
	if err != nil {
		return err
	}
	grantRoot, err := decodeOptionalNullableConfigValue[string](payload, objectName, "grantRoot")
	if err != nil {
		return err
	}
	*p = FileChangeRequestApprovalParams{
		ThreadID: threadID, TurnID: turnID, ItemID: itemID, StartedAtMS: startedAtMS,
		Reason: reason, GrantRoot: grantRoot,
	}
	return nil
}

func serverRequestApprovalParamSchemas() map[string]Schema {
	return map[string]Schema{
		"CommandExecutionRequestApprovalParams": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"approvalId":                      Schema{"description": "Unique identifier for this specific approval callback.\n\nFor regular shell/unified_exec approvals, this is null.\n\nFor zsh-exec-bridge subcommand approvals, multiple callbacks can belong to one parent `itemId`, so `approvalId` is a distinct opaque callback id (a UUID) used to disambiguate routing.", "type": []any{"string", "null"}},
				"command":                         Schema{"description": "The command to be executed.", "type": []any{"string", "null"}},
				"commandActions":                  Schema{"description": "Best-effort parsed command actions for friendly display.", "items": Schema{"$ref": "#/$defs/CommandAction"}, "type": []any{"array", "null"}},
				"cwd":                             Schema{"anyOf": []any{Schema{"$ref": "#/$defs/LegacyAppPathString"}, Schema{"type": "null"}}, "description": "The command's working directory."},
				"environmentId":                   Schema{"default": nil, "description": "Environment in which the command will run.", "type": []any{"string", "null"}},
				"itemId":                          Schema{"type": "string"},
				"networkApprovalContext":          Schema{"anyOf": []any{Schema{"$ref": "#/$defs/NetworkApprovalContext"}, Schema{"type": "null"}}, "description": "Optional context for a managed-network approval prompt."},
				"proposedExecpolicyAmendment":     Schema{"description": "Optional proposed execpolicy amendment to allow similar commands without prompting.", "items": Schema{"type": "string"}, "type": []any{"array", "null"}},
				"proposedNetworkPolicyAmendments": Schema{"description": "Optional proposed network policy amendments (allow/deny host) for future requests.", "items": Schema{"$ref": "#/$defs/NetworkPolicyAmendment"}, "type": []any{"array", "null"}},
				"reason":                          Schema{"description": "Optional explanatory reason (e.g. request for network access).", "type": []any{"string", "null"}},
				"startedAtMs":                     Schema{"description": "Unix timestamp (in milliseconds) when this approval request started.", "format": "int64", "type": "integer"},
				"threadId":                        Schema{"type": "string"},
				"turnId":                          Schema{"type": "string"},
			},
			"required": []string{"itemId", "startedAtMs", "threadId", "turnId"},
			"title":    "CommandExecutionRequestApprovalParams",
			"type":     "object",
		},
		"FileChangeRequestApprovalParams": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"grantRoot":   Schema{"description": "[UNSTABLE] When set, the agent is asking the user to allow writes under this root for the remainder of the session (unclear if this is honored today).", "type": []any{"string", "null"}},
				"itemId":      Schema{"type": "string"},
				"reason":      Schema{"description": "Optional explanatory reason (e.g. request for extra write access).", "type": []any{"string", "null"}},
				"startedAtMs": Schema{"description": "Unix timestamp (in milliseconds) when this approval request started.", "format": "int64", "type": "integer"},
				"threadId":    Schema{"type": "string"},
				"turnId":      Schema{"type": "string"},
			},
			"required": []string{"itemId", "startedAtMs", "threadId", "turnId"},
			"title":    "FileChangeRequestApprovalParams",
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*CommandExecutionRequestApprovalParams)(nil)
	_ json.Unmarshaler = (*FileChangeRequestApprovalParams)(nil)
)
