package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestThreadItemMessagePrerequisiteSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ByteRange",
		"HookPromptFragment",
		"ImageDetail",
		"MemoryCitation",
		"MemoryCitationEntry",
		"MessagePhase",
		"TextElement",
		"UserInput",
	} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("$defs missing %s", name)
		}
	}

	byteRange := defs["ByteRange"].(Schema)
	assertSchemaRequiredNames(t, byteRange, "start", "end")
	for _, name := range []string{"start", "end"} {
		property := byteRange["properties"].(Schema)[name].(Schema)
		if property["type"] != "integer" || property["minimum"] != 0 {
			t.Fatalf("ByteRange.%s = %#v", name, property)
		}
	}

	textElement := defs["TextElement"].(Schema)
	assertSchemaRequiredNames(t, textElement, "byteRange", "placeholder")
	textProperties := textElement["properties"].(Schema)
	if textProperties["byteRange"].(Schema)["$ref"] != "#/$defs/ByteRange" {
		t.Fatalf("TextElement.byteRange = %#v", textProperties["byteRange"])
	}
	assertNullableStringSchema(t, textProperties["placeholder"])

	assertStringEnum(t, defs["ImageDetail"], "auto", "low", "high", "original")
	assertStringEnum(t, defs["MessagePhase"], "commentary", "final_answer")

	entry := defs["MemoryCitationEntry"].(Schema)
	assertSchemaRequiredNames(t, entry, "path", "lineStart", "lineEnd", "note")
	for _, name := range []string{"lineStart", "lineEnd"} {
		property := entry["properties"].(Schema)[name].(Schema)
		if property["type"] != "integer" || property["minimum"] != 0 {
			t.Fatalf("MemoryCitationEntry.%s = %#v", name, property)
		}
	}

	citation := defs["MemoryCitation"].(Schema)
	assertSchemaRequiredNames(t, citation, "entries", "threadIds")
	for _, name := range []string{"entries", "threadIds"} {
		property := citation["properties"].(Schema)[name].(Schema)
		if property["type"] != "array" {
			t.Fatalf("MemoryCitation.%s = %#v", name, property)
		}
		if variants, ok := property["anyOf"]; ok {
			t.Fatalf("MemoryCitation.%s is nullable: %#v", name, variants)
		}
	}

	hook := defs["HookPromptFragment"].(Schema)
	assertSchemaRequiredNames(t, hook, "text", "hookRunId")

	input := defs["UserInput"].(Schema)
	variants, ok := input["oneOf"].([]any)
	if !ok || len(variants) != 5 {
		t.Fatalf("UserInput variants = %#v", input["oneOf"])
	}
	want := []struct {
		inputType string
		required  []string
	}{
		{inputType: "text", required: []string{"type", "text", "text_elements"}},
		{inputType: "image", required: []string{"type", "url"}},
		{inputType: "localImage", required: []string{"type", "path"}},
		{inputType: "skill", required: []string{"type", "name", "path"}},
		{inputType: "mention", required: []string{"type", "name", "path"}},
	}
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("UserInput %s allows extra fields", expected.inputType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.inputType}) {
			t.Fatalf("UserInput variant %d type = %#v", index, properties["type"])
		}
		if !slices.Equal(schemaRequiredNames(variant), expected.required) {
			t.Fatalf("UserInput %s required = %v, want %v", expected.inputType, schemaRequiredNames(variant), expected.required)
		}
		if detail, exists := properties["detail"]; exists {
			detailSchema := detail.(Schema)
			if detailSchema["$ref"] != "#/$defs/ImageDetail" || slices.Contains(schemaRequiredNames(variant), "detail") {
				t.Fatalf("UserInput %s detail = %#v", expected.inputType, detailSchema)
			}
		}
	}
}

func TestThreadItemMessagePrerequisiteWireValidation(t *testing.T) {
	t.Run("ByteRange", func(t *testing.T) {
		assertJSONAccepts[ByteRange](t, `{"start":0,"end":0}`)
		assertJSONAccepts[ByteRange](t, `{"start":9,"end":2}`)
		for _, input := range []string{
			`{}`, `{"start":0}`, `{"end":0}`, `{"start":-1,"end":0}`,
			`{"start":0,"end":-1}`, `{"start":0,"end":0,"extra":true}`,
		} {
			assertJSONRejects[ByteRange](t, input)
		}
	})

	t.Run("TextElement", func(t *testing.T) {
		assertJSONAccepts[TextElement](t, `{"byteRange":{"start":0,"end":4},"placeholder":null}`)
		assertJSONAccepts[TextElement](t, `{"byteRange":{"start":0,"end":4},"placeholder":"@file"}`)
		for _, input := range []string{
			`{"placeholder":null}`, `{"byteRange":{"start":0,"end":4}}`,
			`{"byteRange":{"start":0,"end":4},"placeholder":1}`,
			`{"byteRange":{"start":0,"end":4},"placeholder":null,"extra":true}`,
		} {
			assertJSONRejects[TextElement](t, input)
		}
	})

	t.Run("Enums", func(t *testing.T) {
		for _, input := range []string{`"auto"`, `"low"`, `"high"`, `"original"`} {
			assertJSONAccepts[ImageDetail](t, input)
		}
		for _, input := range []string{`null`, `"medium"`, `1`} {
			assertJSONRejects[ImageDetail](t, input)
		}
		for _, input := range []string{`"commentary"`, `"final_answer"`} {
			assertJSONAccepts[MessagePhase](t, input)
		}
		for _, input := range []string{`null`, `"finalAnswer"`, `"analysis"`} {
			assertJSONRejects[MessagePhase](t, input)
		}
	})

	t.Run("MemoryCitation", func(t *testing.T) {
		assertJSONAccepts[MemoryCitation](t, `{"entries":[],"threadIds":[]}`)
		assertJSONAccepts[MemoryCitation](t, `{"entries":[{"path":"relative.md","lineStart":9,"lineEnd":2,"note":"context"}],"threadIds":["thread-1"]}`)
		for _, input := range []string{
			`{}`, `{"entries":[],"threadIds":null}`, `{"entries":null,"threadIds":[]}`,
			`{"entries":[{"path":"x","lineStart":-1,"lineEnd":0,"note":""}],"threadIds":[]}`,
			`{"entries":[{"path":"x","lineStart":0,"lineEnd":0}],"threadIds":[]}`,
			`{"entries":[],"threadIds":[],"extra":true}`,
		} {
			assertJSONRejects[MemoryCitation](t, input)
		}
	})

	t.Run("HookPromptFragment", func(t *testing.T) {
		assertJSONAccepts[HookPromptFragment](t, `{"text":"review","hookRunId":"hook-1"}`)
		for _, input := range []string{
			`{}`, `{"text":"review"}`, `{"hookRunId":"hook-1"}`,
			`{"text":"review","hook_run_id":"hook-1"}`,
			`{"text":"review","hookRunId":"hook-1","extra":true}`,
		} {
			assertJSONRejects[HookPromptFragment](t, input)
		}
	})

	t.Run("UserInput", func(t *testing.T) {
		valid := []string{
			`{"type":"text","text":"hello","text_elements":[]}`,
			`{"type":"text","text":"@file","text_elements":[{"byteRange":{"start":0,"end":5},"placeholder":null}]}`,
			`{"type":"image","url":"https://example.test/image.png"}`,
			`{"type":"image","detail":"original","url":"data:image/png;base64,AA=="}`,
			`{"type":"localImage","path":"relative.png"}`,
			`{"type":"localImage","detail":"low","path":"/tmp/image.png"}`,
			`{"type":"skill","name":"review","path":"skills/review/SKILL.md"}`,
			`{"type":"mention","name":"guide","path":"docs/guide.md"}`,
		}
		for _, input := range valid {
			assertJSONAccepts[UserInput](t, input)
		}
		invalid := []string{
			`{}`, `{"type":"audio","url":"x"}`,
			`{"type":"text","text":"hello"}`,
			`{"type":"text","text":"hello","text_elements":null}`,
			`{"type":"text","text":"hello","textElements":[]}`,
			`{"type":"text","text":"hello","text_elements":[],"url":"x"}`,
			`{"type":"image","url":"x","detail":null}`,
			`{"type":"image","url":"x","detail":"medium"}`,
			`{"type":"image","path":"x"}`,
			`{"type":"localImage","url":"x"}`,
			`{"type":"skill","name":"review"}`,
			`{"type":"mention","name":"guide","path":"docs/guide.md","extra":true}`,
		}
		for _, input := range invalid {
			assertJSONRejects[UserInput](t, input)
		}
	})
}

func TestThreadItemMessagePrerequisiteMarshalIsCanonical(t *testing.T) {
	placeholder := "@file"
	detail := ImageDetailOriginal
	cases := []struct {
		value any
		want  string
	}{
		{ByteRange{Start: 9, End: 2}, `{"start":9,"end":2}`},
		{TextElement{ByteRange: ByteRange{Start: 0, End: 5}}, `{"byteRange":{"start":0,"end":5},"placeholder":null}`},
		{TextElement{ByteRange: ByteRange{Start: 0, End: 5}, Placeholder: &placeholder}, `{"byteRange":{"start":0,"end":5},"placeholder":"@file"}`},
		{MemoryCitation{}, `{"entries":[],"threadIds":[]}`},
		{HookPromptFragment{Text: "review", HookRunID: "hook-1"}, `{"text":"review","hookRunId":"hook-1"}`},
		{UserInput{Type: "text", Text: "hello"}, `{"type":"text","text":"hello","text_elements":[]}`},
		{UserInput{Type: "image", Detail: &detail, URL: "image.png"}, `{"type":"image","detail":"original","url":"image.png"}`},
	}
	for _, testCase := range cases {
		encoded, err := json.Marshal(testCase.value)
		if err != nil {
			t.Fatalf("Marshal(%#v): %v", testCase.value, err)
		}
		if string(encoded) != testCase.want {
			t.Fatalf("Marshal(%#v) = %s, want %s", testCase.value, encoded, testCase.want)
		}
	}

	invalidDetail := ImageDetail("medium")
	for _, value := range []any{
		ImageDetail("medium"),
		MessagePhase("analysis"),
		UserInput{Type: "image", Detail: &invalidDetail, URL: "image.png"},
		UserInput{Type: "audio", URL: "audio.wav"},
		UserInput{Type: "text", Text: "hello", URL: "crossed"},
	} {
		if _, err := json.Marshal(value); err == nil {
			t.Errorf("Marshal(%#v) succeeded", value)
		}
	}
}

func TestThreadItemMessagePrerequisiteNilReceiversRejectJSON(t *testing.T) {
	var byteRange *ByteRange
	var textElement *TextElement
	var imageDetail *ImageDetail
	var messagePhase *MessagePhase
	var entry *MemoryCitationEntry
	var citation *MemoryCitation
	var hook *HookPromptFragment
	var input *UserInput
	for name, decode := range map[string]func() error{
		"ByteRange": func() error { return byteRange.UnmarshalJSON([]byte(`{"start":0,"end":0}`)) },
		"TextElement": func() error {
			return textElement.UnmarshalJSON([]byte(`{"byteRange":{"start":0,"end":0},"placeholder":null}`))
		},
		"ImageDetail":         func() error { return imageDetail.UnmarshalJSON([]byte(`"auto"`)) },
		"MessagePhase":        func() error { return messagePhase.UnmarshalJSON([]byte(`"commentary"`)) },
		"MemoryCitationEntry": func() error { return entry.UnmarshalJSON([]byte(`{"path":"x","lineStart":0,"lineEnd":0,"note":""}`)) },
		"MemoryCitation":      func() error { return citation.UnmarshalJSON([]byte(`{"entries":[],"threadIds":[]}`)) },
		"HookPromptFragment":  func() error { return hook.UnmarshalJSON([]byte(`{"text":"x","hookRunId":"h"}`)) },
		"UserInput":           func() error { return input.UnmarshalJSON([]byte(`{"type":"text","text":"x","text_elements":[]}`)) },
	} {
		if err := decode(); err == nil {
			t.Errorf("%s nil receiver succeeded", name)
		}
	}
}

func assertJSONAccepts[T any](t *testing.T, input string) {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(input), &value); err != nil {
		t.Errorf("Unmarshal %T from %s: %v", value, input, err)
	}
}

func assertJSONRejects[T any](t *testing.T, input string) {
	t.Helper()
	var value T
	if err := json.Unmarshal([]byte(input), &value); err == nil {
		t.Errorf("Unmarshal %T from %s succeeded: %#v", value, input, value)
	}
}

func assertNullableStringSchema(t *testing.T, value any) {
	t.Helper()
	schema, ok := value.(Schema)
	if !ok {
		t.Fatalf("nullable string schema = %#v", value)
	}
	variants, ok := schema["anyOf"].([]any)
	if !ok || len(variants) != 2 || variants[0].(Schema)["type"] != "string" || variants[1].(Schema)["type"] != "null" {
		t.Fatalf("nullable string schema = %#v", schema)
	}
}
