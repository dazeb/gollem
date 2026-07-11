package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFilesystemSchemasAndBindingsAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"FsReadFileParams", "FsReadFileResponse",
		"FsWriteFileParams", "FsWriteFileResponse",
		"FsCreateDirectoryParams", "FsCreateDirectoryResponse",
		"FsGetMetadataParams", "FsGetMetadataResponse",
		"FsReadDirectoryParams", "FsReadDirectoryEntry", "FsReadDirectoryResponse",
		"FsRemoveParams", "FsRemoveResponse",
		"FsCopyParams", "FsCopyResponse",
		"FsWatchParams", "FsWatchResponse",
		"FsUnwatchParams", "FsUnwatchResponse",
		"FsChangedNotification", "FileChangedNotification",
	} {
		if _, ok := defs[name]; !ok {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	for _, name := range []string{
		"FsReadFileParams", "FsWriteFileParams", "FsCreateDirectoryParams",
		"FsGetMetadataParams", "FsReadDirectoryParams", "FsRemoveParams",
	} {
		assertFilesystemPropertyRef(t, defs, name, "path", "AbsolutePathBuf")
	}
	assertFilesystemPropertyRef(t, defs, "FsCopyParams", "sourcePath", "AbsolutePathBuf")
	assertFilesystemPropertyRef(t, defs, "FsCopyParams", "destinationPath", "AbsolutePathBuf")
	assertFilesystemPropertyRef(t, defs, "FsWatchParams", "path", "AbsolutePathBuf")
	assertFilesystemPropertyRef(t, defs, "FsWatchResponse", "path", "AbsolutePathBuf")
	assertFilesystemPropertyRef(t, defs, "FsChangedNotification", "changedPaths", "AbsolutePathBuf")

	for name, fields := range map[string][]string{
		"FsReadFileParams":        {"path"},
		"FsReadFileResponse":      {"dataBase64"},
		"FsWriteFileParams":       {"path", "dataBase64"},
		"FsCreateDirectoryParams": {"path"},
		"FsGetMetadataParams":     {"path"},
		"FsGetMetadataResponse":   {"isDirectory", "isFile", "isSymlink", "createdAtMs", "modifiedAtMs"},
		"FsReadDirectoryParams":   {"path"},
		"FsReadDirectoryEntry":    {"fileName", "isDirectory", "isFile"},
		"FsReadDirectoryResponse": {"entries"},
		"FsRemoveParams":          {"path"},
		"FsCopyParams":            {"sourcePath", "destinationPath"},
		"FsWatchParams":           {"watchId", "path"},
		"FsWatchResponse":         {"path"},
		"FsUnwatchParams":         {"watchId"},
		"FsChangedNotification":   {"watchId", "changedPaths"},
		"FileChangedNotification": {"operation", "at"},
	} {
		assertSchemaRequired(t, defs[name].(Schema), fields...)
	}

	for _, tc := range []struct {
		definition string
		property   string
		nullable   bool
	}{
		{"FsCreateDirectoryParams", "recursive", true},
		{"FsRemoveParams", "recursive", true},
		{"FsRemoveParams", "force", true},
		{"FsCopyParams", "recursive", false},
	} {
		property := defs[tc.definition].(Schema)["properties"].(Schema)[tc.property].(Schema)
		_, nullable := property["anyOf"]
		if nullable != tc.nullable {
			t.Errorf("%s.%s nullable = %v, want %v: %#v", tc.definition, tc.property, nullable, tc.nullable, property)
		}
	}

	wantBindings := map[string]WireTypeBinding{
		"fs/readFile":        {Method: "fs/readFile", Surface: SurfaceClientRequest, Params: []string{"FsReadFileParams"}, Result: []string{"FsReadFileResponse"}},
		"fs/writeFile":       {Method: "fs/writeFile", Surface: SurfaceClientRequest, Params: []string{"FsWriteFileParams"}, Result: []string{"FsWriteFileResponse"}},
		"fs/createDirectory": {Method: "fs/createDirectory", Surface: SurfaceClientRequest, Params: []string{"FsCreateDirectoryParams"}, Result: []string{"FsCreateDirectoryResponse"}},
		"fs/getMetadata":     {Method: "fs/getMetadata", Surface: SurfaceClientRequest, Params: []string{"FsGetMetadataParams"}, Result: []string{"FsGetMetadataResponse"}},
		"fs/readDirectory":   {Method: "fs/readDirectory", Surface: SurfaceClientRequest, Params: []string{"FsReadDirectoryParams"}, Result: []string{"FsReadDirectoryResponse"}},
		"fs/remove":          {Method: "fs/remove", Surface: SurfaceClientRequest, Params: []string{"FsRemoveParams"}, Result: []string{"FsRemoveResponse"}},
		"fs/copy":            {Method: "fs/copy", Surface: SurfaceClientRequest, Params: []string{"FsCopyParams"}, Result: []string{"FsCopyResponse"}},
		"fs/watch":           {Method: "fs/watch", Surface: SurfaceClientRequest, Params: []string{"FsWatchParams"}, Result: []string{"FsWatchResponse"}},
		"fs/unwatch":         {Method: "fs/unwatch", Surface: SurfaceClientRequest, Params: []string{"FsUnwatchParams"}, Result: []string{"FsUnwatchResponse"}},
		"fs/changed":         {Method: "fs/changed", Surface: SurfaceServerNotification, Params: []string{"FsChangedNotification", "FileChangedNotification"}},
	}
	for method, want := range wantBindings {
		got, ok := filesystemBinding(WireTypeBindings(), method)
		if !ok {
			t.Errorf("missing %s binding", method)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s binding = %+v, want %+v", method, got, want)
		}
	}
}

func TestFilesystemValuesPreserveExactAndExtensionWire(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  any
		want string
	}{
		{"read response", FsReadFileResponse{DataBase64: "AAEC"}, `{"dataBase64":"AAEC"}`},
		{"empty directory", FsReadDirectoryResponse{}, `{"entries":[]}`},
		{"empty changed paths", FsChangedNotification{WatchID: "watch"}, `{"watchId":"watch","changedPaths":[]}`},
		{"empty write response", FsWriteFileResponse{}, `{}`},
		{"empty create response", FsCreateDirectoryResponse{}, `{}`},
		{"empty remove response", FsRemoveResponse{}, `{}`},
		{"empty copy response", FsCopyResponse{}, `{}`},
		{"empty unwatch response", FsUnwatchResponse{}, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.got)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("wire = %s, want %s", got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		wire string
		out  any
	}{
		{"read relative", `{"path":"relative.txt"}`, new(FsReadFileParams)},
		{"write relative", `{"path":"relative.txt","dataBase64":"YQ=="}`, new(FsWriteFileParams)},
		{"copy relative source", `{"sourcePath":"relative","destinationPath":"/workspace/dst"}`, new(FsCopyParams)},
		{"watch relative", `{"watchId":"watch","path":"relative"}`, new(FsWatchParams)},
		{"null directory entries", `{"entries":null}`, new(FsReadDirectoryResponse)},
		{"null changed paths", `{"watchId":"watch","changedPaths":null}`, new(FsChangedNotification)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(tc.wire), tc.out); err == nil {
				t.Fatalf("Unmarshal(%s) succeeded", tc.wire)
			}
		})
	}

	var changed FileChangedNotification
	if err := json.Unmarshal([]byte(`{"path":"legacy.txt","operation":"writeFile","at":"2026-07-11T12:00:00Z"}`), &changed); err != nil {
		t.Fatalf("Unmarshal legacy changed notification: %v", err)
	}
	if changed.Path == nil || *changed.Path != "legacy.txt" || changed.Operation != "writeFile" {
		t.Fatalf("legacy changed notification = %+v", changed)
	}
}

func TestFilesystemWireFixtureDecodesCanonicalAndExtensionTypes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "filesystem_wire_v1.json"))
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var fixture typeScriptFixtureDocument
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		t.Fatalf("fixture has trailing JSON: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion)
	}
	if len(fixture.Cases) != 20 {
		t.Fatalf("fixture cases = %d, want 20", len(fixture.Cases))
	}

	seen := make(map[string]bool, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		var envelope struct {
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(fixtureCase.Message, &envelope); err != nil {
			t.Fatalf("decode %s envelope: %v", fixtureCase.Name, err)
		}
		typeName := fixtureCase.ParamsType
		wire := envelope.Params
		if typeName == "" {
			typeName = fixtureCase.ResultType
			wire = envelope.Result
		}
		value := filesystemFixtureValue(typeName)
		if value == nil {
			t.Fatalf("%s has unsupported fixture type %q", fixtureCase.Name, typeName)
		}
		if err := json.Unmarshal(wire, value); err != nil {
			t.Fatalf("decode %s as %s: %v", fixtureCase.Name, typeName, err)
		}
		seen[typeName] = true
	}
	for _, name := range []string{
		"FsReadFileParams", "FsReadFileResponse", "FsWriteFileParams", "FsWriteFileResponse",
		"FsCreateDirectoryParams", "FsCreateDirectoryResponse", "FsGetMetadataParams", "FsGetMetadataResponse",
		"FsReadDirectoryParams", "FsReadDirectoryResponse", "FsRemoveParams", "FsRemoveResponse",
		"FsCopyParams", "FsCopyResponse", "FsWatchParams", "FsWatchResponse", "FsUnwatchParams",
		"FsUnwatchResponse", "FsChangedNotification", "FileChangedNotification",
	} {
		if !seen[name] {
			t.Errorf("fixture missing %s", name)
		}
	}
}

func filesystemFixtureValue(name string) any {
	switch name {
	case "FsReadFileParams":
		return new(FsReadFileParams)
	case "FsReadFileResponse":
		return new(FsReadFileResponse)
	case "FsWriteFileParams":
		return new(FsWriteFileParams)
	case "FsWriteFileResponse":
		return new(FsWriteFileResponse)
	case "FsCreateDirectoryParams":
		return new(FsCreateDirectoryParams)
	case "FsCreateDirectoryResponse":
		return new(FsCreateDirectoryResponse)
	case "FsGetMetadataParams":
		return new(FsGetMetadataParams)
	case "FsGetMetadataResponse":
		return new(FsGetMetadataResponse)
	case "FsReadDirectoryParams":
		return new(FsReadDirectoryParams)
	case "FsReadDirectoryResponse":
		return new(FsReadDirectoryResponse)
	case "FsRemoveParams":
		return new(FsRemoveParams)
	case "FsRemoveResponse":
		return new(FsRemoveResponse)
	case "FsCopyParams":
		return new(FsCopyParams)
	case "FsCopyResponse":
		return new(FsCopyResponse)
	case "FsWatchParams":
		return new(FsWatchParams)
	case "FsWatchResponse":
		return new(FsWatchResponse)
	case "FsUnwatchParams":
		return new(FsUnwatchParams)
	case "FsUnwatchResponse":
		return new(FsUnwatchResponse)
	case "FsChangedNotification":
		return new(FsChangedNotification)
	case "FileChangedNotification":
		return new(FileChangedNotification)
	default:
		return nil
	}
}

func assertFilesystemPropertyRef(t *testing.T, defs Schema, definition, property, want string) {
	t.Helper()
	properties := defs[definition].(Schema)["properties"].(Schema)
	field := properties[property].(Schema)
	if property == "changedPaths" {
		field = field["items"].(Schema)
	}
	if field["$ref"] != "#/$defs/"+want {
		t.Fatalf("%s.%s = %#v, want %s ref", definition, property, properties[property], want)
	}
}

func filesystemBinding(bindings []WireTypeBinding, method string) (WireTypeBinding, bool) {
	for _, binding := range bindings {
		if binding.Method == method {
			return binding, true
		}
	}
	return WireTypeBinding{}, false
}
