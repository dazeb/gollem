package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDynamicToolCallSchemaAndBindingAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{"DynamicToolCallParams", "DynamicToolCallResponse", "DynamicToolCallOutputContentItem"} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("$defs missing %s", name)
		}
	}
	params := defs["DynamicToolCallParams"].(Schema)
	for _, name := range []string{"threadId", "turnId", "callId", "namespace", "tool", "arguments"} {
		assertSchemaRequired(t, params, name)
	}
	response := defs["DynamicToolCallResponse"].(Schema)
	assertSchemaRequired(t, response, "contentItems")
	assertSchemaRequired(t, response, "success")

	content := defs["DynamicToolCallOutputContentItem"].(Schema)
	variants, ok := content["oneOf"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("content variants = %#v", content["oneOf"])
	}
	for index, want := range []struct {
		contentType string
		field       string
	}{{"inputText", "text"}, {"inputImage", "imageUrl"}} {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("variant %s allows extra fields", want.contentType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{want.contentType}) {
			t.Fatalf("variant %d type = %#v", index, properties["type"])
		}
		assertSchemaRequired(t, variant, want.field)
	}

	bindings := WireTypeBindings()
	assertBinding(t, bindings, "item/tool/call", SurfaceServerRequest, "DynamicToolCallParams")
	assertBinding(t, bindings, "item/tool/call", SurfaceServerRequest, "DynamicToolCallResponse")
}

func TestDynamicToolCallResponseWireValidation(t *testing.T) {
	valid := []string{
		`{"contentItems":[],"success":false}`,
		`{"contentItems":[{"type":"inputText","text":""}],"success":true}`,
		`{"contentItems":[{"type":"inputImage","imageUrl":"data:image/png;base64,AA=="}],"success":true}`,
	}
	for _, input := range valid {
		var response DynamicToolCallResponse
		if err := json.Unmarshal([]byte(input), &response); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
		}
	}

	invalid := []string{
		`[`,
		`{}`,
		`{"contentItems":null,"success":true}`,
		`{"contentItems":{},"success":true}`,
		`{"contentItems":[],"success":null}`,
		`{"contentItems":[{"type":1,"text":"bad"}],"success":true}`,
		`{"contentItems":[{"type":"inputText"}],"success":true}`,
		`{"contentItems":[{"type":"inputText","text":1}],"success":true}`,
		`{"contentItems":[{"type":"inputText","text":"ok","imageUrl":"bad"}],"success":true}`,
		`{"contentItems":[{"type":"inputImage"}],"success":true}`,
		`{"contentItems":[{"type":"video","url":"bad"}],"success":true}`,
		`{"contentItems":[{"type":"inputText","text":"ok","extra":true}],"success":true}`,
		`{"contentItems":[],"success":true,"extra":true}`,
	}
	for _, input := range invalid {
		var response DynamicToolCallResponse
		if err := json.Unmarshal([]byte(input), &response); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var response *DynamicToolCallResponse
	if err := response.UnmarshalJSON([]byte(`{"contentItems":[],"success":true}`)); err == nil {
		t.Error("nil response receiver succeeded")
	}
	var item *DynamicToolCallOutputContentItem
	if err := item.UnmarshalJSON([]byte(`{"type":"inputText","text":"ok"}`)); err == nil {
		t.Error("nil content receiver succeeded")
	}
}

func TestDynamicToolCallResponseMarshalUsesPublicVariants(t *testing.T) {
	response := DynamicToolCallResponse{ContentItems: []DynamicToolCallOutputContentItem{
		{Type: "inputText", Text: "match"},
		{Type: "inputImage", ImageURL: "data:image/png;base64,AA=="},
	}, Success: true}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"type":"inputText","text":"match"`, `"type":"inputImage","imageUrl":"data:image/png;base64,AA=="`, `"success":true`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("response = %s, want %s", encoded, want)
		}
	}
	empty, err := json.Marshal(DynamicToolCallResponse{})
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	if string(empty) != `{"contentItems":[],"success":false}` {
		t.Fatalf("empty response = %s", empty)
	}
	invalid := []DynamicToolCallOutputContentItem{
		{Type: "inputText", Text: "ok", ImageURL: "bad"},
		{Type: "inputImage", ImageURL: "image", Text: "bad"},
		{Type: "video"},
	}
	for _, item := range invalid {
		if _, err := json.Marshal(item); err == nil {
			t.Errorf("Marshal(%#v) succeeded", item)
		}
	}
}
