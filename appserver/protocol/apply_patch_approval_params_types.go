package protocol

import (
	"encoding/json"
	"errors"
)

// ApplyPatchApprovalParams is the exact legacy public patch-approval request
// data. It remains separate from Gollem's live file-change approval request.
type ApplyPatchApprovalParams struct {
	ConversationID ThreadId              `json:"conversationId"`
	CallID         string                `json:"callId"`
	FileChanges    map[string]FileChange `json:"fileChanges"`
	Reason         *string               `json:"reason,omitempty"`
	GrantRoot      *string               `json:"grantRoot,omitempty"`
}

func (p ApplyPatchApprovalParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		ConversationID ThreadId              `json:"conversationId"`
		CallID         string                `json:"callId"`
		FileChanges    map[string]FileChange `json:"fileChanges"`
		Reason         *string               `json:"reason"`
		GrantRoot      *string               `json:"grantRoot"`
	}
	fileChanges := p.FileChanges
	if fileChanges == nil {
		fileChanges = map[string]FileChange{}
	}
	return json.Marshal(wire{
		ConversationID: p.ConversationID,
		CallID:         p.CallID,
		FileChanges:    fileChanges,
		Reason:         p.Reason,
		GrantRoot:      p.GrantRoot,
	})
}

func (p *ApplyPatchApprovalParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode apply-patch approval params into nil receiver")
	}
	const objectName = "apply-patch approval params"
	payload, err := decodeRustSerdeObject(
		data,
		objectName,
		"conversationId",
		"callId",
		"fileChanges",
		"reason",
		"grantRoot",
	)
	if err != nil {
		return err
	}
	conversationID, err := decodeRequiredThreadItemValue[ThreadId](
		payload, objectName, "conversationId",
	)
	if err != nil {
		return err
	}
	callID, err := decodeRequiredThreadItemValue[string](payload, objectName, "callId")
	if err != nil {
		return err
	}
	fileChanges, err := decodeRequiredThreadItemValue[map[string]FileChange](
		payload, objectName, "fileChanges",
	)
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
	*p = ApplyPatchApprovalParams{
		ConversationID: conversationID,
		CallID:         callID,
		FileChanges:    fileChanges,
		Reason:         reason,
		GrantRoot:      grantRoot,
	}
	return nil
}

var (
	_ json.Marshaler   = ApplyPatchApprovalParams{}
	_ json.Unmarshaler = (*ApplyPatchApprovalParams)(nil)
)
