package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// HookRunSummary is the exact standalone public summary of one hook run. It
// does not imply that Gollem discovers or executes hooks.
type HookRunSummary struct {
	ID            string            `json:"id"`
	EventName     HookEventName     `json:"eventName"`
	HandlerType   HookHandlerType   `json:"handlerType"`
	ExecutionMode HookExecutionMode `json:"executionMode"`
	Scope         HookScope         `json:"scope"`
	SourcePath    AbsolutePathBuf   `json:"sourcePath"`
	Source        HookSource        `json:"source"`
	DisplayOrder  int64             `json:"displayOrder"`
	Status        HookRunStatus     `json:"status"`
	StatusMessage *string           `json:"statusMessage"`
	StartedAt     int64             `json:"startedAt"`
	CompletedAt   *int64            `json:"completedAt"`
	DurationMS    *int64            `json:"durationMs"`
	Entries       []HookOutputEntry `json:"entries"`
}

func (s HookRunSummary) MarshalJSON() ([]byte, error) {
	entries := s.Entries
	if entries == nil {
		entries = []HookOutputEntry{}
	}
	return json.Marshal(struct {
		ID            string            `json:"id"`
		EventName     HookEventName     `json:"eventName"`
		HandlerType   HookHandlerType   `json:"handlerType"`
		ExecutionMode HookExecutionMode `json:"executionMode"`
		Scope         HookScope         `json:"scope"`
		SourcePath    AbsolutePathBuf   `json:"sourcePath"`
		Source        HookSource        `json:"source"`
		DisplayOrder  int64             `json:"displayOrder"`
		Status        HookRunStatus     `json:"status"`
		StatusMessage *string           `json:"statusMessage"`
		StartedAt     int64             `json:"startedAt"`
		CompletedAt   *int64            `json:"completedAt"`
		DurationMS    *int64            `json:"durationMs"`
		Entries       []HookOutputEntry `json:"entries"`
	}{
		ID: s.ID, EventName: s.EventName, HandlerType: s.HandlerType,
		ExecutionMode: s.ExecutionMode, Scope: s.Scope, SourcePath: s.SourcePath,
		Source: s.Source, DisplayOrder: s.DisplayOrder, Status: s.Status,
		StatusMessage: s.StatusMessage, StartedAt: s.StartedAt,
		CompletedAt: s.CompletedAt, DurationMS: s.DurationMS, Entries: entries,
	})
}

func (s *HookRunSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode hook run summary into nil receiver")
	}
	const objectName = "hook run summary"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"id", "eventName", "handlerType", "executionMode", "scope", "sourcePath",
		"source", "displayOrder", "status", "statusMessage", "startedAt",
		"completedAt", "durationMs", "entries",
	)
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return err
	}
	eventName, err := decodeRequiredThreadItemValue[HookEventName](payload, objectName, "eventName")
	if err != nil {
		return err
	}
	handlerType, err := decodeRequiredThreadItemValue[HookHandlerType](payload, objectName, "handlerType")
	if err != nil {
		return err
	}
	executionMode, err := decodeRequiredThreadItemValue[HookExecutionMode](payload, objectName, "executionMode")
	if err != nil {
		return err
	}
	scope, err := decodeRequiredThreadItemValue[HookScope](payload, objectName, "scope")
	if err != nil {
		return err
	}
	sourcePath, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "sourcePath")
	if err != nil {
		return err
	}
	source, err := decodeHookRunSource(payload, objectName)
	if err != nil {
		return err
	}
	displayOrder, err := decodeRequiredThreadItemValue[int64](payload, objectName, "displayOrder")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[HookRunStatus](payload, objectName, "status")
	if err != nil {
		return err
	}
	statusMessage, err := decodeOptionalNullableConfigValue[string](payload, objectName, "statusMessage")
	if err != nil {
		return err
	}
	startedAt, err := decodeRequiredThreadItemValue[int64](payload, objectName, "startedAt")
	if err != nil {
		return err
	}
	completedAt, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "completedAt")
	if err != nil {
		return err
	}
	durationMS, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "durationMs")
	if err != nil {
		return err
	}
	entries, err := decodeRequiredThreadItemArray[HookOutputEntry](payload, objectName, "entries")
	if err != nil {
		return err
	}
	*s = HookRunSummary{
		ID: id, EventName: eventName, HandlerType: handlerType,
		ExecutionMode: executionMode, Scope: scope, SourcePath: sourcePath,
		Source: source, DisplayOrder: displayOrder, Status: status,
		StatusMessage: statusMessage, StartedAt: startedAt, CompletedAt: completedAt,
		DurationMS: durationMS, Entries: entries,
	}
	return nil
}

func decodeHookRunSource(payload map[string]json.RawMessage, objectName string) (HookSource, error) {
	raw, ok := payload["source"]
	if !ok {
		return HookSourceUnknown, nil
	}
	if isJSONNull(raw) {
		return "", fmt.Errorf("%s source cannot be null", objectName)
	}
	var source HookSource
	if err := json.Unmarshal(raw, &source); err != nil {
		return "", fmt.Errorf("decode %s source: %w", objectName, err)
	}
	return source, nil
}

// HookStartedNotification is the exact standalone public hook-start payload.
type HookStartedNotification struct {
	ThreadID string         `json:"threadId"`
	TurnID   *string        `json:"turnId"`
	Run      HookRunSummary `json:"run"`
}

func (n HookStartedNotification) MarshalJSON() ([]byte, error) {
	return marshalHookRunNotification(n.ThreadID, n.TurnID, n.Run)
}

func (n *HookStartedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode hook-started notification into nil receiver")
	}
	threadID, turnID, run, err := unmarshalHookRunNotification(data, "hook-started notification")
	if err != nil {
		return err
	}
	*n = HookStartedNotification{ThreadID: threadID, TurnID: turnID, Run: run}
	return nil
}

// HookCompletedNotification is the exact standalone public hook-completion payload.
type HookCompletedNotification struct {
	ThreadID string         `json:"threadId"`
	TurnID   *string        `json:"turnId"`
	Run      HookRunSummary `json:"run"`
}

func (n HookCompletedNotification) MarshalJSON() ([]byte, error) {
	return marshalHookRunNotification(n.ThreadID, n.TurnID, n.Run)
}

func (n *HookCompletedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode hook-completed notification into nil receiver")
	}
	threadID, turnID, run, err := unmarshalHookRunNotification(data, "hook-completed notification")
	if err != nil {
		return err
	}
	*n = HookCompletedNotification{ThreadID: threadID, TurnID: turnID, Run: run}
	return nil
}

func marshalHookRunNotification(threadID string, turnID *string, run HookRunSummary) ([]byte, error) {
	return json.Marshal(struct {
		ThreadID string         `json:"threadId"`
		TurnID   *string        `json:"turnId"`
		Run      HookRunSummary `json:"run"`
	}{ThreadID: threadID, TurnID: turnID, Run: run})
}

func unmarshalHookRunNotification(data []byte, objectName string) (string, *string, HookRunSummary, error) {
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "turnId", "run")
	if err != nil {
		return "", nil, HookRunSummary{}, err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return "", nil, HookRunSummary{}, err
	}
	turnID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "turnId")
	if err != nil {
		return "", nil, HookRunSummary{}, err
	}
	run, err := decodeRequiredThreadItemValue[HookRunSummary](payload, objectName, "run")
	if err != nil {
		return "", nil, HookRunSummary{}, err
	}
	return threadID, turnID, run, nil
}

func hookRunSummarySchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"id":            Schema{"type": "string"},
		"eventName":     Schema{"$ref": "#/$defs/HookEventName"},
		"handlerType":   Schema{"$ref": "#/$defs/HookHandlerType"},
		"executionMode": Schema{"$ref": "#/$defs/HookExecutionMode"},
		"scope":         Schema{"$ref": "#/$defs/HookScope"},
		"sourcePath":    Schema{"$ref": "#/$defs/AbsolutePathBuf"},
		"source": Schema{
			"allOf":   []any{Schema{"$ref": "#/$defs/HookSource"}},
			"default": "unknown",
		},
		"displayOrder":  Schema{"type": "integer", "format": "int64"},
		"status":        Schema{"$ref": "#/$defs/HookRunStatus"},
		"statusMessage": Schema{"type": []any{"string", "null"}},
		"startedAt":     Schema{"type": "integer", "format": "int64"},
		"completedAt":   Schema{"type": []any{"integer", "null"}, "format": "int64"},
		"durationMs":    Schema{"type": []any{"integer", "null"}, "format": "int64"},
		"entries": Schema{
			"type": "array", "items": Schema{"$ref": "#/$defs/HookOutputEntry"},
		},
	}, []string{
		"displayOrder", "entries", "eventName", "executionMode", "handlerType",
		"id", "scope", "sourcePath", "startedAt", "status",
	})
}

func hookRunNotificationSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"threadId": Schema{"type": "string"},
		"turnId":   Schema{"type": []any{"string", "null"}},
		"run":      Schema{"$ref": "#/$defs/HookRunSummary"},
	}, []string{"run", "threadId"})
}

var (
	_ json.Marshaler   = HookRunSummary{}
	_ json.Unmarshaler = (*HookRunSummary)(nil)
	_ json.Marshaler   = HookStartedNotification{}
	_ json.Unmarshaler = (*HookStartedNotification)(nil)
	_ json.Marshaler   = HookCompletedNotification{}
	_ json.Unmarshaler = (*HookCompletedNotification)(nil)
)
