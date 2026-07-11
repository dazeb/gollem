package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFileChangeItemContractsMatchPublicWire(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{"FileUpdateChange", "PatchApplyStatus", "PatchChangeKind"} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("missing %s schema definition", name)
		}
	}
	assertStringEnum(t, defs["PatchApplyStatus"], "inProgress", "completed", "failed", "declined")

	fileUpdate := defs["FileUpdateChange"].(Schema)
	assertSchemaRequiredNames(t, fileUpdate, "path", "kind", "diff")
	fileProperties := fileUpdate["properties"].(Schema)
	if len(fileProperties) != 3 || fileProperties["kind"].(Schema)["$ref"] != "#/$defs/PatchChangeKind" {
		t.Fatalf("FileUpdateChange properties = %#v", fileProperties)
	}

	fileItem := defs["FileChangeItem"].(Schema)
	status := fileItem["properties"].(Schema)["status"].(Schema)
	if status["$ref"] != "#/$defs/PatchApplyStatus" {
		t.Fatalf("FileChangeItem.status = %#v", status)
	}

	kind := defs["PatchChangeKind"].(Schema)
	variants, ok := kind["oneOf"].([]any)
	if !ok || len(variants) != 4 {
		t.Fatalf("PatchChangeKind variants = %#v", kind["oneOf"])
	}
	var sawAdd, sawDelete, sawPublicUpdate, sawLegacyUpdate bool
	for _, raw := range variants {
		variant := raw.(Schema)
		properties := variant["properties"].(Schema)
		typeValues := properties["type"].(Schema)["enum"].([]any)
		if len(typeValues) != 1 {
			t.Fatalf("PatchChangeKind type enum = %#v", typeValues)
		}
		required := schemaRequiredNames(variant)
		switch typeValues[0] {
		case "add":
			sawAdd = len(properties) == 1 && slices.Equal(required, []string{"type"})
		case "delete":
			sawDelete = len(properties) == 1 && slices.Equal(required, []string{"type"})
		case "update":
			if _, ok := properties["move_path"]; !ok {
				t.Fatalf("update variant lacks public move_path: %#v", variant)
			}
			if _, ok := properties["movePath"]; !ok {
				t.Fatalf("update variant lacks legacy movePath: %#v", variant)
			}
			sawPublicUpdate = sawPublicUpdate || slices.Contains(required, "move_path")
			sawLegacyUpdate = sawLegacyUpdate || slices.Contains(required, "movePath")
		default:
			t.Fatalf("unexpected PatchChangeKind variant %#v", variant)
		}
		if variant["additionalProperties"] != false {
			t.Fatalf("PatchChangeKind variant allows extra properties: %#v", variant)
		}
	}
	if !sawAdd || !sawDelete || !sawPublicUpdate || !sawLegacyUpdate {
		t.Fatalf("PatchChangeKind coverage add=%t delete=%t public=%t legacy=%t", sawAdd, sawDelete, sawPublicUpdate, sawLegacyUpdate)
	}
}

func TestPatchChangeKindCanonicalAndLegacyWireBehavior(t *testing.T) {
	movePath := "renamed.txt"
	encoded, err := json.Marshal(PatchChangeKind{Type: "update", MovePath: &movePath})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode marshaled kind: %v", err)
	}
	if payload["type"] != "update" || payload["move_path"] != movePath || payload["movePath"] != movePath {
		t.Fatalf("canonical update payload = %#v", payload)
	}
	for _, kind := range []string{"add", "delete"} {
		encoded, err := json.Marshal(PatchChangeKind{Type: kind})
		if err != nil {
			t.Fatalf("Marshal %s: %v", kind, err)
		}
		if string(encoded) != `{"type":"`+kind+`"}` {
			t.Fatalf("Marshal %s = %s", kind, encoded)
		}
	}
	if _, err := json.Marshal(PatchChangeKind{Type: "add", MovePath: &movePath}); err == nil {
		t.Fatal("Marshal add with move path unexpectedly succeeded")
	}
	if _, err := json.Marshal(PatchChangeKind{Type: "unknown"}); err == nil {
		t.Fatal("Marshal unknown kind unexpectedly succeeded")
	}
	encoded, err = json.Marshal(PatchChangeKind{Type: "update"})
	if err != nil {
		t.Fatalf("Marshal update with null move path: %v", err)
	}
	if err := json.Unmarshal(encoded, &payload); err != nil || payload["move_path"] != nil || payload["movePath"] != nil {
		t.Fatalf("canonical null update = %#v, error %v", payload, err)
	}

	for _, tc := range []struct {
		name string
		wire string
		want *string
	}{
		{name: "public", wire: `{"type":"update","move_path":"public.txt"}`, want: stringPointer("public.txt")},
		{name: "legacy", wire: `{"type":"update","movePath":"legacy.txt"}`, want: stringPointer("legacy.txt")},
		{name: "public empty wins", wire: `{"type":"update","move_path":"","movePath":"legacy.txt"}`, want: stringPointer("")},
		{name: "public null wins", wire: `{"type":"update","move_path":null,"movePath":"legacy.txt"}`, want: nil},
		{name: "public null", wire: `{"type":"update","move_path":null}`, want: nil},
		{name: "legacy null", wire: `{"type":"update","movePath":null}`, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got PatchChangeKind
			if err := json.Unmarshal([]byte(tc.wire), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Type != "update" || !equalStringPointers(got.MovePath, tc.want) {
				t.Fatalf("decoded kind = %#v, want move path %#v", got, tc.want)
			}
		})
	}

	for _, wire := range []string{
		`{}`,
		`{"type":"move","move_path":"other.txt"}`,
		`{"type":"update"}`,
		`{"type":"add","move_path":"other.txt"}`,
		`{"type":"delete","movePath":null}`,
		`{"type":"add","unexpected":true}`,
		`{"type":"update","move_path":12}`,
	} {
		var kind PatchChangeKind
		if err := json.Unmarshal([]byte(wire), &kind); err == nil {
			t.Fatalf("Unmarshal(%s) unexpectedly succeeded with %#v", wire, kind)
		}
	}
	var nilKind *PatchChangeKind
	if err := nilKind.UnmarshalJSON([]byte(`{"type":"add"}`)); err == nil {
		t.Fatal("nil receiver unexpectedly accepted patch kind")
	}
}

func TestFileChangeItemWireV1FixtureUsesPublicAndLegacyContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "file_change_item_wire_v1.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion)
	}

	wantItems := map[string]struct {
		status   PatchApplyStatus
		kind     string
		movePath *string
	}{
		"file-change-add-started":            {status: PatchApplyStatusInProgress, kind: "add"},
		"file-change-delete-completed":       {status: PatchApplyStatusCompleted, kind: "delete"},
		"file-change-public-update-failed":   {status: PatchApplyStatusFailed, kind: "update"},
		"file-change-legacy-update-declined": {status: PatchApplyStatusDeclined, kind: "update", movePath: stringPointer("new-name.txt")},
	}
	seen := make(map[string]bool, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		if seen[fixtureCase.Name] {
			t.Fatalf("duplicate fixture case %q", fixtureCase.Name)
		}
		seen[fixtureCase.Name] = true
		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Fatalf("%s payload: %v", fixtureCase.Name, err)
		}
		if want, ok := wantItems[fixtureCase.Name]; ok {
			var item FileChangeItem
			switch fixtureCase.ParamsType {
			case "FileChangeItemStartedNotificationParams":
				var params FileChangeItemStartedNotificationParams
				if err := decodeRuntimeFixture(payload, &params); err != nil {
					t.Fatalf("%s decode: %v", fixtureCase.Name, err)
				}
				item = params.Item
			case "FileChangeItemCompletedNotificationParams":
				var params FileChangeItemCompletedNotificationParams
				if err := decodeRuntimeFixture(payload, &params); err != nil {
					t.Fatalf("%s decode: %v", fixtureCase.Name, err)
				}
				item = params.Item
			default:
				t.Fatalf("%s params type = %q", fixtureCase.Name, fixtureCase.ParamsType)
			}
			if item.Status != want.status || len(item.Changes) != 1 || item.Changes[0].Kind.Type != want.kind ||
				!equalStringPointers(item.Changes[0].Kind.MovePath, want.movePath) {
				t.Fatalf("%s item = %#v", fixtureCase.Name, item)
			}
			continue
		}
		if fixtureCase.Name == "file-change-patch-updated-public-precedence" {
			var params FileChangePatchUpdatedNotification
			if err := decodeRuntimeFixture(payload, &params); err != nil {
				t.Fatalf("%s decode: %v", fixtureCase.Name, err)
			}
			if len(params.Changes) != 1 || !equalStringPointers(params.Changes[0].Kind.MovePath, stringPointer("public-name.txt")) {
				t.Fatalf("public precedence changes = %#v", params.Changes)
			}
			continue
		}
		t.Fatalf("unexpected fixture case %q", fixtureCase.Name)
	}
	if len(seen) != len(wantItems)+1 {
		t.Fatalf("fixture case count = %d, want %d", len(seen), len(wantItems)+1)
	}
}

func assertSchemaRequiredNames(t *testing.T, schema Schema, names ...string) {
	t.Helper()
	if got := schemaRequiredNames(schema); !slices.Equal(got, names) {
		t.Fatalf("required = %v, want %v", got, names)
	}
}

func schemaRequiredNames(schema Schema) []string {
	raw, _ := schema["required"].([]string)
	return raw
}

func stringPointer(value string) *string {
	return &value
}

func equalStringPointers(left, right *string) bool {
	if left == nil || right == nil {
		return left == right
	}
	return *left == *right
}
