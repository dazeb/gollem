package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ExecCommandApprovalParams is the exact legacy public approval request data.
// It remains separate from Gollem's live command-execution approval request.
type ExecCommandApprovalParams struct {
	ConversationID ThreadId        `json:"conversationId"`
	CallID         string          `json:"callId"`
	ApprovalID     *string         `json:"approvalId,omitempty"`
	Command        []string        `json:"command"`
	CWD            string          `json:"cwd"`
	Reason         *string         `json:"reason,omitempty"`
	ParsedCmd      []ParsedCommand `json:"parsedCmd"`
}

func (p ExecCommandApprovalParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		ConversationID ThreadId        `json:"conversationId"`
		CallID         string          `json:"callId"`
		ApprovalID     *string         `json:"approvalId"`
		Command        []string        `json:"command"`
		CWD            string          `json:"cwd"`
		Reason         *string         `json:"reason"`
		ParsedCmd      []ParsedCommand `json:"parsedCmd"`
	}
	command := p.Command
	if command == nil {
		command = []string{}
	}
	parsedCmd := p.ParsedCmd
	if parsedCmd == nil {
		parsedCmd = []ParsedCommand{}
	}
	return json.Marshal(wire{
		ConversationID: p.ConversationID,
		CallID:         p.CallID,
		ApprovalID:     p.ApprovalID,
		Command:        command,
		CWD:            p.CWD,
		Reason:         p.Reason,
		ParsedCmd:      parsedCmd,
	})
}

func (p *ExecCommandApprovalParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode exec-command approval params into nil receiver")
	}
	const objectName = "exec-command approval params"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"conversationId",
		"callId",
		"approvalId",
		"command",
		"cwd",
		"reason",
		"parsedCmd",
	)
	if err != nil {
		return err
	}
	conversationID, err := decodeRequiredThreadItemValue[ThreadId](payload, objectName, "conversationId")
	if err != nil {
		return err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "callId")
	if err != nil {
		return err
	}
	approvalID, err := decodeOptionalNullableExecCommandApprovalParam[string](payload, "approvalId")
	if err != nil {
		return err
	}
	command, err := decodeRequiredThreadItemValue[[]string](payload, objectName, "command")
	if err != nil {
		return err
	}
	cwd, err := decodeRequiredThreadItemValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	reason, err := decodeOptionalNullableExecCommandApprovalParam[string](payload, "reason")
	if err != nil {
		return err
	}
	parsedCmd, err := decodeRequiredThreadItemValue[[]ParsedCommand](payload, objectName, "parsedCmd")
	if err != nil {
		return err
	}
	*p = ExecCommandApprovalParams{
		ConversationID: conversationID,
		CallID:         callID,
		ApprovalID:     approvalID,
		Command:        command,
		CWD:            cwd,
		Reason:         reason,
		ParsedCmd:      parsedCmd,
	}
	return nil
}

func decodeOptionalNullableExecCommandApprovalParam[T any](
	payload map[string]json.RawMessage,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode exec-command approval %s: %w", fieldName, err)
	}
	return &value, nil
}

var (
	_ json.Marshaler   = ExecCommandApprovalParams{}
	_ json.Unmarshaler = (*ExecCommandApprovalParams)(nil)
)
