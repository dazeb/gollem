package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestResponsesAPIWebSearchActionSchemaIsExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	action, ok := defs["ResponsesApiWebSearchAction"].(Schema)
	if !ok {
		t.Fatal("$defs missing ResponsesApiWebSearchAction")
	}
	variants, ok := action["oneOf"].([]any)
	if !ok || len(variants) != 4 {
		t.Fatalf("ResponsesApiWebSearchAction variants = %#v", action["oneOf"])
	}
	want := []struct {
		actionType string
		fields     []string
	}{
		{actionType: "search", fields: []string{"query", "queries"}},
		{actionType: "open_page", fields: []string{"url"}},
		{actionType: "find_in_page", fields: []string{"url", "pattern"}},
		{actionType: "other"},
	}
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("ResponsesApiWebSearchAction %s allows extra fields", expected.actionType)
		}
		if !slices.Equal(schemaRequiredNames(variant), []string{"type"}) {
			t.Fatalf("ResponsesApiWebSearchAction %s required = %v", expected.actionType, schemaRequiredNames(variant))
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.actionType}) {
			t.Fatalf("ResponsesApiWebSearchAction variant %d type = %#v", index, properties["type"])
		}
		for _, field := range expected.fields {
			if field == "queries" {
				wantSchema := Schema{"type": "array", "items": Schema{"type": "string"}}
				if !reflect.DeepEqual(properties[field], wantSchema) {
					t.Fatalf("ResponsesApiWebSearchAction %s %s = %#v", expected.actionType, field, properties[field])
				}
				continue
			}
			if !reflect.DeepEqual(properties[field], Schema{"type": "string"}) {
				t.Fatalf("ResponsesApiWebSearchAction %s %s = %#v", expected.actionType, field, properties[field])
			}
		}
	}
}

func TestResponsesAPIWebSearchActionWireValidation(t *testing.T) {
	valid := []string{
		`{"type":"search"}`,
		`{"type":"search","query":"gollem"}`,
		`{"type":"search","queries":[]}`,
		`{"type":"search","query":"gollem","queries":["gollem","slang"]}`,
		`{"type":"open_page"}`,
		`{"type":"open_page","url":"https://example.com"}`,
		`{"type":"find_in_page"}`,
		`{"type":"find_in_page","url":"https://example.com","pattern":"needle"}`,
		`{"type":"other"}`,
	}
	for _, input := range valid {
		var action ResponsesApiWebSearchAction
		if err := json.Unmarshal([]byte(input), &action); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(action)
		if err != nil || string(encoded) != input {
			t.Errorf("round trip %s = %s, %v", input, encoded, err)
		}
	}
	for _, input := range []string{
		`null`, `[]`, `{}`,
		`{"type":"search","query":null}`,
		`{"type":"search","queries":null}`,
		`{"type":"search","query":1}`,
		`{"type":"search","queries":"gollem"}`,
		`{"type":"search","queries":["gollem",null]}`,
		`{"type":"search","queries":["gollem",1]}`,
		`{"type":"search","url":"https://example.com"}`,
		`{"type":"open_page","url":null}`,
		`{"type":"open_page","query":"gollem"}`,
		`{"type":"find_in_page","url":null}`,
		`{"type":"find_in_page","pattern":null}`,
		`{"type":"find_in_page","queries":[]}`,
		`{"type":"other","url":"https://example.com"}`,
		`{"type":"openPage","url":"https://example.com"}`,
		`{"type":"unknown"}`,
		`{"type":1}`,
		`{"type":"other","extra":true}`,
	} {
		var action ResponsesApiWebSearchAction
		if err := json.Unmarshal([]byte(input), &action); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	if _, err := json.Marshal(ResponsesApiWebSearchAction{}); err == nil {
		t.Fatal("Marshal empty ResponsesApiWebSearchAction succeeded")
	}
	var nilAction *ResponsesApiWebSearchAction
	if err := nilAction.UnmarshalJSON([]byte(`{"type":"other"}`)); err == nil {
		t.Fatal("nil ResponsesApiWebSearchAction receiver succeeded")
	}
}
