package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestThreadGoalSetParamsTracksNullableBudget(t *testing.T) {
	for _, test := range []struct {
		name       string
		payload    string
		present    bool
		wantBudget *int64
	}{
		{name: "omitted", payload: `{"threadId":"thread-1"}`},
		{name: "null", payload: `{"threadId":"thread-1","tokenBudget":null}`, present: true},
		{name: "number", payload: `{"threadId":"thread-1","tokenBudget":12000}`, present: true, wantBudget: int64Pointer(12000)},
	} {
		t.Run(test.name, func(t *testing.T) {
			var params ThreadGoalSetParams
			if err := json.Unmarshal([]byte(test.payload), &params); err != nil {
				t.Fatalf("decode params: %v", err)
			}
			if params.HasTokenBudget() != test.present {
				t.Fatalf("HasTokenBudget = %v, want %v", params.HasTokenBudget(), test.present)
			}
			if (params.TokenBudget == nil) != (test.wantBudget == nil) ||
				(params.TokenBudget != nil && *params.TokenBudget != *test.wantBudget) {
				t.Fatalf("TokenBudget = %v, want %v", params.TokenBudget, test.wantBudget)
			}
		})
	}
	params := ThreadGoalSetParams{ThreadID: "thread-1"}
	params.SetTokenBudget(nil)
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode explicit null budget: %v", err)
	}
	if !strings.Contains(string(encoded), `"tokenBudget":null`) {
		t.Fatalf("encoded params = %s, want explicit null budget", encoded)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func TestThreadGoalSchemaAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ThreadGoal",
		"ThreadGoalClearParams",
		"ThreadGoalClearResponse",
		"ThreadGoalClearedNotification",
		"ThreadGoalGetParams",
		"ThreadGoalGetResponse",
		"ThreadGoalSetParams",
		"ThreadGoalSetResponse",
		"ThreadGoalStatus",
		"ThreadGoalUpdatedNotification",
	} {
		if defs[name] == nil {
			t.Errorf("schema missing %s", name)
		}
	}
	if t.Failed() {
		t.FailNow()
	}

	assertStringEnum(t, defs["ThreadGoalStatus"], "active", "paused", "blocked", "usageLimited", "budgetLimited", "complete")
	assertSchemaRequired(t, defs["ThreadGoal"].(Schema),
		"threadId", "objective", "status", "tokenBudget", "tokensUsed", "timeUsedSeconds", "createdAt", "updatedAt")
	assertSchemaNullableProperty(t, defs["ThreadGoal"].(Schema), "tokenBudget")
	for _, name := range []string{"ThreadGoalGetParams", "ThreadGoalClearParams"} {
		assertSchemaRequired(t, defs[name].(Schema), "threadId")
	}
	assertSchemaRequired(t, defs["ThreadGoalSetParams"].(Schema), "threadId")
	for _, field := range []string{"objective", "status", "tokenBudget"} {
		if slices.Contains(schemaRequiredFields(defs["ThreadGoalSetParams"].(Schema)), field) {
			t.Errorf("ThreadGoalSetParams.%s must remain optional", field)
		}
		assertSchemaNullableProperty(t, defs["ThreadGoalSetParams"].(Schema), field)
	}
	assertSchemaRequired(t, defs["ThreadGoalSetResponse"].(Schema), "goal")
	assertSchemaRequired(t, defs["ThreadGoalGetResponse"].(Schema), "goal")
	assertSchemaNullableProperty(t, defs["ThreadGoalGetResponse"].(Schema), "goal")
	assertSchemaRequired(t, defs["ThreadGoalClearResponse"].(Schema), "cleared")
	assertSchemaRequired(t, defs["ThreadGoalUpdatedNotification"].(Schema), "threadId", "turnId", "goal")
	assertSchemaNullableProperty(t, defs["ThreadGoalUpdatedNotification"].(Schema), "turnId")
	assertSchemaRequired(t, defs["ThreadGoalClearedNotification"].(Schema), "threadId")

	bindings := WireTypeBindings()
	for _, test := range []struct {
		method string
		params string
		result string
	}{
		{method: "thread/goal/clear", params: "ThreadGoalClearParams", result: "ThreadGoalClearResponse"},
		{method: "thread/goal/get", params: "ThreadGoalGetParams", result: "ThreadGoalGetResponse"},
		{method: "thread/goal/set", params: "ThreadGoalSetParams", result: "ThreadGoalSetResponse"},
	} {
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.params)
		assertBinding(t, bindings, test.method, SurfaceClientRequest, test.result)
	}
	assertBinding(t, bindings, "thread/goal/updated", SurfaceServerNotification, "ThreadGoalUpdatedNotification")
	assertBinding(t, bindings, "thread/goal/cleared", SurfaceServerNotification, "ThreadGoalClearedNotification")
}

func TestThreadGoalWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "thread_goal_wire_v1.json"))
	if err != nil {
		t.Fatalf("read thread goal fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode thread goal fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 8 {
		t.Fatalf("thread goal fixture has %d cases, want 8", len(fixture.Cases))
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

func assertSchemaNullableProperty(t *testing.T, definition Schema, field string) {
	t.Helper()
	properties, _ := definition["properties"].(Schema)
	property, _ := properties[field].(Schema)
	variants, _ := property["anyOf"].([]any)
	for _, variant := range variants {
		if schema, ok := variant.(Schema); ok && schema["type"] == "null" {
			return
		}
	}
	t.Errorf("%s is not nullable: %#v", field, property)
}
