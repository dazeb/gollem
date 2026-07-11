package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestWebSearchItemSchemaIsExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	item, ok := defs["WebSearchItem"].(Schema)
	if !ok {
		t.Fatal("$defs missing WebSearchItem")
	}
	if item["additionalProperties"] != false {
		t.Fatalf("WebSearchItem allows extra fields: %#v", item)
	}
	if !slices.Equal(schemaRequiredNames(item), []string{"id", "query", "action"}) {
		t.Fatalf("WebSearchItem required = %v", schemaRequiredNames(item))
	}
	properties := item["properties"].(Schema)
	if !reflect.DeepEqual(properties["id"], Schema{"type": "string"}) {
		t.Fatalf("WebSearchItem id = %#v", properties["id"])
	}
	if !reflect.DeepEqual(properties["query"], Schema{"type": "string"}) {
		t.Fatalf("WebSearchItem query = %#v", properties["query"])
	}
	assertNullableSchemaRef(t, properties["action"], "#/$defs/WebSearchAction")
}

func TestWebSearchItemWireValidationAndCanonicalization(t *testing.T) {
	valid := []struct {
		input string
		want  string
	}{
		{
			input: `{"id":"","query":"","action":null}`,
			want:  `{"id":"","query":"","action":null}`,
		},
		{
			input: `{"query":"gollem","action":{"type":"search","queries":["gollem","slang"],"query":"gollem"},"id":"search-1"}`,
			want:  `{"id":"search-1","query":"gollem","action":{"type":"search","query":"gollem","queries":["gollem","slang"]}}`,
		},
		{
			input: `{"id":"search-2","query":"https://example.com","action":{"type":"openPage","url":null}}`,
			want:  `{"id":"search-2","query":"https://example.com","action":{"type":"openPage","url":null}}`,
		},
		{
			input: `{"id":"search-3","query":"needle","action":{"type":"findInPage","url":"https://example.com","pattern":"needle"}}`,
			want:  `{"id":"search-3","query":"needle","action":{"type":"findInPage","url":"https://example.com","pattern":"needle"}}`,
		},
		{
			input: `{"id":"search-4","query":"custom","action":{"type":"other"}}`,
			want:  `{"id":"search-4","query":"custom","action":{"type":"other"}}`,
		},
	}
	for _, testCase := range valid {
		var item WebSearchItem
		if err := json.Unmarshal([]byte(testCase.input), &item); err != nil {
			t.Errorf("Unmarshal(%s): %v", testCase.input, err)
			continue
		}
		encoded, err := json.Marshal(item)
		if err != nil || string(encoded) != testCase.want {
			t.Errorf("round trip %s = %s, %v; want %s", testCase.input, encoded, err, testCase.want)
		}
	}

	invalid := []string{
		`null`, `[]`, `{}`,
		`{"query":"q","action":null}`,
		`{"id":"id","action":null}`,
		`{"id":"id","query":"q"}`,
		`{"id":null,"query":"q","action":null}`,
		`{"id":"id","query":null,"action":null}`,
		`{"id":1,"query":"q","action":null}`,
		`{"id":"id","query":1,"action":null}`,
		`{"id":"id","query":"q","action":1}`,
		`{"id":"id","query":"q","action":{"type":"search","query":null}}`,
		`{"id":"id","query":"q","action":{"type":"open_page","url":null}}`,
		`{"id":"id","query":"q","action":null,"extra":true}`,
		`{"id":"id","query":"q","Action":null}`,
	}
	for _, input := range invalid {
		var item WebSearchItem
		if err := json.Unmarshal([]byte(input), &item); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
}

func TestWebSearchItemInvalidActionAndNilReceiverFailClosed(t *testing.T) {
	if _, err := json.Marshal(WebSearchItem{Action: &WebSearchAction{}}); err == nil {
		t.Fatal("Marshal WebSearchItem with empty action succeeded")
	}
	var item *WebSearchItem
	if err := item.UnmarshalJSON([]byte(`{"id":"id","query":"q","action":null}`)); err == nil {
		t.Fatal("nil WebSearchItem receiver succeeded")
	}
}
