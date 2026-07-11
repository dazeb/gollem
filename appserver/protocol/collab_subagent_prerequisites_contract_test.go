package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestCollabSubagentPrerequisiteSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	if !reflect.DeepEqual(defs["AgentPath"], Schema{"type": "string"}) {
		t.Fatalf("AgentPath schema = %#v", defs["AgentPath"])
	}
	if !reflect.DeepEqual(defs["ReasoningEffort"], Schema{"type": "string", "minLength": 1}) {
		t.Fatalf("ReasoningEffort schema = %#v", defs["ReasoningEffort"])
	}
	assertStringEnumSchema(t, defs, "CollabAgentStatus", []string{
		"pendingInit", "running", "interrupted", "completed", "errored", "shutdown", "notFound",
	})
	assertStringEnumSchema(t, defs, "CollabAgentTool", []string{
		"spawnAgent", "sendInput", "resumeAgent", "wait", "closeAgent",
	})
	assertStringEnumSchema(t, defs, "CollabAgentToolCallStatus", []string{
		"inProgress", "completed", "failed",
	})
	assertStringEnumSchema(t, defs, "SubAgentActivityKind", []string{
		"started", "interacted", "interrupted",
	})

	state, ok := defs["CollabAgentState"].(Schema)
	if !ok {
		t.Fatal("$defs missing CollabAgentState")
	}
	if state["additionalProperties"] != false {
		t.Fatalf("CollabAgentState allows extra fields: %#v", state)
	}
	if !slices.Equal(schemaRequiredNames(state), []string{"status", "message"}) {
		t.Fatalf("CollabAgentState required = %v", schemaRequiredNames(state))
	}
	properties := state["properties"].(Schema)
	if properties["status"].(Schema)["$ref"] != "#/$defs/CollabAgentStatus" {
		t.Fatalf("CollabAgentState status = %#v", properties["status"])
	}
	assertNullableStringSchema(t, properties["message"])
}

func TestCollabSubagentEnumsRejectUnknownValues(t *testing.T) {
	tests := []struct {
		name    string
		values  []string
		decode  func([]byte) error
		marshal func(string) error
	}{
		{
			name:   "collab status",
			values: []string{"pendingInit", "running", "interrupted", "completed", "errored", "shutdown", "notFound"},
			decode: func(data []byte) error {
				var value CollabAgentStatus
				return json.Unmarshal(data, &value)
			},
			marshal: func(value string) error {
				_, err := json.Marshal(CollabAgentStatus(value))
				return err
			},
		},
		{
			name:   "collab tool",
			values: []string{"spawnAgent", "sendInput", "resumeAgent", "wait", "closeAgent"},
			decode: func(data []byte) error {
				var value CollabAgentTool
				return json.Unmarshal(data, &value)
			},
			marshal: func(value string) error {
				_, err := json.Marshal(CollabAgentTool(value))
				return err
			},
		},
		{
			name:   "collab tool-call status",
			values: []string{"inProgress", "completed", "failed"},
			decode: func(data []byte) error {
				var value CollabAgentToolCallStatus
				return json.Unmarshal(data, &value)
			},
			marshal: func(value string) error {
				_, err := json.Marshal(CollabAgentToolCallStatus(value))
				return err
			},
		},
		{
			name:   "subagent activity",
			values: []string{"started", "interacted", "interrupted"},
			decode: func(data []byte) error {
				var value SubAgentActivityKind
				return json.Unmarshal(data, &value)
			},
			marshal: func(value string) error {
				_, err := json.Marshal(SubAgentActivityKind(value))
				return err
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			for _, value := range testCase.values {
				if err := testCase.decode([]byte(`"` + value + `"`)); err != nil {
					t.Errorf("decode %q: %v", value, err)
				}
				if err := testCase.marshal(value); err != nil {
					t.Errorf("marshal %q: %v", value, err)
				}
			}
			for _, data := range [][]byte{[]byte(`null`), []byte(`"unknown"`), []byte(`1`), []byte(`{}`)} {
				if err := testCase.decode(data); err == nil {
					t.Errorf("decode %s succeeded", data)
				}
			}
			for _, value := range []string{"", "unknown"} {
				if err := testCase.marshal(value); err == nil {
					t.Errorf("marshal %q succeeded", value)
				}
			}
		})
	}

	var nilStatus *CollabAgentStatus
	if err := nilStatus.UnmarshalJSON([]byte(`"running"`)); err == nil {
		t.Fatal("nil CollabAgentStatus receiver succeeded")
	}
}

func TestAgentPathAndReasoningEffortWireValidation(t *testing.T) {
	for _, value := range []string{"", "agent/1", "not/a/filesystem/path"} {
		var path AgentPath
		if err := json.Unmarshal([]byte(`"`+value+`"`), &path); err != nil {
			t.Errorf("decode AgentPath %q: %v", value, err)
		}
		encoded, err := json.Marshal(path)
		if err != nil || string(encoded) != `"`+value+`"` {
			t.Errorf("marshal AgentPath %q = %s, %v", value, encoded, err)
		}
	}
	for _, data := range []string{`null`, `1`, `{}`} {
		var path AgentPath
		if err := json.Unmarshal([]byte(data), &path); err == nil {
			t.Errorf("decode AgentPath %s succeeded", data)
		}
	}
	var nilPath *AgentPath
	if err := nilPath.UnmarshalJSON([]byte(`"agent/1"`)); err == nil {
		t.Fatal("nil AgentPath receiver succeeded")
	}

	for _, value := range []string{"low", "custom", " "} {
		var effort ReasoningEffort
		if err := json.Unmarshal([]byte(`"`+value+`"`), &effort); err != nil {
			t.Errorf("decode ReasoningEffort %q: %v", value, err)
		}
		encoded, err := json.Marshal(effort)
		if err != nil || string(encoded) != `"`+value+`"` {
			t.Errorf("marshal ReasoningEffort %q = %s, %v", value, encoded, err)
		}
	}
	for _, data := range []string{`null`, `""`, `1`, `{}`} {
		var effort ReasoningEffort
		if err := json.Unmarshal([]byte(data), &effort); err == nil {
			t.Errorf("decode ReasoningEffort %s succeeded", data)
		}
	}
	if _, err := json.Marshal(ReasoningEffort("")); err == nil {
		t.Fatal("marshal empty ReasoningEffort succeeded")
	}
	var nilEffort *ReasoningEffort
	if err := nilEffort.UnmarshalJSON([]byte(`"low"`)); err == nil {
		t.Fatal("nil ReasoningEffort receiver succeeded")
	}
}

func TestCollabAgentStateWireValidation(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{input: `{"status":"pendingInit","message":null}`, want: `{"status":"pendingInit","message":null}`},
		{input: `{"status":"running","message":"working"}`, want: `{"status":"running","message":"working"}`},
		{input: `{"status":"completed","message":""}`, want: `{"status":"completed","message":""}`},
	}
	for _, testCase := range valid {
		var state CollabAgentState
		if err := json.Unmarshal([]byte(testCase.input), &state); err != nil {
			t.Errorf("Unmarshal(%s): %v", testCase.input, err)
			continue
		}
		encoded, err := json.Marshal(state)
		if err != nil || string(encoded) != testCase.want {
			t.Errorf("round trip %s = %s, %v", testCase.input, encoded, err)
		}
	}
	for _, input := range []string{
		`null`, `[]`, `{}`,
		`{"status":"running"}`,
		`{"message":null}`,
		`{"status":null,"message":null}`,
		`{"status":"unknown","message":null}`,
		`{"status":"running","message":1}`,
		`{"status":"running","message":null,"extra":true}`,
	} {
		var state CollabAgentState
		if err := json.Unmarshal([]byte(input), &state); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	if _, err := json.Marshal(CollabAgentState{Message: nil}); err == nil {
		t.Fatal("marshal CollabAgentState with empty status succeeded")
	}
	var nilState *CollabAgentState
	if err := nilState.UnmarshalJSON([]byte(`{"status":"running","message":null}`)); err == nil {
		t.Fatal("nil CollabAgentState receiver succeeded")
	}
}

func assertStringEnumSchema(t *testing.T, defs Schema, name string, want []string) {
	t.Helper()
	schema, ok := defs[name].(Schema)
	if !ok {
		t.Fatalf("$defs missing %s", name)
	}
	values, ok := schema["enum"].([]any)
	if !ok {
		t.Fatalf("%s enum = %#v", name, schema["enum"])
	}
	got := make([]string, len(values))
	for index, value := range values {
		got[index] = value.(string)
	}
	if schema["type"] != "string" || !slices.Equal(got, want) {
		t.Fatalf("%s schema = %#v, want %v", name, schema, want)
	}
}
