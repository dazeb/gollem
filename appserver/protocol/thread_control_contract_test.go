package protocol

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestThreadControlSchemaAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ThreadArchiveParams",
		"ThreadArchiveResponse",
		"ThreadDeleteParams",
		"ThreadDeleteResponse",
		"ThreadLoadedListParams",
		"ThreadLoadedListResponse",
		"ThreadSetNameParams",
		"ThreadSetNameResponse",
		"ThreadUnarchiveParams",
		"ThreadUnarchiveResponse",
		"ThreadUnsubscribeParams",
		"ThreadUnsubscribeResponse",
		"ThreadUnsubscribeStatus",
	} {
		if defs[name] == nil {
			t.Errorf("schema missing %s", name)
		}
	}

	assertStringEnum(t, defs["ThreadUnsubscribeStatus"], "notLoaded", "notSubscribed", "unsubscribed")
	for _, name := range []string{
		"ThreadArchiveParams",
		"ThreadDeleteParams",
		"ThreadUnarchiveParams",
		"ThreadUnsubscribeParams",
	} {
		assertSchemaRequired(t, defs[name].(Schema), "threadId")
	}
	assertSchemaRequired(t, defs["ThreadSetNameParams"].(Schema), "threadId", "name")
	assertSchemaRequired(t, defs["ThreadLoadedListResponse"].(Schema), "data")
	assertSchemaRequired(t, defs["ThreadUnarchiveResponse"].(Schema), "thread")
	assertSchemaRequired(t, defs["ThreadUnsubscribeResponse"].(Schema), "status")
	for _, name := range []string{"ThreadArchiveResponse", "ThreadDeleteResponse", "ThreadSetNameResponse"} {
		assertNoRequiredFields(t, defs[name].(Schema))
	}
	loadedResponse := defs["ThreadLoadedListResponse"].(Schema)
	if slices.Contains(schemaRequiredFields(loadedResponse), "nextCursor") {
		t.Error("ThreadLoadedListResponse.nextCursor must remain optional")
	}

	bindings := WireTypeBindings()
	for _, test := range []struct {
		method string
		params string
		result string
	}{
		{method: "thread/archive", params: "ThreadArchiveParams", result: "ThreadArchiveResponse"},
		{method: "thread/delete", params: "ThreadDeleteParams", result: "ThreadDeleteResponse"},
		{method: "thread/loaded/list", params: "ThreadLoadedListParams", result: "ThreadLoadedListResponse"},
		{method: "thread/name/set", params: "ThreadSetNameParams", result: "ThreadSetNameResponse"},
		{method: "thread/unarchive", params: "ThreadUnarchiveParams", result: "ThreadUnarchiveResponse"},
		{method: "thread/unsubscribe", params: "ThreadUnsubscribeParams", result: "ThreadUnsubscribeResponse"},
	} {
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.params)
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.result)
	}
}

func TestThreadControlWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "thread_control_wire_v1.json"))
	if err != nil {
		t.Fatalf("read thread control fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode thread control fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 12 {
		t.Fatalf("thread control fixture has %d cases, want 12", len(fixture.Cases))
	}
	bindings := WireTypeBindings()
	seen := make(map[string]bool, len(fixture.Cases))
	for _, fixtureCase := range fixture.Cases {
		if seen[fixtureCase.Name] {
			t.Errorf("duplicate fixture case %q", fixtureCase.Name)
			continue
		}
		seen[fixtureCase.Name] = true
		payload, err := fixtureMessagePayload(fixtureCase)
		if err != nil {
			t.Errorf("%s: %v", fixtureCase.Name, err)
			continue
		}
		target := runtimeFixtureTarget(firstFixtureType(fixtureCase))
		if target == nil {
			t.Errorf("%s: unsupported fixture type", fixtureCase.Name)
			continue
		}
		if err := decodeRuntimeFixture(payload, target); err != nil {
			t.Errorf("%s: decode %s: %v", fixtureCase.Name, firstFixtureType(fixtureCase), err)
		}
		if fixtureCase.ParamsType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ParamsType)
		}
		if fixtureCase.ResultType != "" {
			assertBinding(t, bindings, fixtureCase.Method, fixtureCase.Surface, fixtureCase.ResultType)
		}
	}
}

func assertNoRequiredFields(t *testing.T, definition Schema) {
	t.Helper()
	if required := schemaRequiredFields(definition); len(required) > 0 {
		t.Fatalf("required = %v, want none", required)
	}
}

func schemaRequiredFields(definition Schema) []string {
	required, _ := definition["required"].([]string)
	return required
}
