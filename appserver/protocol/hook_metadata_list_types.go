package protocol

import (
	"encoding/json"
	"errors"
)

// HookErrorInfo is one descriptive hook-discovery error. It does not imply
// that Gollem scans hook files.
type HookErrorInfo struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e HookErrorInfo) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}{Path: e.Path, Message: e.Message})
}

func (e *HookErrorInfo) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode hook error info into nil receiver")
	}
	const objectName = "hook error info"
	payload, err := decodeRustSerdeObject(data, objectName, "path", "message")
	if err != nil {
		return err
	}
	path, err := decodeRequiredThreadItemValue[string](payload, objectName, "path")
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, objectName, "message")
	if err != nil {
		return err
	}
	*e = HookErrorInfo{Path: path, Message: message}
	return nil
}

// HookMetadata is exact standalone public metadata for one configured hook.
// Its fields are descriptive and do not grant trust or execution authority.
type HookMetadata struct {
	Key           string          `json:"key"`
	EventName     HookEventName   `json:"eventName"`
	HandlerType   HookHandlerType `json:"handlerType"`
	Matcher       *string         `json:"matcher"`
	Command       *string         `json:"command"`
	TimeoutSec    uint64          `json:"timeoutSec"`
	StatusMessage *string         `json:"statusMessage"`
	SourcePath    AbsolutePathBuf `json:"sourcePath"`
	Source        HookSource      `json:"source"`
	PluginID      *string         `json:"pluginId"`
	DisplayOrder  int64           `json:"displayOrder"`
	Enabled       bool            `json:"enabled"`
	IsManaged     bool            `json:"isManaged"`
	CurrentHash   string          `json:"currentHash"`
	TrustStatus   HookTrustStatus `json:"trustStatus"`
}

func (m HookMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Key           string          `json:"key"`
		EventName     HookEventName   `json:"eventName"`
		HandlerType   HookHandlerType `json:"handlerType"`
		Matcher       *string         `json:"matcher"`
		Command       *string         `json:"command"`
		TimeoutSec    uint64          `json:"timeoutSec"`
		StatusMessage *string         `json:"statusMessage"`
		SourcePath    AbsolutePathBuf `json:"sourcePath"`
		Source        HookSource      `json:"source"`
		PluginID      *string         `json:"pluginId"`
		DisplayOrder  int64           `json:"displayOrder"`
		Enabled       bool            `json:"enabled"`
		IsManaged     bool            `json:"isManaged"`
		CurrentHash   string          `json:"currentHash"`
		TrustStatus   HookTrustStatus `json:"trustStatus"`
	}{
		Key: m.Key, EventName: m.EventName, HandlerType: m.HandlerType,
		Matcher: m.Matcher, Command: m.Command, TimeoutSec: m.TimeoutSec,
		StatusMessage: m.StatusMessage, SourcePath: m.SourcePath, Source: m.Source,
		PluginID: m.PluginID, DisplayOrder: m.DisplayOrder, Enabled: m.Enabled,
		IsManaged: m.IsManaged, CurrentHash: m.CurrentHash, TrustStatus: m.TrustStatus,
	})
}

func (m *HookMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode hook metadata into nil receiver")
	}
	const objectName = "hook metadata"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"key", "eventName", "handlerType", "matcher", "command", "timeoutSec",
		"statusMessage", "sourcePath", "source", "pluginId", "displayOrder",
		"enabled", "isManaged", "currentHash", "trustStatus",
	)
	if err != nil {
		return err
	}
	key, err := decodeRequiredThreadItemValue[string](payload, objectName, "key")
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
	matcher, err := decodeOptionalNullableConfigValue[string](payload, objectName, "matcher")
	if err != nil {
		return err
	}
	command, err := decodeOptionalNullableConfigValue[string](payload, objectName, "command")
	if err != nil {
		return err
	}
	timeoutSec, err := decodeRequiredThreadItemValue[uint64](payload, objectName, "timeoutSec")
	if err != nil {
		return err
	}
	statusMessage, err := decodeOptionalNullableConfigValue[string](payload, objectName, "statusMessage")
	if err != nil {
		return err
	}
	sourcePath, err := decodeRequiredThreadItemValue[AbsolutePathBuf](payload, objectName, "sourcePath")
	if err != nil {
		return err
	}
	source, err := decodeRequiredThreadItemValue[HookSource](payload, objectName, "source")
	if err != nil {
		return err
	}
	pluginID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "pluginId")
	if err != nil {
		return err
	}
	displayOrder, err := decodeRequiredThreadItemValue[int64](payload, objectName, "displayOrder")
	if err != nil {
		return err
	}
	enabled, err := decodeRequiredThreadItemValue[bool](payload, objectName, "enabled")
	if err != nil {
		return err
	}
	isManaged, err := decodeRequiredThreadItemValue[bool](payload, objectName, "isManaged")
	if err != nil {
		return err
	}
	currentHash, err := decodeRequiredThreadItemValue[string](payload, objectName, "currentHash")
	if err != nil {
		return err
	}
	trustStatus, err := decodeRequiredThreadItemValue[HookTrustStatus](payload, objectName, "trustStatus")
	if err != nil {
		return err
	}
	*m = HookMetadata{
		Key: key, EventName: eventName, HandlerType: handlerType,
		Matcher: matcher, Command: command, TimeoutSec: timeoutSec,
		StatusMessage: statusMessage, SourcePath: sourcePath, Source: source,
		PluginID: pluginID, DisplayOrder: displayOrder, Enabled: enabled,
		IsManaged: isManaged, CurrentHash: currentHash, TrustStatus: trustStatus,
	}
	return nil
}

// HooksListEntry is exact standalone hook metadata and diagnostics for one cwd.
type HooksListEntry struct {
	CWD      string          `json:"cwd"`
	Hooks    []HookMetadata  `json:"hooks"`
	Warnings []string        `json:"warnings"`
	Errors   []HookErrorInfo `json:"errors"`
}

func (e HooksListEntry) MarshalJSON() ([]byte, error) {
	hooks := e.Hooks
	if hooks == nil {
		hooks = []HookMetadata{}
	}
	warnings := e.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	errors := e.Errors
	if errors == nil {
		errors = []HookErrorInfo{}
	}
	return json.Marshal(struct {
		CWD      string          `json:"cwd"`
		Hooks    []HookMetadata  `json:"hooks"`
		Warnings []string        `json:"warnings"`
		Errors   []HookErrorInfo `json:"errors"`
	}{CWD: e.CWD, Hooks: hooks, Warnings: warnings, Errors: errors})
}

func (e *HooksListEntry) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode hooks list entry into nil receiver")
	}
	const objectName = "hooks list entry"
	payload, err := decodeRustSerdeObject(data, objectName, "cwd", "hooks", "warnings", "errors")
	if err != nil {
		return err
	}
	cwd, err := decodeRequiredThreadItemValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	hooks, err := decodeRequiredThreadItemArray[HookMetadata](payload, objectName, "hooks")
	if err != nil {
		return err
	}
	warnings, err := decodeRequiredThreadItemArray[string](payload, objectName, "warnings")
	if err != nil {
		return err
	}
	errors, err := decodeRequiredThreadItemArray[HookErrorInfo](payload, objectName, "errors")
	if err != nil {
		return err
	}
	*e = HooksListEntry{CWD: cwd, Hooks: hooks, Warnings: warnings, Errors: errors}
	return nil
}

// HooksListParams is the exact standalone public request shape. Gollem does
// not bind it to hooks/list or interpret its working directories.
type HooksListParams struct {
	CWDs []string `json:"cwds,omitempty"`
}

func (p HooksListParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		CWDs []string `json:"cwds,omitempty"`
	}{CWDs: p.CWDs})
}

func (p *HooksListParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode hooks list params into nil receiver")
	}
	const objectName = "hooks list params"
	payload, err := decodeRustSerdeObject(data, objectName, "cwds")
	if err != nil {
		return err
	}
	cwds := []string{}
	if _, ok := payload["cwds"]; ok {
		cwds, err = decodeRequiredThreadItemArray[string](payload, objectName, "cwds")
		if err != nil {
			return err
		}
	}
	*p = HooksListParams{CWDs: cwds}
	return nil
}

// HooksListResponse is the exact standalone public hook-list envelope.
type HooksListResponse struct {
	Data []HooksListEntry `json:"data"`
}

func (r HooksListResponse) MarshalJSON() ([]byte, error) {
	data := r.Data
	if data == nil {
		data = []HooksListEntry{}
	}
	return json.Marshal(struct {
		Data []HooksListEntry `json:"data"`
	}{Data: data})
}

func (r *HooksListResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode hooks list response into nil receiver")
	}
	const objectName = "hooks list response"
	payload, err := decodeRustSerdeObject(data, objectName, "data")
	if err != nil {
		return err
	}
	entries, err := decodeRequiredThreadItemArray[HooksListEntry](payload, objectName, "data")
	if err != nil {
		return err
	}
	*r = HooksListResponse{Data: entries}
	return nil
}

func hookErrorInfoSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"path": Schema{"type": "string"}, "message": Schema{"type": "string"},
	}, []string{"message", "path"})
}

func hookMetadataSchema() Schema {
	nullableString := Schema{"type": []any{"string", "null"}}
	return closedThreadSessionParamSchema(Schema{
		"key":           Schema{"type": "string"},
		"eventName":     Schema{"$ref": "#/$defs/HookEventName"},
		"handlerType":   Schema{"$ref": "#/$defs/HookHandlerType"},
		"matcher":       nullableString,
		"command":       nullableString,
		"timeoutSec":    Schema{"type": "integer", "format": "uint64", "minimum": float64(0)},
		"statusMessage": nullableString,
		"sourcePath":    Schema{"$ref": "#/$defs/AbsolutePathBuf"},
		"source":        Schema{"$ref": "#/$defs/HookSource"},
		"pluginId":      nullableString,
		"displayOrder":  Schema{"type": "integer", "format": "int64"},
		"enabled":       Schema{"type": "boolean"},
		"isManaged":     Schema{"type": "boolean"},
		"currentHash":   Schema{"type": "string"},
		"trustStatus":   Schema{"$ref": "#/$defs/HookTrustStatus"},
	}, []string{
		"currentHash", "displayOrder", "enabled", "eventName", "handlerType",
		"isManaged", "key", "source", "sourcePath", "timeoutSec", "trustStatus",
	})
}

func hooksListEntrySchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"cwd":      Schema{"type": "string"},
		"hooks":    Schema{"type": "array", "items": Schema{"$ref": "#/$defs/HookMetadata"}},
		"warnings": Schema{"type": "array", "items": Schema{"type": "string"}},
		"errors":   Schema{"type": "array", "items": Schema{"$ref": "#/$defs/HookErrorInfo"}},
	}, []string{"cwd", "errors", "hooks", "warnings"})
}

func hooksListParamsSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"cwds": Schema{
			"type": "array", "items": Schema{"type": "string"},
			"description": "When empty, defaults to the current session working directory.",
		},
	}, nil)
}

func hooksListResponseSchema() Schema {
	return closedThreadSessionParamSchema(Schema{
		"data": Schema{"type": "array", "items": Schema{"$ref": "#/$defs/HooksListEntry"}},
	}, []string{"data"})
}

var (
	_ json.Marshaler   = HookErrorInfo{}
	_ json.Unmarshaler = (*HookErrorInfo)(nil)
	_ json.Marshaler   = HookMetadata{}
	_ json.Unmarshaler = (*HookMetadata)(nil)
	_ json.Marshaler   = HooksListEntry{}
	_ json.Unmarshaler = (*HooksListEntry)(nil)
	_ json.Marshaler   = HooksListParams{}
	_ json.Unmarshaler = (*HooksListParams)(nil)
	_ json.Marshaler   = HooksListResponse{}
	_ json.Unmarshaler = (*HooksListResponse)(nil)
)
