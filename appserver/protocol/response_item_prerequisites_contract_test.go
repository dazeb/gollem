package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestResponseItemPrerequisiteSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	assertContentItemSchema(t, defs, "ContentItem", []contentItemSchemaExpectation{
		{itemType: "input_text", field: "text"},
		{itemType: "input_image", field: "image_url", optionalDetail: true},
		{itemType: "output_text", field: "text"},
	})
	assertContentItemSchema(t, defs, "FunctionCallOutputContentItem", []contentItemSchemaExpectation{
		{itemType: "input_text", field: "text"},
		{itemType: "input_image", field: "image_url", optionalDetail: true},
		{itemType: "encrypted_content", field: "encrypted_content"},
	})

	body := defs["FunctionCallOutputBody"].(Schema)
	bodyVariants, ok := body["anyOf"].([]any)
	if !ok || len(bodyVariants) != 2 {
		t.Fatalf("FunctionCallOutputBody variants = %#v", body)
	}
	if !reflect.DeepEqual(bodyVariants[0], Schema{"type": "string"}) {
		t.Fatalf("FunctionCallOutputBody string = %#v", bodyVariants[0])
	}
	if !reflect.DeepEqual(bodyVariants[1], Schema{
		"type": "array", "items": Schema{"$ref": "#/$defs/FunctionCallOutputContentItem"},
	}) {
		t.Fatalf("FunctionCallOutputBody array = %#v", bodyVariants[1])
	}

	metadata := defs["InternalChatMessageMetadataPassthrough"].(Schema)
	if metadata["type"] != "object" || metadata["additionalProperties"] != false {
		t.Fatalf("InternalChatMessageMetadataPassthrough = %#v", metadata)
	}
	if required := schemaRequiredNames(metadata); len(required) != 0 {
		t.Fatalf("InternalChatMessageMetadataPassthrough required = %v", required)
	}
	if !reflect.DeepEqual(metadata["properties"].(Schema)["turn_id"], Schema{"type": "string"}) {
		t.Fatalf("InternalChatMessageMetadataPassthrough.turn_id = %#v", metadata["properties"])
	}

	assertStringEnum(t, defs["LocalShellStatus"], "completed", "in_progress", "incomplete")
	action := defs["LocalShellAction"].(Schema)
	actionVariants, ok := action["oneOf"].([]any)
	if !ok || len(actionVariants) != 1 {
		t.Fatalf("LocalShellAction variants = %#v", action)
	}
	execAction := actionVariants[0].(Schema)
	if execAction["additionalProperties"] != false {
		t.Fatalf("LocalShellAction allows extra fields")
	}
	wantRequired := []string{"type", "command", "timeout_ms", "working_directory", "env", "user"}
	if !slices.Equal(schemaRequiredNames(execAction), wantRequired) {
		t.Fatalf("LocalShellAction required = %v, want %v", schemaRequiredNames(execAction), wantRequired)
	}
	properties := execAction["properties"].(Schema)
	if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{"exec"}) {
		t.Fatalf("LocalShellAction.type = %#v", properties["type"])
	}
	if !reflect.DeepEqual(properties["command"], Schema{"type": "array", "items": Schema{"type": "string"}}) {
		t.Fatalf("LocalShellAction.command = %#v", properties["command"])
	}
	assertNullableUnsignedIntegerSchema(t, properties["timeout_ms"])
	assertNullableStringSchema(t, properties["working_directory"])
	assertNullableStringSchema(t, properties["user"])
	assertNullableStringMapSchema(t, properties["env"])
}

func TestResponseItemContentPrerequisiteWireValidation(t *testing.T) {
	t.Run("ContentItem", func(t *testing.T) {
		valid := []string{
			`{"type":"input_text","text":""}`,
			`{"type":"input_text","text":"hello"}`,
			`{"type":"input_image","image_url":"https://example.test/image.png"}`,
			`{"type":"input_image","image_url":"data:image/png;base64,AA==","detail":"original"}`,
			`{"type":"output_text","text":"done"}`,
		}
		invalid := []string{
			`null`, `[]`, `{}`,
			`{"type":"input_text"}`,
			`{"type":"input_text","text":null}`,
			`{"type":"input_text","text":"hello","image_url":"crossed"}`,
			`{"type":"input_image"}`,
			`{"type":"input_image","image_url":null}`,
			`{"type":"input_image","image_url":"x","detail":null}`,
			`{"type":"input_image","image_url":"x","detail":"medium"}`,
			`{"type":"inputImage","image_url":"x"}`,
			`{"type":"output_text","text":"done","extra":true}`,
		}
		assertRawJSONRoundTrips[ContentItem](t, valid)
		assertRawJSONRejects[ContentItem](t, invalid)
		assertEmptyRawJSONMarshalRejects(t, ContentItem{})
		var value *ContentItem
		if err := value.UnmarshalJSON([]byte(valid[0])); err == nil {
			t.Fatal("nil ContentItem receiver succeeded")
		}
	})

	t.Run("FunctionCallOutputContentItem", func(t *testing.T) {
		valid := []string{
			`{"type":"input_text","text":"tool output"}`,
			`{"type":"input_image","image_url":"https://example.test/image.png"}`,
			`{"type":"input_image","image_url":"image.png","detail":"low"}`,
			`{"type":"encrypted_content","encrypted_content":"cipher"}`,
		}
		invalid := []string{
			`null`, `[]`, `{}`,
			`{"type":"input_text","text":null}`,
			`{"type":"input_image","image_url":"x","detail":null}`,
			`{"type":"input_image","image_url":"x","text":"crossed"}`,
			`{"type":"encrypted_content"}`,
			`{"type":"encrypted_content","encrypted_content":null}`,
			`{"type":"encrypted_content","encryptedContent":"cipher"}`,
			`{"type":"output_text","text":"unsupported"}`,
			`{"type":"input_text","text":"tool output","extra":true}`,
		}
		assertRawJSONRoundTrips[FunctionCallOutputContentItem](t, valid)
		assertRawJSONRejects[FunctionCallOutputContentItem](t, invalid)
		assertEmptyRawJSONMarshalRejects(t, FunctionCallOutputContentItem{})
		var value *FunctionCallOutputContentItem
		if err := value.UnmarshalJSON([]byte(valid[0])); err == nil {
			t.Fatal("nil FunctionCallOutputContentItem receiver succeeded")
		}
	})
}

func TestFunctionCallOutputBodyWireValidation(t *testing.T) {
	valid := []string{
		`"plain"`, `""`, `[]`,
		`[{"type":"input_text","text":"tool output"}]`,
		`[{"type":"input_image","image_url":"image.png","detail":"high"},{"type":"encrypted_content","encrypted_content":"cipher"}]`,
	}
	invalid := []string{
		`null`, `{}`, `1`, `true`,
		`[null]`, `["plain"]`,
		`[{"type":"input_image","image_url":"image.png","detail":null}]`,
		`[{"type":"output_text","text":"unsupported"}]`,
	}
	assertRawJSONRoundTrips[FunctionCallOutputBody](t, valid)
	assertRawJSONRejects[FunctionCallOutputBody](t, invalid)
	assertEmptyRawJSONMarshalRejects(t, FunctionCallOutputBody{})
	var value *FunctionCallOutputBody
	if err := value.UnmarshalJSON([]byte(`"plain"`)); err == nil {
		t.Fatal("nil FunctionCallOutputBody receiver succeeded")
	}
	for _, input := range []string{"", `"unterminated`, `[{"type":`} {
		if _, err := validateFunctionCallOutputBodyJSON([]byte(input)); err == nil {
			t.Errorf("validateFunctionCallOutputBodyJSON(%q) succeeded", input)
		}
	}
}

func TestInternalChatMessageMetadataPassthroughWireValidation(t *testing.T) {
	valid := []string{`{}`, `{"turn_id":"turn-1"}`, `{"turn_id":""}`}
	for _, input := range valid {
		var value InternalChatMessageMetadataPassthrough
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != input {
			t.Errorf("round trip %s = %s, %v", input, encoded, err)
		}
	}
	for _, input := range []string{`null`, `[]`, `{"turn_id":null}`, `{"turnId":"turn-1"}`, `{"turn_id":1}`, `{"extra":true}`} {
		assertJSONRejects[InternalChatMessageMetadataPassthrough](t, input)
	}
	var value *InternalChatMessageMetadataPassthrough
	if err := value.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("nil InternalChatMessageMetadataPassthrough receiver succeeded")
	}
}

func TestLocalShellPrerequisiteWireValidation(t *testing.T) {
	for _, input := range []string{`"completed"`, `"in_progress"`, `"incomplete"`} {
		assertJSONAccepts[LocalShellStatus](t, input)
	}
	for _, input := range []string{`null`, `"inProgress"`, `"failed"`, `1`} {
		assertJSONRejects[LocalShellStatus](t, input)
	}
	for _, status := range []LocalShellStatus{LocalShellStatusCompleted, LocalShellStatusInProgress, LocalShellStatusIncomplete} {
		if _, err := json.Marshal(status); err != nil {
			t.Errorf("Marshal(%q): %v", status, err)
		}
	}
	if _, err := json.Marshal(LocalShellStatus("failed")); err == nil {
		t.Fatal("Marshal invalid LocalShellStatus succeeded")
	}

	valid := []string{
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":["printf","ok"],"timeout_ms":0,"working_directory":"","env":{},"user":""}`,
		`{"type":"exec","command":["go","test","./..."],"timeout_ms":60000,"working_directory":"/workspace","env":{"A":"1","EMPTY":""},"user":"runner"}`,
	}
	invalid := []string{
		`null`, `[]`, `{}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null}`,
		`{"type":"exec","command":null,"timeout_ms":null,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":"go","timeout_ms":null,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":["go",null],"timeout_ms":null,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":["go",1],"timeout_ms":null,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":-1,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":1.5,"working_directory":null,"env":null,"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":1,"env":null,"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":[],"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":{"A":1},"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":{"A":null},"user":null}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":1}`,
		`{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null,"extra":true}`,
		`{"type":"shell","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}`,
	}
	assertRawJSONRoundTrips[LocalShellAction](t, valid)
	assertRawJSONRejects[LocalShellAction](t, invalid)
	assertEmptyRawJSONMarshalRejects(t, LocalShellAction{})
	var action *LocalShellAction
	if err := action.UnmarshalJSON([]byte(valid[0])); err == nil {
		t.Fatal("nil LocalShellAction receiver succeeded")
	}
	var status *LocalShellStatus
	if err := status.UnmarshalJSON([]byte(`"completed"`)); err == nil {
		t.Fatal("nil LocalShellStatus receiver succeeded")
	}
}

type contentItemSchemaExpectation struct {
	itemType       string
	field          string
	optionalDetail bool
}

func assertContentItemSchema(t *testing.T, defs Schema, name string, want []contentItemSchemaExpectation) {
	t.Helper()
	schema, ok := defs[name].(Schema)
	if !ok {
		t.Fatalf("$defs missing %s", name)
	}
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) != len(want) {
		t.Fatalf("%s variants = %#v", name, schema)
	}
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("%s %s allows extra fields", name, expected.itemType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.itemType}) {
			t.Fatalf("%s %s type = %#v", name, expected.itemType, properties["type"])
		}
		if !slices.Equal(schemaRequiredNames(variant), []string{"type", expected.field}) {
			t.Fatalf("%s %s required = %v", name, expected.itemType, schemaRequiredNames(variant))
		}
		if !reflect.DeepEqual(properties[expected.field], Schema{"type": "string"}) {
			t.Fatalf("%s %s field = %#v", name, expected.itemType, properties[expected.field])
		}
		if expected.optionalDetail {
			if !reflect.DeepEqual(properties["detail"], Schema{"$ref": "#/$defs/ImageDetail"}) {
				t.Fatalf("%s input_image detail = %#v", name, properties["detail"])
			}
		}
	}
}

func assertNullableUnsignedIntegerSchema(t *testing.T, raw any) {
	t.Helper()
	want := Schema{"anyOf": []any{Schema{"type": "integer", "minimum": 0}, Schema{"type": "null"}}}
	if !reflect.DeepEqual(raw, want) {
		t.Fatalf("nullable unsigned integer = %#v", raw)
	}
}

func assertNullableStringMapSchema(t *testing.T, raw any) {
	t.Helper()
	want := Schema{"anyOf": []any{
		Schema{"type": "object", "additionalProperties": Schema{"type": "string"}},
		Schema{"type": "null"},
	}}
	if !reflect.DeepEqual(raw, want) {
		t.Fatalf("nullable string map = %#v", raw)
	}
}

func assertRawJSONRoundTrips[T json.Marshaler](t *testing.T, inputs []string) {
	t.Helper()
	for _, input := range inputs {
		var value T
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != input {
			t.Errorf("round trip %s = %s, %v", input, encoded, err)
		}
	}
}

func assertRawJSONRejects[T any](t *testing.T, inputs []string) {
	t.Helper()
	for _, input := range inputs {
		var value T
		if err := json.Unmarshal([]byte(input), &value); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
}

func assertEmptyRawJSONMarshalRejects(t *testing.T, value any) {
	t.Helper()
	if _, err := json.Marshal(value); err == nil {
		t.Fatalf("Marshal(%T zero value) succeeded", value)
	}
}
