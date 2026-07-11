package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestCommandActionSchemaIsExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	action, ok := defs["CommandAction"].(Schema)
	if !ok {
		t.Fatal("$defs missing CommandAction")
	}
	variants, ok := action["oneOf"].([]any)
	if !ok || len(variants) != 4 {
		t.Fatalf("CommandAction variants = %#v", action["oneOf"])
	}
	want := []struct {
		actionType string
		required   []string
	}{
		{actionType: "read", required: []string{"type", "command", "name", "path"}},
		{actionType: "listFiles", required: []string{"type", "command", "path"}},
		{actionType: "search", required: []string{"type", "command", "query", "path"}},
		{actionType: "unknown", required: []string{"type", "command"}},
	}
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("CommandAction %s allows extra fields", expected.actionType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.actionType}) {
			t.Fatalf("CommandAction variant %d type = %#v", index, properties["type"])
		}
		if !slices.Equal(schemaRequiredNames(variant), expected.required) {
			t.Fatalf("CommandAction %s required = %v, want %v", expected.actionType, schemaRequiredNames(variant), expected.required)
		}
	}

	readProperties := variants[0].(Schema)["properties"].(Schema)
	if readProperties["path"].(Schema)["$ref"] != "#/$defs/AbsolutePathBuf" {
		t.Fatalf("read path = %#v", readProperties["path"])
	}
	listProperties := variants[1].(Schema)["properties"].(Schema)
	assertNullableStringSchema(t, listProperties["path"])
	searchProperties := variants[2].(Schema)["properties"].(Schema)
	assertNullableStringSchema(t, searchProperties["query"])
	assertNullableStringSchema(t, searchProperties["path"])
}

func TestCommandActionWireValidationAndCanonicalization(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{
			input: `{"type":"read","command":"cat file","name":"cat","path":"/workspace/dir/../file"}`,
			want:  `{"type":"read","command":"cat file","name":"cat","path":"/workspace/file"}`,
		},
		{
			input: `{"type":"listFiles","command":"ls","path":null}`,
			want:  `{"type":"listFiles","command":"ls","path":null}`,
		},
		{
			input: `{"type":"listFiles","command":"ls src","path":"relative/src"}`,
			want:  `{"type":"listFiles","command":"ls src","path":"relative/src"}`,
		},
		{
			input: `{"type":"search","command":"rg needle","query":null,"path":null}`,
			want:  `{"type":"search","command":"rg needle","query":null,"path":null}`,
		},
		{
			input: `{"type":"search","command":"rg needle src","query":"needle","path":"relative/src"}`,
			want:  `{"type":"search","command":"rg needle src","query":"needle","path":"relative/src"}`,
		},
		{
			input: `{"type":"unknown","command":"custom --flag"}`,
			want:  `{"type":"unknown","command":"custom --flag"}`,
		},
	}
	for _, testCase := range valid {
		var action CommandAction
		if err := json.Unmarshal([]byte(testCase.input), &action); err != nil {
			t.Errorf("Unmarshal(%s): %v", testCase.input, err)
			continue
		}
		encoded, err := json.Marshal(action)
		if err != nil {
			t.Errorf("Marshal(%s): %v", testCase.input, err)
			continue
		}
		if string(encoded) != testCase.want {
			t.Errorf("round trip %s = %s, want %s", testCase.input, encoded, testCase.want)
		}
	}

	invalid := []string{
		`null`, `[]`, `{}`,
		`{"type":"read","command":"cat","name":"cat"}`,
		`{"type":"read","command":"cat","name":"cat","path":null}`,
		`{"type":"read","command":"cat","name":"cat","path":"relative"}`,
		`{"type":"read","command":null,"name":"cat","path":"/file"}`,
		`{"type":"read","command":"cat","name":null,"path":"/file"}`,
		`{"type":"read","command":"cat","name":"cat","path":"/file","query":null}`,
		`{"type":"listFiles","command":"ls"}`,
		`{"type":"listFiles","command":"ls","path":1}`,
		`{"type":"listFiles","command":"ls","path":null,"name":"ls"}`,
		`{"type":"search","command":"rg","path":null}`,
		`{"type":"search","command":"rg","query":null}`,
		`{"type":"search","command":"rg","query":1,"path":null}`,
		`{"type":"search","command":"rg","query":null,"path":false}`,
		`{"type":"search","command":"rg","query":null,"path":null,"name":"rg"}`,
		`{"type":"unknown"}`,
		`{"type":"unknown","command":"custom","path":null}`,
		`{"type":"execute","command":"run"}`,
		`{"type":1,"command":"run"}`,
		`{"type":"unknown","command":"run","extra":true}`,
	}
	for _, input := range invalid {
		var action CommandAction
		if err := json.Unmarshal([]byte(input), &action); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
}

func TestCommandActionEmptyAndNilReceiversFailClosed(t *testing.T) {
	if _, err := json.Marshal(CommandAction{}); err == nil {
		t.Fatal("Marshal empty CommandAction succeeded")
	}
	var action *CommandAction
	if err := action.UnmarshalJSON([]byte(`{"type":"unknown","command":"run"}`)); err == nil {
		t.Fatal("nil CommandAction receiver succeeded")
	}
}
