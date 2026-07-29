package protocol

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf16"
)

type ModeKind string

const (
	ModeKindPlan    ModeKind = "plan"
	ModeKindDefault ModeKind = "default"
)

// MultiAgentMode is standalone policy-description data. It does not enable
// delegation or subagent execution.
type MultiAgentMode struct{ raw json.RawMessage }

// Settings is the exact public settings record used by a collaboration mode.
// Optional serde inputs remain explicit nulls in canonical output.
type Settings struct {
	Model                 string           `json:"model"`
	ReasoningEffort       *ReasoningEffort `json:"reasoning_effort"`
	DeveloperInstructions *string          `json:"developer_instructions"`
}

// CollaborationMode is standalone session configuration data.
type CollaborationMode struct {
	Mode     ModeKind `json:"mode"`
	Settings Settings `json:"settings"`
}

// CollaborationModeMask is the public app-server preset metadata record.
// Its nested optional effort collapses to one nullable Go pointer on the wire.
type CollaborationModeMask struct {
	Name            string           `json:"name"`
	Mode            *ModeKind        `json:"mode"`
	Model           *string          `json:"model"`
	ReasoningEffort *ReasoningEffort `json:"reasoning_effort"`
}

// CapabilityRootLocation describes an environment-owned path. It does not
// resolve the environment or grant access to the path.
type CapabilityRootLocation struct{ raw json.RawMessage }

// SelectedCapabilityRoot is standalone capability-selection data.
type SelectedCapabilityRoot struct {
	ID       string                 `json:"id"`
	Location CapabilityRootLocation `json:"location"`
}

func (m ModeKind) MarshalJSON() ([]byte, error) {
	if m != ModeKindPlan && m != ModeKindDefault {
		return nil, fmt.Errorf("unsupported collaboration mode kind %q", m)
	}
	return json.Marshal(string(m))
}

func (m *ModeKind) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode collaboration mode kind into nil receiver")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode collaboration mode kind: %w", err)
	}
	switch value {
	case "plan":
		*m = ModeKindPlan
	case "default", "code", "pair_programming", "execute", "custom":
		*m = ModeKindDefault
	default:
		return fmt.Errorf("unsupported collaboration mode kind %q", value)
	}
	return nil
}

func (m MultiAgentMode) MarshalJSON() ([]byte, error) {
	if len(m.raw) == 0 {
		return nil, errors.New("multi-agent mode is empty")
	}
	return validateMultiAgentMode(m.raw)
}

func (m *MultiAgentMode) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode multi-agent mode into nil receiver")
	}
	canonical, err := validateMultiAgentMode(data)
	if err != nil {
		return err
	}
	m.raw = canonical
	return nil
}

func validateMultiAgentMode(data []byte) ([]byte, error) {
	var simple string
	if err := json.Unmarshal(data, &simple); err == nil {
		switch simple {
		case "explicitRequestOnly", "proactive":
			return json.Marshal(simple)
		case "none":
			return []byte(`{"custom":""}`), nil
		default:
			return nil, fmt.Errorf("unsupported multi-agent mode %q", simple)
		}
	}

	payload, err := decodeRustSerdeObject(data, "multi-agent mode", "custom")
	if err != nil {
		return nil, err
	}
	var allFields map[string]json.RawMessage
	_ = json.Unmarshal(data, &allFields) // decodeRustSerdeObject already validated the object.
	if len(allFields) != 1 {
		return nil, errors.New("multi-agent mode custom variant requires only custom")
	}
	custom, err := decodeRequiredThreadItemValue[string](payload, "multi-agent mode", "custom")
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Custom string `json:"custom"`
	}{Custom: custom})
}

func (s *Settings) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode collaboration settings into nil receiver")
	}
	const objectName = "collaboration settings"
	payload, err := decodeRustSerdeObject(
		data, objectName, "model", "reasoning_effort", "developer_instructions",
	)
	if err != nil {
		return err
	}
	model, err := decodeRequiredThreadItemValue[string](payload, objectName, "model")
	if err != nil {
		return err
	}
	reasoningEffort, err := decodeOptionalCollaborationValue[ReasoningEffort](
		payload, objectName, "reasoning_effort",
	)
	if err != nil {
		return err
	}
	developerInstructions, err := decodeOptionalCollaborationValue[string](
		payload, objectName, "developer_instructions",
	)
	if err != nil {
		return err
	}
	*s = Settings{
		Model: model, ReasoningEffort: reasoningEffort,
		DeveloperInstructions: developerInstructions,
	}
	return nil
}

func (m *CollaborationMode) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode collaboration mode into nil receiver")
	}
	const objectName = "collaboration mode"
	payload, err := decodeRustSerdeObject(data, objectName, "mode", "settings")
	if err != nil {
		return err
	}
	mode, err := decodeRequiredThreadItemValue[ModeKind](payload, objectName, "mode")
	if err != nil {
		return err
	}
	settings, err := decodeRequiredThreadItemValue[Settings](payload, objectName, "settings")
	if err != nil {
		return err
	}
	*m = CollaborationMode{Mode: mode, Settings: settings}
	return nil
}

func (m *CollaborationModeMask) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode collaboration mode mask into nil receiver")
	}
	const objectName = "collaboration mode mask"
	payload, err := decodeRustSerdeObject(
		data, objectName, "name", "mode", "model", "reasoning_effort",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	mode, err := decodeOptionalCollaborationValue[ModeKind](payload, objectName, "mode")
	if err != nil {
		return err
	}
	model, err := decodeOptionalCollaborationValue[string](payload, objectName, "model")
	if err != nil {
		return err
	}
	reasoningEffort, err := decodeOptionalCollaborationValue[ReasoningEffort](
		payload, objectName, "reasoning_effort",
	)
	if err != nil {
		return err
	}
	*m = CollaborationModeMask{
		Name: name, Mode: mode, Model: model, ReasoningEffort: reasoningEffort,
	}
	return nil
}

func (l CapabilityRootLocation) MarshalJSON() ([]byte, error) {
	if len(l.raw) == 0 {
		return nil, errors.New("capability root location is empty")
	}
	return validateCapabilityRootLocation(l.raw)
}

func (l *CapabilityRootLocation) UnmarshalJSON(data []byte) error {
	if l == nil {
		return errors.New("decode capability root location into nil receiver")
	}
	canonical, err := validateCapabilityRootLocation(data)
	if err != nil {
		return err
	}
	l.raw = canonical
	return nil
}

func validateCapabilityRootLocation(data []byte) ([]byte, error) {
	const objectName = "capability root location"
	payload, err := decodeRustSerdeObject(data, objectName, "type", "environmentId", "path")
	if err != nil {
		return nil, err
	}
	locationType, err := decodeRequiredThreadItemValue[string](payload, objectName, "type")
	if err != nil {
		return nil, err
	}
	if locationType != "environment" {
		return nil, fmt.Errorf("unsupported capability root location type %q", locationType)
	}
	environmentID, err := decodeRequiredThreadItemValue[string](payload, objectName, "environmentId")
	if err != nil {
		return nil, err
	}
	rawPath, err := decodeRequiredThreadItemValue[string](payload, objectName, "path")
	if err != nil {
		return nil, err
	}
	canonicalPath, err := canonicalCapabilityRootPath(rawPath)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type          string `json:"type"`
		EnvironmentID string `json:"environmentId"`
		Path          string `json:"path"`
	}{
		Type: locationType, EnvironmentID: environmentID, Path: canonicalPath,
	})
}

func (r *SelectedCapabilityRoot) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode selected capability root into nil receiver")
	}
	const objectName = "selected capability root"
	payload, err := decodeRustSerdeObject(data, objectName, "id", "location")
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return err
	}
	location, err := decodeRequiredThreadItemValue[CapabilityRootLocation](
		payload, objectName, "location",
	)
	if err != nil {
		return err
	}
	*r = SelectedCapabilityRoot{ID: id, Location: location}
	return nil
}

func decodeOptionalCollaborationValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func canonicalCapabilityRootPath(value string) (string, error) {
	if isWindowsDriveAbsolutePath(value) {
		if strings.Contains(value, "\x00") {
			return opaqueWindowsCapabilityPathURI(value), nil
		}
		segments := append(
			[]string{value[:2]},
			splitWindowsCapabilityPath(value[3:])...,
		)
		return capabilityFileURI(
			"", normalizeNativeCapabilitySegments(segments, 1, true),
		), nil
	}
	if strings.HasPrefix(value, `\\`) {
		if isWindowsNamespacePath(value) || strings.Contains(value, "\x00") {
			return opaqueWindowsCapabilityPathURI(value), nil
		}
		components := splitWindowsCapabilityPath(value[2:])
		if len(components) < 2 || components[0] == "" || components[1] == "" {
			return "", fmt.Errorf("capability root path %q is not a valid UNC path", value)
		}
		host, ok := canonicalCapabilityURIHost(components[0])
		if !ok {
			return opaqueWindowsCapabilityPathURI(value), nil
		}
		segments := normalizeNativeCapabilitySegments(
			append([]string{components[1]}, components[2:]...), 1, false,
		)
		return capabilityFileURI(host, segments), nil
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" {
		if !strings.EqualFold(parsed.Scheme, "file") {
			return "", fmt.Errorf("capability root path %q must use file scheme", value)
		}
		if parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", fmt.Errorf("capability root path %q contains unsupported URI metadata", value)
		}
		parsed.Scheme = "file"
		if parsed.Opaque != "" {
			parsed.Path = "/" + strings.TrimLeft(parsed.Opaque, "/")
			parsed.Opaque = ""
			parsed.RawPath = ""
		}
		if strings.EqualFold(parsed.Host, "localhost") {
			parsed.Host = ""
		} else {
			parsed.Host = strings.ToLower(parsed.Host)
		}
		if parsed.Path == "" {
			parsed.Path = "/"
		}
		if strings.Contains(parsed.Path, "\x00") &&
			!validOpaqueCapabilityPathURI(parsed.String()) {
			return "", fmt.Errorf("capability root path %q contains a null byte", value)
		}
		if strings.Contains(parsed.Path, "\x00") {
			return parsed.String(), nil
		}
		escapedPath := parsed.EscapedPath()
		anchorDepth := 0
		if parsed.Host == "" && capabilityURIHasWindowsDrive(escapedPath) {
			anchorDepth = 1
		}
		escapedPath = normalizeCapabilityURIEscapedPath(escapedPath, anchorDepth)
		decodedPath, _ := url.PathUnescape(escapedPath)
		parsed.Path = decodedPath
		parsed.RawPath = escapedPath
		return parsed.String(), nil
	}

	if strings.HasPrefix(value, "/") {
		if strings.Contains(value, "\x00") {
			return opaqueCapabilityPathURI([]byte(value)), nil
		}
		segments := normalizeNativeCapabilitySegments(strings.Split(value[1:], "/"), 0, false)
		return capabilityFileURI("", segments), nil
	}
	return "", fmt.Errorf("capability root path %q is not absolute", value)
}

func splitWindowsCapabilityPath(value string) []string {
	return strings.Split(strings.ReplaceAll(value, `\`, "/"), "/")
}

func normalizeNativeCapabilitySegments(
	segments []string,
	anchorDepth int,
	forceAnchorTrailingSlash bool,
) []string {
	normalized := make([]string, 0, len(segments))
	depth := 0
	hasTrailingSeparator := false
	for _, segment := range segments {
		switch segment {
		case "":
			hasTrailingSeparator = true
		case ".":
			hasTrailingSeparator = false
		case "..":
			hasTrailingSeparator = false
			if depth > anchorDepth {
				normalized = normalized[:len(normalized)-1]
				depth--
			}
		default:
			normalized = append(normalized, segment)
			depth++
			hasTrailingSeparator = false
		}
	}
	if hasTrailingSeparator || (forceAnchorTrailingSlash && depth == anchorDepth) {
		normalized = append(normalized, "")
	}
	return normalized
}

func normalizeCapabilityURIEscapedPath(value string, anchorDepth int) string {
	segments := strings.Split(value[1:], "/")
	normalized := make([]string, 0, len(segments))
	for index, segment := range segments {
		last := index == len(segments)-1
		switch capabilityURIDotSegment(segment) {
		case 1:
			if last {
				normalized = append(normalized, "")
			}
		case 2:
			if len(normalized) > anchorDepth {
				normalized = normalized[:len(normalized)-1]
			}
			if last {
				normalized = append(normalized, "")
			}
		default:
			normalized = append(normalized, segment)
		}
	}
	return "/" + strings.Join(normalized, "/")
}

func capabilityURIHasWindowsDrive(value string) bool {
	first, _, _ := strings.Cut(value[1:], "/")
	decoded, err := url.PathUnescape(first)
	return err == nil && len(decoded) == 2 &&
		((decoded[0] >= 'A' && decoded[0] <= 'Z') ||
			(decoded[0] >= 'a' && decoded[0] <= 'z')) &&
		decoded[1] == ':'
}

func capabilityURIDotSegment(segment string) int {
	switch strings.ToLower(segment) {
	case ".", "%2e":
		return 1
	case "..", ".%2e", "%2e.", "%2e%2e":
		return 2
	default:
		return 0
	}
}

func capabilityFileURI(host string, segments []string) string {
	return (&url.URL{
		Scheme: "file",
		Host:   host,
		Path:   "/" + strings.Join(segments, "/"),
	}).String()
}

func canonicalCapabilityURIHost(host string) (string, bool) {
	parsed, err := url.Parse("file://" + host + "/")
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Port() != "" {
		return "", false
	}
	return strings.ToLower(parsed.Host), true
}

func isWindowsDriveAbsolutePath(value string) bool {
	if len(value) < 3 {
		return false
	}
	drive := value[0]
	return ((drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')) &&
		value[1] == ':' && (value[2] == '/' || value[2] == '\\')
}

func isWindowsNamespacePath(value string) bool {
	return len(value) >= 4 &&
		(value[0] == '/' || value[0] == '\\') &&
		(value[1] == '/' || value[1] == '\\') &&
		(value[2] == '.' || value[2] == '?') &&
		(value[3] == '/' || value[3] == '\\')
}

func opaqueCapabilityPathURI(value []byte) string {
	return "file:///%00/bad/path/" + base64.RawURLEncoding.EncodeToString(value)
}

func opaqueWindowsCapabilityPathURI(value string) string {
	codeUnits := utf16.Encode([]rune(value))
	bytes := make([]byte, 0, len(codeUnits)*2)
	for _, codeUnit := range codeUnits {
		bytes = append(bytes, byte(codeUnit), byte(codeUnit>>8))
	}
	return opaqueCapabilityPathURI(bytes)
}

func validOpaqueCapabilityPathURI(value string) bool {
	const prefix = "file:///%00/bad/path/"
	encoded, ok := strings.CutPrefix(value, prefix)
	if !ok || encoded == "" || strings.Contains(encoded, "/") {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	return err == nil && base64.RawURLEncoding.EncodeToString(decoded) == encoded
}

var (
	_ json.Marshaler   = ModeKind("")
	_ json.Unmarshaler = (*ModeKind)(nil)
	_ json.Marshaler   = MultiAgentMode{}
	_ json.Unmarshaler = (*MultiAgentMode)(nil)
	_ json.Unmarshaler = (*Settings)(nil)
	_ json.Unmarshaler = (*CollaborationMode)(nil)
	_ json.Unmarshaler = (*CollaborationModeMask)(nil)
	_ json.Marshaler   = CapabilityRootLocation{}
	_ json.Unmarshaler = (*CapabilityRootLocation)(nil)
	_ json.Unmarshaler = (*SelectedCapabilityRoot)(nil)
)

func collaborationCapabilitySchemas() map[string]Schema {
	nullableRef := func(name string) Schema {
		return Schema{"anyOf": []any{
			Schema{"$ref": "#/$defs/" + name},
			Schema{"type": "null"},
		}}
	}
	return map[string]Schema{
		"ModeKind": {
			"description": "Initial collaboration mode to use when the TUI starts.",
			"enum":        []any{"plan", "default"},
			"type":        "string",
		},
		"MultiAgentMode": {
			"description": "Controls the effective multi-agent delegation instructions for a turn. `custom` means the configured mode hint defines the policy instead of a built-in policy.",
			"oneOf": []any{
				Schema{"enum": []any{"explicitRequestOnly", "proactive"}, "type": "string"},
				Schema{
					"additionalProperties": false,
					"properties":           Schema{"custom": Schema{"type": "string"}},
					"required":             []string{"custom"},
					"title":                "CustomMultiAgentMode",
					"type":                 "object",
				},
			},
		},
		"Settings": {
			"description": "Settings for a collaboration mode.",
			"properties": Schema{
				"developer_instructions": Schema{"type": []any{"string", "null"}},
				"model":                  Schema{"type": "string"},
				"reasoning_effort":       nullableRef("ReasoningEffort"),
			},
			"required": []string{"model"},
			"type":     "object",
		},
		"CollaborationMode": {
			"description": "Collaboration mode for a Codex session.",
			"properties": Schema{
				"mode":     Schema{"$ref": "#/$defs/ModeKind"},
				"settings": Schema{"$ref": "#/$defs/Settings"},
			},
			"required": []string{"mode", "settings"},
			"type":     "object",
		},
		"CollaborationModeMask": {
			"description": "EXPERIMENTAL - collaboration mode preset metadata for clients.",
			"properties": Schema{
				"mode":  nullableRef("ModeKind"),
				"model": Schema{"type": []any{"string", "null"}},
				"name":  Schema{"type": "string"},
				"reasoning_effort": Schema{"anyOf": []any{
					nullableRef("ReasoningEffort"),
					Schema{"type": "null"},
				}},
			},
			"required": []string{"name"},
			"type":     "object",
		},
		"CapabilityRootLocation": {
			"description": "Location used to resolve a selected capability root.",
			"oneOf": []any{Schema{
				"description": "A path owned by an execution environment.",
				"properties": Schema{
					"environmentId": Schema{"type": "string"},
					"path": Schema{
						"description": "Absolute path for the root in the selected environment.",
						"type":        "string",
					},
					"type": Schema{
						"enum":  []any{"environment"},
						"title": "EnvironmentCapabilityRootLocationType",
						"type":  "string",
					},
				},
				"required": []string{"environmentId", "path", "type"},
				"title":    "EnvironmentCapabilityRootLocation",
				"type":     "object",
			}},
		},
		"SelectedCapabilityRoot": {
			"description": "A user-selected root that can expose one or more runtime capabilities.",
			"properties": Schema{
				"id": Schema{
					"description": "Stable identifier supplied by the capability selection platform.",
					"type":        "string",
				},
				"location": Schema{
					"allOf":       []any{Schema{"$ref": "#/$defs/CapabilityRootLocation"}},
					"description": "Where the selected root can be resolved.",
				},
			},
			"required": []string{"id", "location"},
			"type":     "object",
		},
	}
}
