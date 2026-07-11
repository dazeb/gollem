package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestWebSearchActionSchemaIsExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	action, ok := defs["WebSearchAction"].(Schema)
	if !ok {
		t.Fatal("$defs missing WebSearchAction")
	}
	variants, ok := action["oneOf"].([]any)
	if !ok || len(variants) != 4 {
		t.Fatalf("WebSearchAction variants = %#v", action["oneOf"])
	}
	want := []struct {
		actionType string
		required   []string
	}{
		{actionType: "search", required: []string{"type", "query", "queries"}},
		{actionType: "openPage", required: []string{"type", "url"}},
		{actionType: "findInPage", required: []string{"type", "url", "pattern"}},
		{actionType: "other", required: []string{"type"}},
	}
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("WebSearchAction %s allows extra fields", expected.actionType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.actionType}) {
			t.Fatalf("WebSearchAction variant %d type = %#v", index, properties["type"])
		}
		if !slices.Equal(schemaRequiredNames(variant), expected.required) {
			t.Fatalf("WebSearchAction %s required = %v, want %v", expected.actionType, schemaRequiredNames(variant), expected.required)
		}
	}

	searchProperties := variants[0].(Schema)["properties"].(Schema)
	assertNullableStringSchema(t, searchProperties["query"])
	assertNullableStringArraySchema(t, searchProperties["queries"])
	openProperties := variants[1].(Schema)["properties"].(Schema)
	assertNullableStringSchema(t, openProperties["url"])
	findProperties := variants[2].(Schema)["properties"].(Schema)
	assertNullableStringSchema(t, findProperties["url"])
	assertNullableStringSchema(t, findProperties["pattern"])
}

func TestWebSearchActionWireValidationAndCanonicalization(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{input: `{"type":"search","query":null,"queries":null}`, want: `{"type":"search","query":null,"queries":null}`},
		{input: `{"type":"search","query":"gollem","queries":["gollem","slang"]}`, want: `{"type":"search","query":"gollem","queries":["gollem","slang"]}`},
		{input: `{"type":"search","query":"","queries":[]}`, want: `{"type":"search","query":"","queries":[]}`},
		{input: `{"type":"openPage","url":null}`, want: `{"type":"openPage","url":null}`},
		{input: `{"type":"openPage","url":"https://example.com"}`, want: `{"type":"openPage","url":"https://example.com"}`},
		{input: `{"type":"findInPage","url":null,"pattern":null}`, want: `{"type":"findInPage","url":null,"pattern":null}`},
		{input: `{"type":"findInPage","url":"https://example.com","pattern":"needle"}`, want: `{"type":"findInPage","url":"https://example.com","pattern":"needle"}`},
		{input: `{"type":"other"}`, want: `{"type":"other"}`},
	}
	for _, testCase := range valid {
		var action WebSearchAction
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
		`{"type":"search","queries":null}`,
		`{"type":"search","query":null}`,
		`{"type":"search","query":1,"queries":null}`,
		`{"type":"search","query":null,"queries":"gollem"}`,
		`{"type":"search","query":null,"queries":["gollem",null]}`,
		`{"type":"search","query":null,"queries":[1]}`,
		`{"type":"search","query":null,"queries":null,"url":null}`,
		`{"type":"openPage"}`,
		`{"type":"openPage","url":1}`,
		`{"type":"openPage","url":null,"query":null}`,
		`{"type":"findInPage","pattern":null}`,
		`{"type":"findInPage","url":null}`,
		`{"type":"findInPage","url":false,"pattern":null}`,
		`{"type":"findInPage","url":null,"pattern":[]}`,
		`{"type":"findInPage","url":null,"pattern":null,"query":null}`,
		`{"type":"other","url":null}`,
		`{"type":"search_web","query":null,"queries":null}`,
		`{"type":1,"query":null,"queries":null}`,
		`{"type":"other","extra":true}`,
	}
	for _, input := range invalid {
		var action WebSearchAction
		if err := json.Unmarshal([]byte(input), &action); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
}

func TestWebSearchActionEmptyAndNilReceiversFailClosed(t *testing.T) {
	if _, err := json.Marshal(WebSearchAction{}); err == nil {
		t.Fatal("Marshal empty WebSearchAction succeeded")
	}
	var action *WebSearchAction
	if err := action.UnmarshalJSON([]byte(`{"type":"other"}`)); err == nil {
		t.Fatal("nil WebSearchAction receiver succeeded")
	}
}

func assertNullableStringArraySchema(t *testing.T, value any) {
	t.Helper()
	want := Schema{"oneOf": []any{
		Schema{"type": "array", "items": Schema{"type": "string"}},
		Schema{"type": "null"},
	}}
	if !reflect.DeepEqual(value, want) {
		t.Fatalf("nullable string array schema = %#v, want %#v", value, want)
	}
}
