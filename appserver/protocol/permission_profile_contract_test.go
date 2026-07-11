package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestPermissionProfileSchemasAreExportedButLiveApprovalRemainsUnbound(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"AbsolutePathBuf",
		"ActivePermissionProfile",
		"AdditionalFileSystemPermissions",
		"AdditionalNetworkPermissions",
		"AdditionalPermissionProfile",
		"FileSystemAccessMode",
		"FileSystemPath",
		"FileSystemSandboxEntry",
		"FileSystemSpecialPath",
		"GrantedPermissionProfile",
		"LegacyAppPathString",
		"PermissionGrantScope",
		"PermissionsRequestApprovalParams",
		"PermissionsRequestApprovalResponse",
		"RequestPermissionProfile",
	} {
		if _, ok := defs[name]; !ok {
			t.Errorf("schema missing %s", name)
		}
	}

	var params PermissionsRequestApprovalParams
	var response PermissionsRequestApprovalResponse
	if reflect.TypeOf(params).Name() != "PermissionsRequestApprovalParams" ||
		reflect.TypeOf(response).Name() != "PermissionsRequestApprovalResponse" {
		t.Fatal("public permission request/response types are not named")
	}

	for _, binding := range WireTypeBindings() {
		if binding.Method != "item/permissions/requestApproval" {
			continue
		}
		if !reflect.DeepEqual(binding.Params, []string{"PermissionsApprovalRequestParams"}) {
			t.Fatalf("live permission params binding = %v", binding.Params)
		}
		if len(binding.Result) != 0 {
			t.Fatalf("incompatible permission result unexpectedly bound: %v", binding.Result)
		}
		return
	}
	t.Fatal("missing live permission approval binding")
}

func TestPermissionProfileSchemasMatchPublicWire(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	assertStringEnum(t, defs["FileSystemAccessMode"], "read", "write", "deny")
	assertStringEnum(t, defs["PermissionGrantScope"], "turn", "session")
	assertPermissionUnionVariants(t, defs["FileSystemSpecialPath"].(Schema), 6)
	assertPermissionUnionVariants(t, defs["FileSystemPath"].(Schema), 3)
	assertSchemaRequired(t, defs["FileSystemSandboxEntry"].(Schema), "path")
	assertSchemaRequired(t, defs["FileSystemSandboxEntry"].(Schema), "access")
	assertSchemaRequired(t, defs["ActivePermissionProfile"].(Schema), "extends")
	assertSchemaRequired(t, defs["AdditionalFileSystemPermissions"].(Schema), "read")
	assertSchemaRequired(t, defs["AdditionalFileSystemPermissions"].(Schema), "write")
	assertSchemaRequired(t, defs["AdditionalNetworkPermissions"].(Schema), "enabled")
	assertSchemaRequired(t, defs["AdditionalPermissionProfile"].(Schema), "network")
	assertSchemaRequired(t, defs["AdditionalPermissionProfile"].(Schema), "fileSystem")
	assertSchemaRequired(t, defs["RequestPermissionProfile"].(Schema), "network")
	assertSchemaRequired(t, defs["RequestPermissionProfile"].(Schema), "fileSystem")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "threadId")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "turnId")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "itemId")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "startedAtMs")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "cwd")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "permissions")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "environmentId")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalParams"].(Schema), "reason")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalResponse"].(Schema), "permissions")
	assertSchemaRequired(t, defs["PermissionsRequestApprovalResponse"].(Schema), "scope")

	responseProperties := defs["PermissionsRequestApprovalResponse"].(Schema)["properties"].(Schema)
	scope := responseProperties["scope"].(Schema)
	if scope["default"] != "turn" {
		t.Fatalf("permission response scope default = %#v", scope["default"])
	}
	filesystemProperties := defs["AdditionalFileSystemPermissions"].(Schema)["properties"].(Schema)
	depth := filesystemProperties["globScanMaxDepth"].(Schema)
	if depth["minimum"] != 1 {
		t.Fatalf("globScanMaxDepth schema = %#v", depth)
	}
}

func TestPermissionProfilePublicValuesAreStrictAndRoundTrip(t *testing.T) {
	requestJSON := `{
		"threadId":"thread-1",
		"turnId":"turn-1",
		"itemId":"item-1",
		"environmentId":"remote",
		"startedAtMs":1783715400000,
		"cwd":"/workspace/../workspace/project",
		"reason":"Grant requested workspace and network access",
		"permissions":{
			"network":{"enabled":true},
			"fileSystem":{
				"read":["/workspace/project"],
				"write":null,
				"globScanMaxDepth":2,
				"entries":[
					{"path":{"type":"path","path":"/workspace/project"},"access":"read"},
					{"path":{"type":"glob_pattern","pattern":"**/*.env"},"access":"deny"},
					{"path":{"type":"special","value":{"kind":"project_roots","subpath":"src"}},"access":"write"}
				]
			}
		}
	}`
	var request PermissionsRequestApprovalParams
	if err := json.Unmarshal([]byte(requestJSON), &request); err != nil {
		t.Fatalf("decode permission request: %v", err)
	}
	if request.CWD != AbsolutePathBuf("/workspace/project") || request.EnvironmentID == nil || *request.EnvironmentID != "remote" {
		t.Fatalf("permission request = %+v", request)
	}
	entries := request.Permissions.FileSystem.Entries
	if len(entries) != 3 || entries[0].Path.Type() != "path" || entries[1].Path.Type() != "glob_pattern" ||
		entries[2].Path.Type() != "special" || entries[2].Access != FileSystemAccessWrite {
		t.Fatalf("permission entries = %+v", entries)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal permission request: %v", err)
	}
	var roundTrip PermissionsRequestApprovalParams
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("round-trip permission request: %v", err)
	}

	var response PermissionsRequestApprovalResponse
	if err := json.Unmarshal([]byte(`{"permissions":{"network":{"enabled":false}},"strictAutoReview":true}`), &response); err != nil {
		t.Fatalf("decode permission response: %v", err)
	}
	if response.Scope != PermissionGrantTurn || response.StrictAutoReview == nil || !*response.StrictAutoReview {
		t.Fatalf("permission response = %+v", response)
	}
	encoded, err = json.Marshal(PermissionsRequestApprovalResponse{
		Permissions: GrantedPermissionProfile{},
	})
	if err != nil {
		t.Fatalf("marshal default permission response: %v", err)
	}
	if string(encoded) != `{"permissions":{},"scope":"turn"}` {
		t.Fatalf("default permission response = %s", encoded)
	}

	var additional AdditionalPermissionProfile
	if err := json.Unmarshal([]byte(`{"network":{"enabled":null},"fileSystem":null}`), &additional); err != nil {
		t.Fatalf("decode additional permission profile: %v", err)
	}
	var active ActivePermissionProfile
	if err := json.Unmarshal([]byte(`{"id":":workspace","extends":null}`), &active); err != nil {
		t.Fatalf("decode active permission profile: %v", err)
	}
	if active.ID != ":workspace" {
		t.Fatalf("active permission profile = %+v", active)
	}
	canonical, err := json.Marshal(PermissionsRequestApprovalParams{
		ThreadID:    "thread-1",
		TurnID:      "turn-1",
		ItemID:      "item-1",
		StartedAtMS: 1,
		CWD:         AbsolutePathBuf("/workspace"),
	})
	if err != nil {
		t.Fatalf("marshal canonical nullable permission request: %v", err)
	}
	var canonicalObject map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &canonicalObject); err != nil {
		t.Fatalf("decode canonical permission request: %v", err)
	}
	for _, name := range []string{"environmentId", "reason"} {
		if string(canonicalObject[name]) != "null" {
			t.Errorf("canonical permission request %s = %s", name, canonicalObject[name])
		}
	}
	var canonicalProfile map[string]json.RawMessage
	if err := json.Unmarshal(canonicalObject["permissions"], &canonicalProfile); err != nil {
		t.Fatalf("decode canonical request profile: %v", err)
	}
	for _, name := range []string{"network", "fileSystem"} {
		if string(canonicalProfile[name]) != "null" {
			t.Errorf("canonical request profile %s = %s", name, canonicalProfile[name])
		}
	}
}

func TestPermissionPathUnionsAcceptEveryPublicVariant(t *testing.T) {
	specials := []struct {
		input string
		kind  string
	}{
		{`{"kind":"root"}`, "root"},
		{`{"kind":"minimal"}`, "minimal"},
		{`{"kind":"project_roots","subpath":null}`, "project_roots"},
		{`{"kind":"tmpdir"}`, "tmpdir"},
		{`{"kind":"slash_tmp"}`, "slash_tmp"},
		{`{"kind":"unknown","path":"/foreign","subpath":"nested"}`, "unknown"},
		{`{"kind":"current_working_directory"}`, "project_roots"},
	}
	for _, tc := range specials {
		var special FileSystemSpecialPath
		if err := json.Unmarshal([]byte(tc.input), &special); err != nil {
			t.Errorf("Unmarshal(%s): %v", tc.input, err)
			continue
		}
		if special.Kind() != tc.kind {
			t.Errorf("Kind(%s) = %q, want %q", tc.input, special.Kind(), tc.kind)
		}
		encoded, err := json.Marshal(special)
		if err != nil {
			t.Errorf("Marshal(%s): %v", tc.input, err)
			continue
		}
		var roundTrip FileSystemSpecialPath
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Errorf("round-trip %s: %v", encoded, err)
		}
	}
	var defaultedSubpath FileSystemSpecialPath
	if err := json.Unmarshal([]byte(`{"kind":"project_roots"}`), &defaultedSubpath); err != nil {
		t.Fatalf("decode omitted project subpath: %v", err)
	}
	canonicalSpecial, err := json.Marshal(defaultedSubpath)
	if err != nil {
		t.Fatalf("marshal defaulted project subpath: %v", err)
	}
	if string(canonicalSpecial) != `{"kind":"project_roots","subpath":null}` {
		t.Fatalf("canonical project roots = %s", canonicalSpecial)
	}

	paths := []struct {
		input    string
		pathType string
	}{
		{`{"type":"path","path":"relative-is-valid-at-the-legacy-boundary"}`, "path"},
		{`{"type":"glob_pattern","pattern":"**/*.go"}`, "glob_pattern"},
		{`{"type":"special","value":{"kind":"tmpdir"}}`, "special"},
	}
	for _, tc := range paths {
		var path FileSystemPath
		if err := json.Unmarshal([]byte(tc.input), &path); err != nil {
			t.Errorf("Unmarshal(%s): %v", tc.input, err)
			continue
		}
		if path.Type() != tc.pathType {
			t.Errorf("Type(%s) = %q, want %q", tc.input, path.Type(), tc.pathType)
		}
	}
}

func TestPermissionProfilePublicValuesRejectMalformedInput(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		target func() any
	}{
		{"relative cwd", `{"threadId":"t","turnId":"u","itemId":"i","startedAtMs":1,"cwd":"relative","permissions":{}}`, func() any { return new(PermissionsRequestApprovalParams) }},
		{"missing cwd", `{"threadId":"t","turnId":"u","itemId":"i","startedAtMs":1,"permissions":{}}`, func() any { return new(PermissionsRequestApprovalParams) }},
		{"request unknown", `{"threadId":"t","turnId":"u","itemId":"i","startedAtMs":1,"cwd":"/workspace","permissions":{},"extra":true}`, func() any { return new(PermissionsRequestApprovalParams) }},
		{"profile unknown", `{"network":null,"extra":true}`, func() any { return new(RequestPermissionProfile) }},
		{"zero depth", `{"globScanMaxDepth":0}`, func() any { return new(AdditionalFileSystemPermissions) }},
		{"missing entry path", `{"access":"read"}`, func() any { return new(FileSystemSandboxEntry) }},
		{"invalid access", `"execute"`, func() any { return new(FileSystemAccessMode) }},
		{"crossed path", `{"type":"path","pattern":"*.go"}`, func() any { return new(FileSystemPath) }},
		{"null path", `{"type":"path","path":null}`, func() any { return new(FileSystemPath) }},
		{"path unknown", `{"type":"glob_pattern","pattern":"*.go","extra":true}`, func() any { return new(FileSystemPath) }},
		{"special unknown", `{"kind":"root","path":"/"}`, func() any { return new(FileSystemSpecialPath) }},
		{"null unknown special path", `{"kind":"unknown","path":null}`, func() any { return new(FileSystemSpecialPath) }},
		{"invalid scope", `{"permissions":{},"scope":"workspace"}`, func() any { return new(PermissionsRequestApprovalResponse) }},
		{"missing permissions", `{"scope":"turn"}`, func() any { return new(PermissionsRequestApprovalResponse) }},
		{"response unknown", `{"permissions":{},"extra":true}`, func() any { return new(PermissionsRequestApprovalResponse) }},
		{"null object", `null`, func() any { return new(GrantedPermissionProfile) }},
		{"second value", `{"permissions":{}} {}`, func() any { return new(PermissionsRequestApprovalResponse) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.input), tc.target()); err == nil {
				t.Fatalf("Unmarshal(%s) succeeded", tc.input)
			}
		})
	}

	zero := uint64(0)
	if _, err := json.Marshal(AdditionalFileSystemPermissions{GlobScanMaxDepth: &zero}); err == nil {
		t.Fatal("zero glob scan depth marshal succeeded")
	}
	if _, err := json.Marshal(FileSystemPath{}); err == nil {
		t.Fatal("zero filesystem path marshal succeeded")
	}
	if _, err := json.Marshal(PermissionGrantScope("workspace")); err == nil {
		t.Fatal("invalid permission scope marshal succeeded")
	}
}

func TestPermissionProfileMarshalAndNilReceiverGuards(t *testing.T) {
	for name, test := range map[string]func() error{
		"absolute path": func() error {
			var target *AbsolutePathBuf
			return target.UnmarshalJSON([]byte(`"/workspace"`))
		},
		"access mode": func() error {
			var target *FileSystemAccessMode
			return target.UnmarshalJSON([]byte(`"read"`))
		},
		"grant scope": func() error {
			var target *PermissionGrantScope
			return target.UnmarshalJSON([]byte(`"turn"`))
		},
		"special path": func() error {
			var target *FileSystemSpecialPath
			return target.UnmarshalJSON([]byte(`{"kind":"root"}`))
		},
		"filesystem path": func() error {
			var target *FileSystemPath
			return target.UnmarshalJSON([]byte(`{"type":"path","path":"/workspace"}`))
		},
		"sandbox entry": func() error {
			var target *FileSystemSandboxEntry
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"filesystem permissions": func() error {
			var target *AdditionalFileSystemPermissions
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"network permissions": func() error {
			var target *AdditionalNetworkPermissions
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"request profile": func() error {
			var target *RequestPermissionProfile
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"additional profile": func() error {
			var target *AdditionalPermissionProfile
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"granted profile": func() error {
			var target *GrantedPermissionProfile
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"active profile": func() error {
			var target *ActivePermissionProfile
			return target.UnmarshalJSON([]byte(`{"id":"x"}`))
		},
		"approval params": func() error {
			var target *PermissionsRequestApprovalParams
			return target.UnmarshalJSON([]byte(`{}`))
		},
		"approval response": func() error {
			var target *PermissionsRequestApprovalResponse
			return target.UnmarshalJSON([]byte(`{"permissions":{}}`))
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := test(); err == nil {
				t.Fatal("nil receiver succeeded")
			}
		})
	}

	for name, value := range map[string]any{
		"absolute path": AbsolutePathBuf("/workspace/../workspace/project"),
		"access mode":   FileSystemAccessRead,
		"grant scope":   PermissionGrantSession,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := json.Marshal(value); err != nil {
				t.Fatalf("Marshal: %v", err)
			}
		})
	}
	if _, err := json.Marshal(AbsolutePathBuf("relative")); err == nil {
		t.Fatal("relative absolute path marshal succeeded")
	}
	if _, err := json.Marshal(FileSystemAccessMode("execute")); err == nil {
		t.Fatal("invalid access mode marshal succeeded")
	}
	if _, err := json.Marshal(FileSystemSpecialPath{}); err == nil {
		t.Fatal("empty special path marshal succeeded")
	}
}

func TestPermissionProfileWireV1Fixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "permission_profile_wire_v1.json"))
	if err != nil {
		t.Fatalf("read permission profile fixture: %v", err)
	}
	var fixture struct {
		ProtocolVersion string          `json:"protocolVersion"`
		SchemaVersion   string          `json:"schemaVersion"`
		Request         json.RawMessage `json:"request"`
		Response        json.RawMessage `json:"response"`
	}
	if err := decodePermissionObject(data, &fixture, "protocolVersion", "schemaVersion", "request", "response"); err != nil {
		t.Fatalf("decode permission profile fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion)
	}
	var request PermissionsRequestApprovalParams
	if err := json.Unmarshal(fixture.Request, &request); err != nil {
		t.Fatalf("decode fixture request: %v", err)
	}
	var response PermissionsRequestApprovalResponse
	if err := json.Unmarshal(fixture.Response, &response); err != nil {
		t.Fatalf("decode fixture response: %v", err)
	}
	if request.CWD == "" || response.Scope != PermissionGrantSession {
		t.Fatalf("fixture request/response = %+v / %+v", request, response)
	}
}

func assertPermissionUnionVariants(t *testing.T, schema Schema, want int) {
	t.Helper()
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) != want {
		t.Fatalf("oneOf = %#v, want %d variants", schema["oneOf"], want)
	}
	for _, raw := range variants {
		variant, ok := raw.(Schema)
		if !ok || variant["additionalProperties"] != false {
			t.Fatalf("union variant is not closed: %#v", raw)
		}
		required := schemaRequiredNames(variant)
		if !slices.Contains(required, map[bool]string{true: "type", false: "kind"}[variant["properties"].(Schema)["type"] != nil]) {
			t.Fatalf("union variant required = %v", required)
		}
	}
}
