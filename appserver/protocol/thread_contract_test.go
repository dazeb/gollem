package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestThreadDiscoverySchemaAndBindings(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"SortDirection",
		"ThreadLifecycleStatus",
		"ThreadListCwdFilter",
		"ThreadListParams",
		"ThreadListResponse",
		"ThreadListResult",
		"ThreadReadParams",
		"ThreadReadResponse",
		"ThreadReadResult",
		"ThreadRecord",
		"ThreadSortKey",
		"ThreadSourceKind",
		"TurnLifecycleStatus",
		"TurnRecord",
	} {
		if defs[name] == nil {
			t.Errorf("schema missing %s", name)
		}
	}

	assertStringEnum(t, defs["SortDirection"], "asc", "desc")
	assertStringEnum(t, defs["ThreadSortKey"], "created_at", "updated_at", "recency_at")
	assertStringEnum(t, defs["ThreadSourceKind"],
		"cli", "vscode", "exec", "appServer", "subAgent", "subAgentReview",
		"subAgentCompact", "subAgentThreadSpawn", "subAgentOther", "unknown",
	)
	cwd := defs["ThreadListCwdFilter"].(Schema)
	if variants, ok := cwd["oneOf"].([]any); !ok || len(variants) != 2 {
		t.Fatalf("ThreadListCwdFilter oneOf = %#v", cwd["oneOf"])
	}

	listParams := defs["ThreadListParams"].(Schema)
	listProperties := listParams["properties"].(Schema)
	for _, name := range []string{
		"archived", "cursor", "cwd", "limit", "modelProviders", "searchTerm",
		"sortDirection", "sortKey", "sourceKinds", "useStateDbOnly",
		"statuses", "includeDeleted",
	} {
		if listProperties[name] == nil {
			t.Errorf("ThreadListParams missing %s", name)
		}
	}
	readParams := defs["ThreadReadParams"].(Schema)
	assertSchemaRequired(t, readParams, "threadId")
	listResult := defs["ThreadListResult"].(Schema)
	assertSchemaRequired(t, listResult, "data")
	for _, name := range listResult["required"].([]string) {
		if name == "threads" {
			t.Error("ThreadListResult legacy threads field must remain optional in the live schema")
		}
	}

	bindings := WireTypeBindings()
	assertBinding(t, bindings, "thread/list", SurfaceClientRequest, "ThreadListParams")
	assertBinding(t, bindings, "thread/list", SurfaceClientRequest, "ThreadListResult")
	assertBinding(t, bindings, "thread/read", SurfaceClientRequest, "ThreadReadParams")
	assertBinding(t, bindings, "thread/read", SurfaceClientRequest, "ThreadReadResult")
}

func TestThreadListCwdFilterPreservesStringAndArrayWireForms(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "one", input: `{"cwd":"/workspace"}`, want: []string{"/workspace"}},
		{name: "many", input: `{"cwd":["/workspace/a","/workspace/b"]}`, want: []string{"/workspace/a", "/workspace/b"}},
		{name: "empty", input: `{"cwd":[]}`, want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var params ThreadListParams
			if err := json.Unmarshal([]byte(test.input), &params); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if params.CWD == nil || !sameStrings(params.CWD.Paths(), test.want) {
				t.Fatalf("cwd paths = %#v, want %#v", params.CWD, test.want)
			}
			encoded, err := json.Marshal(params)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var roundTrip map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatalf("decode round trip: %v", err)
			}
			if string(roundTrip["cwd"]) != test.input[len(`{"cwd":`):len(test.input)-1] {
				t.Fatalf("cwd round trip = %s, input = %s", roundTrip["cwd"], test.input)
			}
		})
	}
}

func TestThreadListCwdFilterConstructorsPreserveWireForms(t *testing.T) {
	tests := []struct {
		name string
		cwd  ThreadListCwdFilter
		want string
	}{
		{name: "one", cwd: NewThreadListCwdFilter("/workspace"), want: `"/workspace"`},
		{name: "many", cwd: NewThreadListCwdFilters([]string{"/workspace/a", "/workspace/b"}), want: `["/workspace/a","/workspace/b"]`},
		{name: "empty", cwd: NewThreadListCwdFilters(nil), want: `[]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.cwd)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(encoded) != test.want {
				t.Fatalf("cwd = %s, want %s", encoded, test.want)
			}
		})
	}
}

func TestThreadDiscoveryWireV1FixtureUsesExportedContracts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "thread_discovery_wire_v1.json"))
	if err != nil {
		t.Fatalf("read thread discovery fixture: %v", err)
	}
	var fixture runtimeWireFixture
	if err := decodeRuntimeFixture(data, &fixture); err != nil {
		t.Fatalf("decode thread discovery fixture: %v", err)
	}
	if fixture.ProtocolVersion != ProtocolVersion || fixture.SchemaVersion != SchemaVersion {
		t.Fatalf("fixture versions = %s/%s, want %s/%s", fixture.ProtocolVersion, fixture.SchemaVersion, ProtocolVersion, SchemaVersion)
	}
	if len(fixture.Cases) != 4 {
		t.Fatalf("thread discovery fixture has %d cases, want 4", len(fixture.Cases))
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
	for _, name := range []string{"thread-list-request", "thread-list-response", "thread-read-request", "thread-read-response"} {
		if !seen[name] {
			t.Errorf("fixture missing %s", name)
		}
	}
}

func assertStringEnum(t *testing.T, raw any, values ...string) {
	t.Helper()
	schema, ok := raw.(Schema)
	if !ok {
		t.Fatalf("enum schema = %#v", raw)
	}
	got, ok := schema["enum"].([]any)
	if !ok || len(got) != len(values) {
		t.Fatalf("enum = %#v, want %#v", schema["enum"], values)
	}
	for i, value := range values {
		if got[i] != value {
			t.Fatalf("enum[%d] = %#v, want %q", i, got[i], value)
		}
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
