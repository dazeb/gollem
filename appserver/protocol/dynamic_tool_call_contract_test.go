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
	if params["title"] != "DynamicToolCallParams" || params["properties"].(Schema)["arguments"] != true ||
		!reflect.DeepEqual(params["properties"].(Schema)["namespace"].(Schema)["type"], []any{"string", "null"}) {
		t.Fatalf("DynamicToolCallParams source schema = %#v", params)
	}
	for _, name := range []string{"threadId", "turnId", "callId", "tool", "arguments"} {
		assertSchemaRequired(t, params, name)
	}
	if _, required := params["additionalProperties"]; required {
		t.Fatalf("DynamicToolCallParams unexpectedly closes the serde-open record: %#v", params)
	}
	response := defs["DynamicToolCallResponse"].(Schema)
	if response["title"] != "DynamicToolCallResponse" {
		t.Fatalf("DynamicToolCallResponse source schema = %#v", response)
	}
	assertSchemaRequired(t, response, "contentItems")
	assertSchemaRequired(t, response, "success")

	content := defs["DynamicToolCallOutputContentItem"].(Schema)
	variants, ok := content["oneOf"].([]any)
	if !ok || len(variants) != 3 {
		t.Fatalf("content variants = %#v", content["oneOf"])
	}
	for index, want := range []struct {
		contentType string
		field       string
	}{{"inputText", "text"}, {"inputImage", "imageUrl"}, {"inputAudio", "audioUrl"}} {
		variant := variants[index].(Schema)
		if _, closed := variant["additionalProperties"]; closed {
			t.Fatalf("variant %s unexpectedly closes the serde-open record: %#v", want.contentType, variant)
		}
		properties := variant["properties"].(Schema)
		if variant["title"] != "Input"+strings.ToUpper(want.contentType[5:6])+want.contentType[6:]+"DynamicToolCallOutputContentItem" {
			t.Fatalf("variant %s title = %#v", want.contentType, variant["title"])
		}
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{want.contentType}) {
			t.Fatalf("variant %d type = %#v", index, properties["type"])
		}
		assertSchemaRequired(t, variant, want.field)
	}

	bindings := WireTypeBindings()
	assertBinding(t, bindings, "item/tool/call", SurfaceServerRequest, "DynamicToolCallParams")
	assertBinding(t, bindings, "item/tool/call", SurfaceServerRequest, "DynamicToolCallResponse")
	typescript, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	for _, declaration := range []string{
		`export type DynamicToolCallParams = { "threadId": string; "turnId": string; "callId": string; "namespace": string | null; "tool": string; "arguments": JsonValue; };`,
		`export type DynamicToolCallResponse = { "contentItems": Array<DynamicToolCallOutputContentItem>; "success": boolean; };`,
		`export type DynamicToolCallOutputContentItem = { "type": "inputText"; "text": string; } | { "type": "inputImage"; "imageUrl": string; } | { "type": "inputAudio"; "audioUrl": string; };`,
	} {
		if !strings.Contains(string(typescript), declaration) {
			t.Fatalf("generated TypeScript missing %q", declaration)
		}
	}
}

func TestDynamicToolCallParamsUseSourceSerdeSemantics(t *testing.T) {
	var params DynamicToolCallParams
	if err := json.Unmarshal([]byte(`{"threadId":"thread","turnId":"turn","callId":"call","tool":"client.search","arguments":null,"ignored":true}`), &params); err != nil {
		t.Fatalf("Unmarshal source-open params: %v", err)
	}
	if params.Namespace != nil || string(params.Arguments) != "null" {
		t.Fatalf("decoded params = %#v", params)
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("Marshal canonical params: %v", err)
	}
	if string(encoded) != `{"threadId":"thread","turnId":"turn","callId":"call","namespace":null,"tool":"client.search","arguments":null}` {
		t.Fatalf("canonical params = %s", encoded)
	}
	for _, input := range []string{
		`{}`,
		`{"threadId":"thread","turnId":"turn","callId":"call","tool":"client.search"}`,
		`{"threadId":"thread","threadId":"duplicate","turnId":"turn","callId":"call","tool":"client.search","arguments":{}}`,
		`{"threadId":"thread","turnId":"turn","callId":"call","tool":"client.search","arguments":{}} null`,
	} {
		if err := json.Unmarshal([]byte(input), &params); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var nilParams *DynamicToolCallParams
	if err := nilParams.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Error("nil params receiver succeeded")
	}
}

func TestDynamicToolCallResponseWireValidation(t *testing.T) {
	valid := []string{
		`{"contentItems":[],"success":false}`,
		`{"contentItems":[{"type":"inputText","text":""}],"success":true}`,
		`{"contentItems":[{"type":"inputImage","imageUrl":"data:image/png;base64,AA=="}],"success":true}`,
		`{"contentItems":[{"type":"inputAudio","audioUrl":"data:audio/wav;base64,AA=="}],"success":true}`,
		`{"contentItems":[{"type":"inputText","text":"ok","imageUrl":"ignored","extra":true}],"success":true,"extra":true}`,
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
		`{"contentItems":[{"type":"inputImage"}],"success":true}`,
		`{"contentItems":[{"type":"inputAudio"}],"success":true}`,
		`{"contentItems":[{"type":"video","url":"bad"}],"success":true}`,
		`{"contentItems":[],"contentItems":[],"success":true}`,
		`{"contentItems":[{"type":"inputText","text":"ok","text":"duplicate"}],"success":true}`,
		`{"contentItems":[],"success":true} null`,
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
		{Type: "inputAudio", AudioURL: "data:audio/wav;base64,AA=="},
	}, Success: true}
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"type":"inputText","text":"match"`, `"type":"inputImage","imageUrl":"data:image/png;base64,AA=="`, `"type":"inputAudio","audioUrl":"data:audio/wav;base64,AA=="`, `"success":true`} {
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
		{Type: "inputAudio", AudioURL: "audio", Text: "bad"},
		{Type: "video"},
	}
	for _, item := range invalid {
		if _, err := json.Marshal(item); err == nil {
			t.Errorf("Marshal(%#v) succeeded", item)
		}
	}
}
