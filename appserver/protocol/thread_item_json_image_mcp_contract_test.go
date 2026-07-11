package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestThreadItemJSONImageMCPPrerequisiteSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)

	jsonValue := defs["JsonValue"].(Schema)
	variants := jsonValue["anyOf"].([]any)
	if len(variants) != 6 {
		t.Fatalf("JsonValue variants = %#v", variants)
	}
	for index, want := range []Schema{
		{"type": "number"},
		{"type": "string"},
		{"type": "boolean"},
		{"type": "array", "items": Schema{"$ref": "#/$defs/JsonValue"}},
		{
			"type": "object", "additionalProperties": Schema{"$ref": "#/$defs/JsonValue"},
			"x-gollem-typescript-recursive-map": true,
		},
		{"type": "null"},
	} {
		if !reflect.DeepEqual(variants[index], want) {
			t.Fatalf("JsonValue variant %d = %#v, want %#v", index, variants[index], want)
		}
	}

	image := defs["ImageGenerationItem"].(Schema)
	if !slices.Equal(schemaRequiredNames(image), []string{"id", "status", "revisedPrompt", "result"}) {
		t.Fatalf("ImageGenerationItem required = %v", schemaRequiredNames(image))
	}
	imageProperties := image["properties"].(Schema)
	assertNullableStringSchema(t, imageProperties["revisedPrompt"])
	if !reflect.DeepEqual(imageProperties["savedPath"], Schema{"$ref": "#/$defs/AbsolutePathBuf"}) {
		t.Fatalf("ImageGenerationItem.savedPath = %#v", imageProperties["savedPath"])
	}

	result := defs["McpToolCallResult"].(Schema)
	if !slices.Equal(schemaRequiredNames(result), []string{"content", "structuredContent", "_meta"}) {
		t.Fatalf("McpToolCallResult required = %v", schemaRequiredNames(result))
	}
	resultProperties := result["properties"].(Schema)
	if !reflect.DeepEqual(resultProperties["content"], Schema{
		"type": "array", "items": Schema{"$ref": "#/$defs/JsonValue"},
	}) {
		t.Fatalf("McpToolCallResult.content = %#v", resultProperties["content"])
	}
	for _, name := range []string{"structuredContent", "_meta"} {
		want := Schema{"anyOf": []any{
			Schema{"$ref": "#/$defs/JsonValue"},
			Schema{"type": "null"},
		}}
		if !reflect.DeepEqual(resultProperties[name], want) {
			t.Fatalf("McpToolCallResult.%s = %#v", name, resultProperties[name])
		}
	}
}

func TestJsonValueWireValidationAndPrecision(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{input: `null`, want: `null`},
		{input: `true`, want: `true`},
		{input: `"text"`, want: `"text"`},
		{input: `18446744073709551616`, want: `18446744073709551616`},
		{input: `[null,false,1,"x",{"z":1,"a":2}]`, want: `[null,false,1,"x",{"a":2,"z":1}]`},
		{input: ` { "nested": [1e100, {"ok": true}] } `, want: `{"nested":[1e100,{"ok":true}]}`},
	}
	for _, testCase := range valid {
		var value JsonValue
		if err := json.Unmarshal([]byte(testCase.input), &value); err != nil {
			t.Fatalf("Unmarshal(%s): %v", testCase.input, err)
		}
		encoded, err := json.Marshal(value)
		if err != nil || string(encoded) != testCase.want {
			t.Fatalf("round trip %s = %s, %v; want %s", testCase.input, encoded, err, testCase.want)
		}
	}
	for _, input := range []string{"", `undefined`, `1 2`, `{"broken":}`} {
		var value JsonValue
		if err := value.UnmarshalJSON([]byte(input)); err == nil {
			t.Errorf("UnmarshalJSON(%q) succeeded", input)
		}
	}
	if _, err := json.Marshal(JsonValue{}); err == nil {
		t.Fatal("Marshal empty JsonValue succeeded")
	}
	var nilValue *JsonValue
	if err := nilValue.UnmarshalJSON([]byte(`null`)); err == nil {
		t.Fatal("nil JsonValue receiver succeeded")
	}
}

func TestImageGenerationItemWireValidation(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{
			input: `{"id":"image-1","status":"completed","revisedPrompt":null,"result":"base64"}`,
			want:  `{"id":"image-1","status":"completed","revisedPrompt":null,"result":"base64"}`,
		},
		{
			input: `{"id":"","status":"","revisedPrompt":"draw","result":"","savedPath":"/workspace/../workspace/image.png"}`,
			want:  `{"id":"","status":"","revisedPrompt":"draw","result":"","savedPath":"/workspace/image.png"}`,
		},
	}
	for _, testCase := range valid {
		var item ImageGenerationItem
		if err := json.Unmarshal([]byte(testCase.input), &item); err != nil {
			t.Fatalf("Unmarshal(%s): %v", testCase.input, err)
		}
		encoded, err := json.Marshal(item)
		if err != nil || string(encoded) != testCase.want {
			t.Fatalf("round trip %s = %s, %v; want %s", testCase.input, encoded, err, testCase.want)
		}
	}
	invalid := []string{
		`null`, `{}`,
		`{"status":"completed","revisedPrompt":null,"result":"x"}`,
		`{"id":"i","revisedPrompt":null,"result":"x"}`,
		`{"id":"i","status":"completed","result":"x"}`,
		`{"id":"i","status":"completed","revisedPrompt":null}`,
		`{"id":null,"status":"completed","revisedPrompt":null,"result":"x"}`,
		`{"id":"i","status":null,"revisedPrompt":null,"result":"x"}`,
		`{"id":"i","status":"completed","revisedPrompt":1,"result":"x"}`,
		`{"id":"i","status":"completed","revisedPrompt":null,"result":null}`,
		`{"id":"i","status":"completed","revisedPrompt":null,"result":"x","savedPath":null}`,
		`{"id":"i","status":"completed","revisedPrompt":null,"result":"x","savedPath":"relative.png"}`,
		`{"id":"i","status":"completed","revisedPrompt":null,"result":"x","extra":true}`,
	}
	for _, input := range invalid {
		var item ImageGenerationItem
		if err := json.Unmarshal([]byte(input), &item); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var nilItem *ImageGenerationItem
	if err := nilItem.UnmarshalJSON([]byte(valid[0].input)); err == nil {
		t.Fatal("nil ImageGenerationItem receiver succeeded")
	}
	invalidPath := AbsolutePathBuf("relative.png")
	if _, err := json.Marshal(ImageGenerationItem{
		ID: "image-1", Status: "completed", Result: "base64", SavedPath: &invalidPath,
	}); err == nil {
		t.Fatal("Marshal image item with relative savedPath succeeded")
	}
}

func TestMcpToolCallResultWireValidation(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{input: `{"content":[],"structuredContent":null,"_meta":null}`, want: `{"content":[],"structuredContent":null,"_meta":null}`},
		{
			input: `{"content":[null,"text",18446744073709551616,{"z":1,"a":2}],"structuredContent":{"ok":true},"_meta":[1,false]}`,
			want:  `{"content":[null,"text",18446744073709551616,{"a":2,"z":1}],"structuredContent":{"ok":true},"_meta":[1,false]}`,
		},
	}
	for _, testCase := range valid {
		var result McpToolCallResult
		if err := json.Unmarshal([]byte(testCase.input), &result); err != nil {
			t.Fatalf("Unmarshal(%s): %v", testCase.input, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil || string(encoded) != testCase.want {
			t.Fatalf("round trip %s = %s, %v; want %s", testCase.input, encoded, err, testCase.want)
		}
	}
	for _, input := range []string{
		`null`, `{}`,
		`{"structuredContent":null,"_meta":null}`,
		`{"content":null,"structuredContent":null,"_meta":null}`,
		`{"content":{},"structuredContent":null,"_meta":null}`,
		`{"content":[] ,"_meta":null}`,
		`{"content":[],"structuredContent":null}`,
		`{"content":[],"structuredContent":null,"_meta":null,"extra":true}`,
	} {
		var result McpToolCallResult
		if err := json.Unmarshal([]byte(input), &result); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	encoded, err := json.Marshal(McpToolCallResult{})
	if err != nil || string(encoded) != `{"content":[],"structuredContent":null,"_meta":null}` {
		t.Fatalf("Marshal zero result = %s, %v", encoded, err)
	}
	var nilResult *McpToolCallResult
	if err := nilResult.UnmarshalJSON([]byte(valid[0].input)); err == nil {
		t.Fatal("nil McpToolCallResult receiver succeeded")
	}
	if _, err := json.Marshal(McpToolCallResult{Content: []JsonValue{{}}}); err == nil {
		t.Fatal("Marshal MCP result with empty JsonValue succeeded")
	}
}
