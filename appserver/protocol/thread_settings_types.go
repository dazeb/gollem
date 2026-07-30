package protocol

import (
	"encoding/json"
	"errors"
)

// ThreadExtra is the exact open public extension record carried on a thread.
// It remains standalone because Gollem's durable Thread record has a distinct
// extension model.
type ThreadExtra struct{}

// ThreadHistoryMode is the persisted history representation selected for a
// thread. It does not alter Gollem's durable history implementation.
type ThreadHistoryMode string

const (
	ThreadHistoryModeLegacy    ThreadHistoryMode = "legacy"
	ThreadHistoryModePaginated ThreadHistoryMode = "paginated"
)

// ThreadSettings is the exact public snapshot used by the Codex app-server.
// The standalone record does not make Gollem's loose settings-map endpoint a
// compatible implementation of thread/settings/update.
type ThreadSettings struct {
	CWD                     AbsolutePathBuf          `json:"cwd"`
	ApprovalPolicy          AskForApproval           `json:"approvalPolicy"`
	ApprovalsReviewer       ApprovalsReviewer        `json:"approvalsReviewer"`
	SandboxPolicy           SandboxPolicy            `json:"sandboxPolicy"`
	ActivePermissionProfile *ActivePermissionProfile `json:"activePermissionProfile"`
	Model                   string                   `json:"model"`
	ModelProvider           string                   `json:"modelProvider"`
	ServiceTier             *string                  `json:"serviceTier"`
	Effort                  *ReasoningEffort         `json:"effort"`
	Summary                 *ReasoningSummary        `json:"summary"`
	CollaborationMode       CollaborationMode        `json:"collaborationMode"`
	Personality             *Personality             `json:"personality"`
}

// ThreadSettingsUpdatedNotification is the exact public settings-snapshot
// notification. Its standalone export does not change Gollem's existing
// thread/settings/updated runtime payload.
type ThreadSettingsUpdatedNotification struct {
	ThreadID       string         `json:"threadId"`
	ThreadSettings ThreadSettings `json:"threadSettings"`
}

func (m ThreadHistoryMode) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "thread history mode", ThreadHistoryMode.valid)
}

func (m *ThreadHistoryMode) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "thread history mode", ThreadHistoryMode.valid)
}

func (m ThreadHistoryMode) valid() bool {
	return m == ThreadHistoryModeLegacy || m == ThreadHistoryModePaginated
}

func (e *ThreadExtra) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode thread extra into nil receiver")
	}
	if _, err := decodeRustSerdeObject(data, "thread extra"); err != nil {
		return err
	}
	*e = ThreadExtra{}
	return nil
}

func (s *ThreadSettings) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode thread settings into nil receiver")
	}
	const objectName = "thread settings"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"cwd", "approvalPolicy", "approvalsReviewer", "sandboxPolicy",
		"activePermissionProfile", "model", "modelProvider", "serviceTier",
		"effort", "summary", "collaborationMode", "personality",
	)
	if err != nil {
		return err
	}
	cwd, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	approvalPolicy, err := decodeRequiredThreadItemValue[AskForApproval](payload, objectName, "approvalPolicy")
	if err != nil {
		return err
	}
	approvalsReviewer, err := decodeRequiredThreadItemValue[ApprovalsReviewer](payload, objectName, "approvalsReviewer")
	if err != nil {
		return err
	}
	sandboxPolicy, err := decodeRequiredThreadItemValue[SandboxPolicy](payload, objectName, "sandboxPolicy")
	if err != nil {
		return err
	}
	activePermissionProfile, err := decodeOptionalNullableConfigValue[ActivePermissionProfile](payload, objectName, "activePermissionProfile")
	if err != nil {
		return err
	}
	model, err := decodeRequiredThreadItemValue[string](payload, objectName, "model")
	if err != nil {
		return err
	}
	modelProvider, err := decodeRequiredThreadItemValue[string](payload, objectName, "modelProvider")
	if err != nil {
		return err
	}
	serviceTier, err := decodeOptionalNullableConfigValue[string](payload, objectName, "serviceTier")
	if err != nil {
		return err
	}
	effort, err := decodeOptionalNullableConfigValue[ReasoningEffort](payload, objectName, "effort")
	if err != nil {
		return err
	}
	summary, err := decodeOptionalNullableConfigValue[ReasoningSummary](payload, objectName, "summary")
	if err != nil {
		return err
	}
	collaborationMode, err := decodeRequiredThreadItemValue[CollaborationMode](payload, objectName, "collaborationMode")
	if err != nil {
		return err
	}
	personality, err := decodeOptionalNullableConfigValue[Personality](payload, objectName, "personality")
	if err != nil {
		return err
	}
	*s = ThreadSettings{
		CWD:                     cwd,
		ApprovalPolicy:          approvalPolicy,
		ApprovalsReviewer:       approvalsReviewer,
		SandboxPolicy:           sandboxPolicy,
		ActivePermissionProfile: activePermissionProfile,
		Model:                   model,
		ModelProvider:           modelProvider,
		ServiceTier:             serviceTier,
		Effort:                  effort,
		Summary:                 summary,
		CollaborationMode:       collaborationMode,
		Personality:             personality,
	}
	return nil
}

func (n *ThreadSettingsUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread settings updated notification into nil receiver")
	}
	const objectName = "thread settings updated notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "threadSettings")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	threadSettings, err := decodeRequiredThreadItemValue[ThreadSettings](payload, objectName, "threadSettings")
	if err != nil {
		return err
	}
	*n = ThreadSettingsUpdatedNotification{ThreadID: threadID, ThreadSettings: threadSettings}
	return nil
}

func threadSettingsSchemas() map[string]Schema {
	return map[string]Schema{
		"ThreadExtra": {
			"description": "Extra app-server data for a thread.",
			"type":        "object",
		},
		"ThreadHistoryMode": stringEnumSchema("legacy", "paginated"),
		"ThreadSettings": {
			"properties": Schema{
				"activePermissionProfile": nullableSchemaRef("ActivePermissionProfile"),
				"approvalPolicy":          Schema{"$ref": "#/$defs/AskForApproval"},
				"approvalsReviewer":       Schema{"$ref": "#/$defs/ApprovalsReviewer"},
				"collaborationMode":       Schema{"$ref": "#/$defs/CollaborationMode"},
				"cwd":                     Schema{"$ref": "#/$defs/AbsolutePathBuf"},
				"effort":                  nullableSchemaRef("ReasoningEffort"),
				"model":                   Schema{"type": "string"},
				"modelProvider":           Schema{"type": "string"},
				"personality":             nullableSchemaRef("Personality"),
				"sandboxPolicy":           Schema{"$ref": "#/$defs/SandboxPolicy"},
				"serviceTier":             Schema{"type": []any{"string", "null"}},
				"summary":                 nullableSchemaRef("ReasoningSummary"),
			},
			"required": []string{
				"approvalPolicy", "approvalsReviewer", "collaborationMode", "cwd",
				"model", "modelProvider", "sandboxPolicy",
			},
			"type": "object",
		},
		"ThreadSettingsUpdatedNotification": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"threadId":       Schema{"type": "string"},
				"threadSettings": Schema{"$ref": "#/$defs/ThreadSettings"},
			},
			"required": []string{"threadId", "threadSettings"},
			"title":    "ThreadSettingsUpdatedNotification",
			"type":     "object",
		},
	}
}

var (
	_ json.Marshaler   = ThreadHistoryMode("")
	_ json.Unmarshaler = (*ThreadHistoryMode)(nil)
	_ json.Unmarshaler = (*ThreadExtra)(nil)
	_ json.Unmarshaler = (*ThreadSettings)(nil)
	_ json.Unmarshaler = (*ThreadSettingsUpdatedNotification)(nil)
)
