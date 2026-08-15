package protocol

import (
	"encoding/json"
	"errors"
)

// EnvironmentConnectionNotification identifies a thread environment transition.
type EnvironmentConnectionNotification struct {
	ThreadID      string `json:"threadId"`
	EnvironmentID string `json:"environmentId"`
}

func (n *EnvironmentConnectionNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode environment connection notification into nil receiver")
	}
	const objectName = "environment connection notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "environmentId")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	environmentID, err := decodeRequiredThreadItemValue[string](payload, objectName, "environmentId")
	if err != nil {
		return err
	}
	*n = EnvironmentConnectionNotification{ThreadID: threadID, EnvironmentID: environmentID}
	return nil
}

// SkillsChangedNotification invalidates watched local skill metadata.
type SkillsChangedNotification struct{}

func (n *SkillsChangedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode skills changed notification into nil receiver")
	}
	if _, err := decodeRustSerdeObject(data, "skills changed notification"); err != nil {
		return err
	}
	*n = SkillsChangedNotification{}
	return nil
}

// ThreadQueueChangedNotification identifies a thread whose queue changed.
type ThreadQueueChangedNotification struct {
	ThreadID string `json:"threadId"`
}

func (n *ThreadQueueChangedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread queue changed notification into nil receiver")
	}
	const objectName = "thread queue changed notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	*n = ThreadQueueChangedNotification{ThreadID: threadID}
	return nil
}

// ThreadRevertedNotification identifies a thread with reverted history.
type ThreadRevertedNotification struct {
	ThreadID string `json:"threadId"`
}

func (n *ThreadRevertedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode thread reverted notification into nil receiver")
	}
	const objectName = "thread reverted notification"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	*n = ThreadRevertedNotification{ThreadID: threadID}
	return nil
}

// RemoteControlConnectionStatus is the exact closed remote-control state.
type RemoteControlConnectionStatus string

const (
	RemoteControlConnectionStatusDisabled   RemoteControlConnectionStatus = "disabled"
	RemoteControlConnectionStatusConnecting RemoteControlConnectionStatus = "connecting"
	RemoteControlConnectionStatusConnected  RemoteControlConnectionStatus = "connected"
	RemoteControlConnectionStatusErrored    RemoteControlConnectionStatus = "errored"
)

func (s RemoteControlConnectionStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "remote control connection status", RemoteControlConnectionStatus.valid)
}

func (s *RemoteControlConnectionStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "remote control connection status", RemoteControlConnectionStatus.valid)
}

func (s RemoteControlConnectionStatus) valid() bool {
	return s == RemoteControlConnectionStatusDisabled ||
		s == RemoteControlConnectionStatusConnecting ||
		s == RemoteControlConnectionStatusConnected ||
		s == RemoteControlConnectionStatusErrored
}

// RemoteControlStatusChangedNotification reports bounded remote-control state.
type RemoteControlStatusChangedNotification struct {
	Status         RemoteControlConnectionStatus `json:"status"`
	ServerName     string                        `json:"serverName"`
	InstallationID string                        `json:"installationId"`
	EnvironmentID  *string                       `json:"environmentId,omitempty"`
}

func (n RemoteControlStatusChangedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Status         RemoteControlConnectionStatus `json:"status"`
		ServerName     string                        `json:"serverName"`
		InstallationID string                        `json:"installationId"`
		EnvironmentID  *string                       `json:"environmentId"`
	}{
		Status: n.Status, ServerName: n.ServerName, InstallationID: n.InstallationID, EnvironmentID: n.EnvironmentID,
	})
}

func (n *RemoteControlStatusChangedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode remote control status changed notification into nil receiver")
	}
	const objectName = "remote control status changed notification"
	payload, err := decodeRustSerdeObject(data, objectName, "status", "serverName", "installationId", "environmentId")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[RemoteControlConnectionStatus](payload, objectName, "status")
	if err != nil {
		return err
	}
	serverName, err := decodeRequiredThreadItemValue[string](payload, objectName, "serverName")
	if err != nil {
		return err
	}
	installationID, err := decodeRequiredThreadItemValue[string](payload, objectName, "installationId")
	if err != nil {
		return err
	}
	environmentID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "environmentId")
	if err != nil {
		return err
	}
	*n = RemoteControlStatusChangedNotification{
		Status: status, ServerName: serverName, InstallationID: installationID, EnvironmentID: environmentID,
	}
	return nil
}

// WindowsSandboxSetupCompletedNotification reports a Windows sandbox setup result.
type WindowsSandboxSetupCompletedNotification struct {
	Mode    WindowsSandboxSetupMode `json:"mode"`
	Success bool                    `json:"success"`
	Error   *string                 `json:"error,omitempty"`
}

func (n WindowsSandboxSetupCompletedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Mode    WindowsSandboxSetupMode `json:"mode"`
		Success bool                    `json:"success"`
		Error   *string                 `json:"error"`
	}{Mode: n.Mode, Success: n.Success, Error: n.Error})
}

func (n *WindowsSandboxSetupCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode Windows sandbox setup completed notification into nil receiver")
	}
	const objectName = "Windows sandbox setup completed notification"
	payload, err := decodeRustSerdeObject(data, objectName, "mode", "success", "error")
	if err != nil {
		return err
	}
	mode, err := decodeRequiredThreadItemValue[WindowsSandboxSetupMode](payload, objectName, "mode")
	if err != nil {
		return err
	}
	success, err := decodeRequiredThreadItemValue[bool](payload, objectName, "success")
	if err != nil {
		return err
	}
	message, err := decodeOptionalNullableConfigValue[string](payload, objectName, "error")
	if err != nil {
		return err
	}
	*n = WindowsSandboxSetupCompletedNotification{Mode: mode, Success: success, Error: message}
	return nil
}

// WindowsWorldWritableWarningNotification reports unprotected Windows paths.
type WindowsWorldWritableWarningNotification struct {
	SamplePaths []string `json:"samplePaths"`
	ExtraCount  uint64   `json:"extraCount"`
	FailedScan  bool     `json:"failedScan"`
}

func (n WindowsWorldWritableWarningNotification) MarshalJSON() ([]byte, error) {
	samplePaths := n.SamplePaths
	if samplePaths == nil {
		samplePaths = []string{}
	}
	return json.Marshal(struct {
		SamplePaths []string `json:"samplePaths"`
		ExtraCount  uint64   `json:"extraCount"`
		FailedScan  bool     `json:"failedScan"`
	}{SamplePaths: samplePaths, ExtraCount: n.ExtraCount, FailedScan: n.FailedScan})
}

func (n *WindowsWorldWritableWarningNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode Windows world writable warning notification into nil receiver")
	}
	const objectName = "Windows world writable warning notification"
	payload, err := decodeRustSerdeObject(data, objectName, "samplePaths", "extraCount", "failedScan")
	if err != nil {
		return err
	}
	samplePaths, err := decodeRequiredThreadItemArray[string](payload, objectName, "samplePaths")
	if err != nil {
		return err
	}
	extraCount, err := decodeRequiredThreadItemValue[uint64](payload, objectName, "extraCount")
	if err != nil {
		return err
	}
	failedScan, err := decodeRequiredThreadItemValue[bool](payload, objectName, "failedScan")
	if err != nil {
		return err
	}
	*n = WindowsWorldWritableWarningNotification{
		SamplePaths: samplePaths, ExtraCount: extraCount, FailedScan: failedScan,
	}
	return nil
}

func operationalNotificationLeafSchemas() map[string]Schema {
	object := func(title string, properties Schema, required ...string) Schema {
		out := Schema{"$schema": "http://json-schema.org/draft-07/schema#", "properties": properties, "title": title, "type": "object"}
		if len(required) != 0 {
			out["required"] = required
		}
		return out
	}
	return map[string]Schema{
		"EnvironmentConnectionNotification": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"environmentId": Schema{"type": "string"},
				"threadId":      Schema{"type": "string"},
			},
			"required": []string{"environmentId", "threadId"},
			"title":    "EnvironmentConnectionNotification",
			"type":     "object",
		},
		"RemoteControlConnectionStatus": stringEnumSchema(
			string(RemoteControlConnectionStatusDisabled), string(RemoteControlConnectionStatusConnecting),
			string(RemoteControlConnectionStatusConnected), string(RemoteControlConnectionStatusErrored),
		),
		"RemoteControlStatusChangedNotification": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "Current remote-control connection status and remote identity exposed to clients.",
			"properties": Schema{
				"environmentId":  Schema{"type": []any{"string", "null"}},
				"installationId": Schema{"type": "string"},
				"serverName":     Schema{"type": "string"},
				"status":         Schema{"$ref": "#/$defs/RemoteControlConnectionStatus"},
			},
			"required": []string{"installationId", "serverName", "status"},
			"title":    "RemoteControlStatusChangedNotification",
			"type":     "object",
		},
		"SkillsChangedNotification": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"description": "Notification emitted when watched local skill files change.\n\n" +
				"Treat this as an invalidation signal and re-run `skills/list` with the client's current parameters when refreshed skill metadata is needed.",
			"title": "SkillsChangedNotification",
			"type":  "object",
		},
		"ThreadQueueChangedNotification": object("ThreadQueueChangedNotification", Schema{"threadId": Schema{"type": "string"}}, "threadId"),
		"ThreadRevertedNotification":     object("ThreadRevertedNotification", Schema{"threadId": Schema{"type": "string"}}, "threadId"),
		"WindowsSandboxSetupCompletedNotification": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"error":   Schema{"type": []any{"string", "null"}},
				"mode":    Schema{"$ref": "#/$defs/WindowsSandboxSetupMode"},
				"success": Schema{"type": "boolean"},
			},
			"required": []string{"mode", "success"},
			"title":    "WindowsSandboxSetupCompletedNotification",
			"type":     "object",
		},
		"WindowsWorldWritableWarningNotification": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"extraCount":  Schema{"format": "uint", "minimum": 0, "type": "integer"},
				"failedScan":  Schema{"type": "boolean"},
				"samplePaths": Schema{"items": Schema{"type": "string"}, "type": "array"},
			},
			"required": []string{"extraCount", "failedScan", "samplePaths"},
			"title":    "WindowsWorldWritableWarningNotification",
			"type":     "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*EnvironmentConnectionNotification)(nil)
	_ json.Unmarshaler = (*SkillsChangedNotification)(nil)
	_ json.Unmarshaler = (*ThreadQueueChangedNotification)(nil)
	_ json.Unmarshaler = (*ThreadRevertedNotification)(nil)
	_ json.Marshaler   = RemoteControlConnectionStatus("")
	_ json.Unmarshaler = (*RemoteControlConnectionStatus)(nil)
	_ json.Marshaler   = RemoteControlStatusChangedNotification{}
	_ json.Unmarshaler = (*RemoteControlStatusChangedNotification)(nil)
	_ json.Marshaler   = WindowsSandboxSetupCompletedNotification{}
	_ json.Unmarshaler = (*WindowsSandboxSetupCompletedNotification)(nil)
	_ json.Marshaler   = WindowsWorldWritableWarningNotification{}
	_ json.Unmarshaler = (*WindowsWorldWritableWarningNotification)(nil)
)
