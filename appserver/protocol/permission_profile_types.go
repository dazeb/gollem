package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

const (
	FileSystemAccessRead  FileSystemAccessMode = "read"
	FileSystemAccessWrite FileSystemAccessMode = "write"
	FileSystemAccessDeny  FileSystemAccessMode = "deny"

	PermissionGrantTurn    PermissionGrantScope = "turn"
	PermissionGrantSession PermissionGrantScope = "session"
)

// AbsolutePathBuf is an absolute, lexically normalized local path.
type AbsolutePathBuf string

// LegacyAppPathString preserves the public app-server's unrestricted UTF-8
// path string while callers migrate to explicit path conventions.
type LegacyAppPathString string

type FileSystemAccessMode string
type PermissionGrantScope string

// FileSystemSpecialPath is the closed public special-path union.
type FileSystemSpecialPath struct{ raw json.RawMessage }

// FileSystemPath is the closed public path/glob/special union.
type FileSystemPath struct{ raw json.RawMessage }

type FileSystemSandboxEntry struct {
	Path   FileSystemPath       `json:"path"`
	Access FileSystemAccessMode `json:"access"`
}

type AdditionalFileSystemPermissions struct {
	Read             []LegacyAppPathString    `json:"read" jsonschema:"description=This will be removed in favor of entries."`
	Write            []LegacyAppPathString    `json:"write" jsonschema:"description=This will be removed in favor of entries."`
	GlobScanMaxDepth *uint64                  `json:"globScanMaxDepth,omitempty" jsonschema:"nonnullable=true"`
	Entries          []FileSystemSandboxEntry `json:"entries,omitempty" jsonschema:"nonnullable=true"`
}

type AdditionalNetworkPermissions struct {
	Enabled *bool `json:"enabled"`
}

type RequestPermissionProfile struct {
	Network    *AdditionalNetworkPermissions    `json:"network"`
	FileSystem *AdditionalFileSystemPermissions `json:"fileSystem"`
}

type AdditionalPermissionProfile struct {
	Network    *AdditionalNetworkPermissions    `json:"network"`
	FileSystem *AdditionalFileSystemPermissions `json:"fileSystem"`
}

type GrantedPermissionProfile struct {
	Network    *AdditionalNetworkPermissions    `json:"network,omitempty" jsonschema:"nonnullable=true"`
	FileSystem *AdditionalFileSystemPermissions `json:"fileSystem,omitempty" jsonschema:"nonnullable=true"`
}

type ActivePermissionProfile struct {
	ID      string  `json:"id"`
	Extends *string `json:"extends"`
}

// PermissionsRequestApprovalParams is the public scoped permission request.
// Gollem's live Git/MCP approval request remains a separately named extension.
type PermissionsRequestApprovalParams struct {
	ThreadID      string                   `json:"threadId"`
	TurnID        string                   `json:"turnId"`
	ItemID        string                   `json:"itemId"`
	EnvironmentID *string                  `json:"environmentId"`
	StartedAtMS   int64                    `json:"startedAtMs"`
	CWD           AbsolutePathBuf          `json:"cwd"`
	Reason        *string                  `json:"reason"`
	Permissions   RequestPermissionProfile `json:"permissions"`
}

// PermissionsRequestApprovalResponse is exported for wire compatibility but
// is not bound until Gollem can enforce profile intersection and grant scope.
type PermissionsRequestApprovalResponse struct {
	Permissions      GrantedPermissionProfile `json:"permissions"`
	Scope            PermissionGrantScope     `json:"scope"`
	StrictAutoReview *bool                    `json:"strictAutoReview,omitempty" jsonschema:"nonnullable=true"`
}

func (p AbsolutePathBuf) MarshalJSON() ([]byte, error) {
	normalized, err := normalizeAbsolutePath(string(p))
	if err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func (p *AbsolutePathBuf) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal AbsolutePathBuf into nil receiver")
	}
	var value string
	if err := decodePermissionValue(data, &value); err != nil {
		return fmt.Errorf("decode AbsolutePathBuf: %w", err)
	}
	normalized, err := normalizeAbsolutePath(value)
	if err != nil {
		return err
	}
	*p = AbsolutePathBuf(normalized)
	return nil
}

func normalizeAbsolutePath(value string) (string, error) {
	if !filepath.IsAbs(value) {
		return "", fmt.Errorf("path %q is not absolute", value)
	}
	return filepath.Clean(value), nil
}

func (m FileSystemAccessMode) MarshalJSON() ([]byte, error) {
	if !validFileSystemAccessMode(m) {
		return nil, fmt.Errorf("unsupported filesystem access mode %q", m)
	}
	return json.Marshal(string(m))
}

func (m *FileSystemAccessMode) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("cannot unmarshal FileSystemAccessMode into nil receiver")
	}
	var value string
	if err := decodePermissionValue(data, &value); err != nil {
		return err
	}
	parsed := FileSystemAccessMode(value)
	if !validFileSystemAccessMode(parsed) {
		return fmt.Errorf("unsupported filesystem access mode %q", value)
	}
	*m = parsed
	return nil
}

func validFileSystemAccessMode(value FileSystemAccessMode) bool {
	return value == FileSystemAccessRead || value == FileSystemAccessWrite || value == FileSystemAccessDeny
}

func (s PermissionGrantScope) MarshalJSON() ([]byte, error) {
	if !validPermissionGrantScope(s) {
		return nil, fmt.Errorf("unsupported permission grant scope %q", s)
	}
	return json.Marshal(string(s))
}

func (s *PermissionGrantScope) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("cannot unmarshal PermissionGrantScope into nil receiver")
	}
	var value string
	if err := decodePermissionValue(data, &value); err != nil {
		return err
	}
	parsed := PermissionGrantScope(value)
	if !validPermissionGrantScope(parsed) {
		return fmt.Errorf("unsupported permission grant scope %q", value)
	}
	*s = parsed
	return nil
}

func validPermissionGrantScope(value PermissionGrantScope) bool {
	return value == PermissionGrantTurn || value == PermissionGrantSession
}

func (p FileSystemSpecialPath) MarshalJSON() ([]byte, error) {
	if len(p.raw) == 0 {
		return nil, errors.New("filesystem special path is empty")
	}
	return validateFileSystemSpecialPath(p.raw)
}

func (p *FileSystemSpecialPath) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal FileSystemSpecialPath into nil receiver")
	}
	canonical, err := validateFileSystemSpecialPath(data)
	if err != nil {
		return err
	}
	p.raw = canonical
	return nil
}

func (p FileSystemSpecialPath) Kind() string {
	return permissionUnionDiscriminant(p.raw, "kind")
}

func (p FileSystemPath) MarshalJSON() ([]byte, error) {
	if len(p.raw) == 0 {
		return nil, errors.New("filesystem path is empty")
	}
	return validateFileSystemPath(p.raw)
}

func (p *FileSystemPath) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal FileSystemPath into nil receiver")
	}
	canonical, err := validateFileSystemPath(data)
	if err != nil {
		return err
	}
	p.raw = canonical
	return nil
}

func (p FileSystemPath) Type() string {
	return permissionUnionDiscriminant(p.raw, "type")
}

func (e *FileSystemSandboxEntry) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("cannot unmarshal FileSystemSandboxEntry into nil receiver")
	}
	type wire FileSystemSandboxEntry
	var value wire
	if err := decodePermissionObject(data, &value, "path", "access"); err != nil {
		return err
	}
	*e = FileSystemSandboxEntry(value)
	return nil
}

func (p *AdditionalFileSystemPermissions) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal AdditionalFileSystemPermissions into nil receiver")
	}
	type wire AdditionalFileSystemPermissions
	var value wire
	if err := decodePermissionObject(data, &value); err != nil {
		return err
	}
	if value.GlobScanMaxDepth != nil && *value.GlobScanMaxDepth == 0 {
		return errors.New("globScanMaxDepth must be greater than zero")
	}
	*p = AdditionalFileSystemPermissions(value)
	return nil
}

func (p AdditionalFileSystemPermissions) MarshalJSON() ([]byte, error) {
	if p.GlobScanMaxDepth != nil && *p.GlobScanMaxDepth == 0 {
		return nil, errors.New("globScanMaxDepth must be greater than zero")
	}
	type wire AdditionalFileSystemPermissions
	return json.Marshal(wire(p))
}

func (p *AdditionalNetworkPermissions) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal AdditionalNetworkPermissions into nil receiver")
	}
	type wire AdditionalNetworkPermissions
	var value wire
	if err := decodePermissionObject(data, &value); err != nil {
		return err
	}
	*p = AdditionalNetworkPermissions(value)
	return nil
}

func (p *RequestPermissionProfile) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal RequestPermissionProfile into nil receiver")
	}
	type wire RequestPermissionProfile
	var value wire
	if err := decodePermissionObject(data, &value); err != nil {
		return err
	}
	*p = RequestPermissionProfile(value)
	return nil
}

func (p *AdditionalPermissionProfile) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal AdditionalPermissionProfile into nil receiver")
	}
	type wire AdditionalPermissionProfile
	var value wire
	if err := decodePermissionObject(data, &value); err != nil {
		return err
	}
	*p = AdditionalPermissionProfile(value)
	return nil
}

func (p *GrantedPermissionProfile) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal GrantedPermissionProfile into nil receiver")
	}
	type wire GrantedPermissionProfile
	var value wire
	if err := decodePermissionObject(data, &value); err != nil {
		return err
	}
	*p = GrantedPermissionProfile(value)
	return nil
}

func (p *ActivePermissionProfile) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal ActivePermissionProfile into nil receiver")
	}
	type wire ActivePermissionProfile
	var value wire
	if err := decodePermissionObject(data, &value, "id"); err != nil {
		return err
	}
	*p = ActivePermissionProfile(value)
	return nil
}

func (p *PermissionsRequestApprovalParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("cannot unmarshal PermissionsRequestApprovalParams into nil receiver")
	}
	type wire PermissionsRequestApprovalParams
	var value wire
	if err := decodePermissionObject(data, &value,
		"threadId", "turnId", "itemId", "startedAtMs", "cwd", "permissions",
	); err != nil {
		return err
	}
	*p = PermissionsRequestApprovalParams(value)
	return nil
}

func (r *PermissionsRequestApprovalResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("cannot unmarshal PermissionsRequestApprovalResponse into nil receiver")
	}
	type wire PermissionsRequestApprovalResponse
	value := wire{Scope: PermissionGrantTurn}
	if err := decodePermissionObject(data, &value, "permissions"); err != nil {
		return err
	}
	if !validPermissionGrantScope(value.Scope) {
		return fmt.Errorf("unsupported permission grant scope %q", value.Scope)
	}
	*r = PermissionsRequestApprovalResponse(value)
	return nil
}

func (r PermissionsRequestApprovalResponse) MarshalJSON() ([]byte, error) {
	if r.Scope == "" {
		r.Scope = PermissionGrantTurn
	}
	if !validPermissionGrantScope(r.Scope) {
		return nil, fmt.Errorf("unsupported permission grant scope %q", r.Scope)
	}
	type wire PermissionsRequestApprovalResponse
	return json.Marshal(wire(r))
}

func validateFileSystemSpecialPath(data []byte) ([]byte, error) {
	object, err := decodePermissionRawObject(data)
	if err != nil {
		return nil, fmt.Errorf("decode filesystem special path: %w", err)
	}
	kind, err := requiredPermissionString(object, "kind")
	if err != nil {
		return nil, err
	}
	if kind == "current_working_directory" {
		kind = "project_roots"
		object["kind"], _ = json.Marshal(kind)
	}
	switch kind {
	case "root", "minimal", "tmpdir", "slash_tmp":
		if err := requirePermissionFields(object, []string{"kind"}, "kind"); err != nil {
			return nil, err
		}
	case "project_roots":
		if _, ok := object["subpath"]; !ok {
			object["subpath"] = json.RawMessage("null")
		}
		if err := requirePermissionFields(object, []string{"kind", "subpath"}, "kind", "subpath"); err != nil {
			return nil, err
		}
		if err := optionalPermissionNullableString(object, "subpath"); err != nil {
			return nil, err
		}
	case "unknown":
		if _, ok := object["subpath"]; !ok {
			object["subpath"] = json.RawMessage("null")
		}
		if err := requirePermissionFields(object, []string{"kind", "path", "subpath"}, "kind", "path", "subpath"); err != nil {
			return nil, err
		}
		if _, err := requiredPermissionString(object, "path"); err != nil {
			return nil, err
		}
		if err := optionalPermissionNullableString(object, "subpath"); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported filesystem special path kind %q", kind)
	}
	return json.Marshal(object)
}

func validateFileSystemPath(data []byte) ([]byte, error) {
	object, err := decodePermissionRawObject(data)
	if err != nil {
		return nil, fmt.Errorf("decode filesystem path: %w", err)
	}
	typeName, err := requiredPermissionString(object, "type")
	if err != nil {
		return nil, err
	}
	switch typeName {
	case "path":
		if err := requirePermissionFields(object, []string{"type", "path"}, "type", "path"); err != nil {
			return nil, err
		}
		if _, err := requiredPermissionString(object, "path"); err != nil {
			return nil, err
		}
	case "glob_pattern":
		if err := requirePermissionFields(object, []string{"type", "pattern"}, "type", "pattern"); err != nil {
			return nil, err
		}
		if _, err := requiredPermissionString(object, "pattern"); err != nil {
			return nil, err
		}
	case "special":
		if err := requirePermissionFields(object, []string{"type", "value"}, "type", "value"); err != nil {
			return nil, err
		}
		raw, ok := object["value"]
		if !ok {
			return nil, errors.New("filesystem path requires value")
		}
		var special FileSystemSpecialPath
		if err := json.Unmarshal(raw, &special); err != nil {
			return nil, fmt.Errorf("decode filesystem special path value: %w", err)
		}
		object["value"], err = json.Marshal(special)
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported filesystem path type %q", typeName)
	}
	return json.Marshal(object)
}

func decodePermissionValue(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("permission value must contain one JSON value")
		}
		return err
	}
	return nil
}

func decodePermissionObject(data []byte, target any, required ...string) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("permission object must contain one JSON value")
		}
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("permission value must be an object")
	}
	for _, name := range required {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("permission object requires %s", name)
		}
	}
	return nil
}

func decodePermissionRawObject(data []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := decodePermissionValue(data, &object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, errors.New("permission value must be an object")
	}
	return object, nil
}

func requirePermissionFields(object map[string]json.RawMessage, allowed []string, required ...string) error {
	allowedSet := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = true
	}
	for name := range object {
		if !allowedSet[name] {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	for _, name := range required {
		if _, ok := object[name]; !ok {
			return fmt.Errorf("permission object requires %s", name)
		}
	}
	return nil
}

func requiredPermissionString(object map[string]json.RawMessage, name string) (string, error) {
	raw, ok := object[name]
	if !ok {
		return "", fmt.Errorf("permission object requires %s", name)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("permission field %s must be a string", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("permission field %s must be a string", name)
	}
	return value, nil
}

func optionalPermissionNullableString(object map[string]json.RawMessage, name string) error {
	raw, ok := object[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("permission field %s must be a string or null", name)
	}
	return nil
}

func permissionUnionDiscriminant(data json.RawMessage, name string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(data, &object) != nil {
		return ""
	}
	value, _ := requiredPermissionString(object, name)
	return value
}
