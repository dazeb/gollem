package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestThreadMetadataNullablePatchAndCompatibilityHelpers(t *testing.T) {
	var gitInfo ThreadMetadataGitInfoUpdateParams
	if err := json.Unmarshal([]byte(`{"sha":null,"branch":" main "}`), &gitInfo); err != nil {
		t.Fatalf("decode git info: %v", err)
	}
	if !gitInfo.HasSHA() || gitInfo.SHA != nil || !gitInfo.HasBranch() || gitInfo.Branch == nil || *gitInfo.Branch != " main " || gitInfo.HasOriginURL() {
		t.Fatalf("decoded git info = %#v", gitInfo)
	}
	encoded, err := json.Marshal(gitInfo)
	if err != nil {
		t.Fatalf("encode git info: %v", err)
	}
	if !strings.Contains(string(encoded), `"sha":null`) || !strings.Contains(string(encoded), `"branch":" main "`) {
		t.Fatalf("encoded git info = %s", encoded)
	}

	var params ThreadMetadataUpdateParams
	if err := json.Unmarshal([]byte(`{"threadId":"thread-1","gitInfo":null}`), &params); err != nil {
		t.Fatalf("decode metadata params: %v", err)
	}
	if !params.HasGitInfo() || params.GitInfo != nil || params.EffectiveThreadID() != "thread-1" {
		t.Fatalf("decoded metadata params = %#v", params)
	}
	encoded, err = json.Marshal(params)
	if err != nil {
		t.Fatalf("encode metadata params: %v", err)
	}
	if !strings.Contains(string(encoded), `"gitInfo":null`) {
		t.Fatalf("encoded metadata params = %s", encoded)
	}

	legacyParams := ThreadMetadataUpdateParams{ID: "legacy"}
	if legacyParams.EffectiveThreadID() != "legacy" {
		t.Fatalf("legacy metadata id = %q", legacyParams.EffectiveThreadID())
	}
	publicMemory := ThreadMemoryModeSetParams{ThreadID: "public", Mode: ThreadMemoryModeDisabled, ID: "legacy", MemoryMode: ThreadMemoryModeEnabled}
	if publicMemory.EffectiveThreadID() != "public" || publicMemory.EffectiveMode() != ThreadMemoryModeDisabled {
		t.Fatalf("public memory params = %#v", publicMemory)
	}
	legacyMemory := ThreadMemoryModeSetParams{ID: "legacy", MemoryMode: ThreadMemoryModeEnabled}
	if legacyMemory.EffectiveThreadID() != "legacy" || legacyMemory.EffectiveMode() != ThreadMemoryModeEnabled {
		t.Fatalf("legacy memory params = %#v", legacyMemory)
	}
	if !ThreadMemoryModeEnabled.Valid() || !ThreadMemoryModeDisabled.Valid() || ThreadMemoryMode("sometimes").Valid() {
		t.Fatal("thread memory mode validation is incorrect")
	}
}

func TestThreadMetadataSchemaAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ThreadMemoryMode",
		"ThreadMemoryModeSetParams",
		"ThreadMemoryModeSetResponse",
		"ThreadMetadataGitInfoUpdateParams",
		"ThreadMetadataUpdateParams",
		"ThreadMetadataUpdateResponse",
		"ThreadNameUpdatedNotification",
	} {
		if defs[name] == nil {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	assertStringEnum(t, defs["ThreadMemoryMode"], "enabled", "disabled")
	assertSchemaRequiredIDAlternative(t, defs["ThreadMetadataUpdateParams"].(Schema))
	metadataParams := firstSchemaVariant(t, defs["ThreadMetadataUpdateParams"].(Schema))
	assertSchemaNullableProperty(t, metadataParams, "gitInfo")
	gitInfo := defs["ThreadMetadataGitInfoUpdateParams"].(Schema)
	for _, field := range []string{"sha", "branch", "originUrl"} {
		if slices.Contains(schemaRequiredFields(gitInfo), field) {
			t.Errorf("ThreadMetadataGitInfoUpdateParams.%s must remain optional", field)
		}
		assertSchemaNullableProperty(t, gitInfo, field)
	}
	assertSchemaRequiredFieldAlternatives(t, defs["ThreadMemoryModeSetParams"].(Schema),
		[]string{"threadId", "mode"},
		[]string{"id", "mode"},
		[]string{"threadId", "memoryMode"},
		[]string{"id", "memoryMode"},
	)
	assertNoRequiredFields(t, defs["ThreadMemoryModeSetResponse"].(Schema))
	assertSchemaRequired(t, defs["ThreadMetadataUpdateResponse"].(Schema), "thread")
	nameNotification := defs["ThreadNameUpdatedNotification"].(Schema)
	assertSchemaRequired(t, nameNotification, "threadId")
	if slices.Contains(schemaRequiredFields(nameNotification), "threadName") {
		t.Error("ThreadNameUpdatedNotification.threadName must remain optional")
	}
	nameProperties, _ := nameNotification["properties"].(Schema)
	threadName, _ := nameProperties["threadName"].(Schema)
	if threadName["type"] != "string" {
		t.Errorf("ThreadNameUpdatedNotification.threadName = %#v, want optional non-null string", threadName)
	}

	bindings := WireTypeBindings()
	for _, test := range []struct {
		method string
		params string
		result string
	}{
		{method: "thread/memoryMode/set", params: "ThreadMemoryModeSetParams", result: "ThreadMemoryModeSetResponse"},
		{method: "thread/metadata/update", params: "ThreadMetadataUpdateParams", result: "ThreadMetadataUpdateResponse"},
	} {
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.params)
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.result)
	}
	assertBinding(t, bindings, "thread/name/updated", SurfaceServerNotification, "ThreadNameUpdatedNotification")
}

func TestThreadMetadataWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "thread_metadata_wire_v1.json"))
	if err != nil {
		t.Fatalf("read thread metadata fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode thread metadata fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 7 {
		t.Fatalf("thread metadata fixture has %d cases, want 7", len(fixture.Cases))
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

func assertSchemaRequiredFieldAlternatives(t *testing.T, definition Schema, want ...[]string) {
	t.Helper()
	variants := requiredFieldAlternatives(definition)
	if len(variants) != len(want) {
		t.Fatalf("required alternatives = %#v, want %d variants", variants, len(want))
	}
	for index, raw := range variants {
		variant, ok := raw.(Schema)
		if !ok {
			t.Fatalf("required alternative %d = %#v, want schema", index, raw)
		}
		if got := schemaRequiredFields(variant); !slices.Equal(got, want[index]) {
			t.Errorf("required alternative %d = %v, want %v", index, got, want[index])
		}
	}
}
