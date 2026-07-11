package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestResponseItemSchemaIsExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	response, ok := defs["ResponseItem"].(Schema)
	if !ok {
		t.Fatal("$defs missing ResponseItem")
	}
	variants, ok := response["oneOf"].([]any)
	if !ok || len(variants) != 16 {
		t.Fatalf("ResponseItem variants = %#v", response["oneOf"])
	}
	want := []struct {
		itemType string
		required []string
		common   bool
	}{
		{itemType: "message", required: []string{"type", "role", "content"}, common: true},
		{itemType: "agent_message", required: []string{"type", "author", "recipient", "content"}, common: true},
		{itemType: "reasoning", required: []string{"type", "summary", "encrypted_content"}, common: true},
		{itemType: "local_shell_call", required: []string{"type", "call_id", "status", "action"}, common: true},
		{itemType: "function_call", required: []string{"type", "name", "arguments", "call_id"}, common: true},
		{itemType: "tool_search_call", required: []string{"type", "call_id", "execution", "arguments"}, common: true},
		{itemType: "function_call_output", required: []string{"type", "call_id", "output"}, common: true},
		{itemType: "custom_tool_call", required: []string{"type", "call_id", "name", "input"}, common: true},
		{itemType: "custom_tool_call_output", required: []string{"type", "call_id", "output"}, common: true},
		{itemType: "tool_search_output", required: []string{"type", "call_id", "status", "execution", "tools"}, common: true},
		{itemType: "web_search_call", required: []string{"type"}, common: true},
		{itemType: "image_generation_call", required: []string{"type", "status", "result"}, common: true},
		{itemType: "compaction", required: []string{"type", "encrypted_content"}, common: true},
		{itemType: "compaction_trigger", required: []string{"type"}},
		{itemType: "context_compaction", required: []string{"type"}, common: true},
		{itemType: "other", required: []string{"type"}},
	}
	byType := make(map[string]Schema, len(variants))
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("ResponseItem %s allows extra fields", expected.itemType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.itemType}) {
			t.Fatalf("ResponseItem variant %d type = %#v", index, properties["type"])
		}
		if !slices.Equal(schemaRequiredNames(variant), expected.required) {
			t.Fatalf("ResponseItem %s required = %v, want %v", expected.itemType, schemaRequiredNames(variant), expected.required)
		}
		_, hasID := properties["id"]
		_, hasMetadata := properties[responseItemMetadataField]
		if hasID != expected.common || hasMetadata != expected.common {
			t.Fatalf("ResponseItem %s common fields: id=%v metadata=%v", expected.itemType, hasID, hasMetadata)
		}
		if expected.common {
			if !reflect.DeepEqual(properties["id"], Schema{"type": "string"}) {
				t.Fatalf("ResponseItem %s id = %#v", expected.itemType, properties["id"])
			}
			assertResponseItemRef(t, properties[responseItemMetadataField], "InternalChatMessageMetadataPassthrough")
		}
		byType[expected.itemType] = variant
	}

	assertResponseItemArrayRef(t, byType["message"], "content", "ContentItem")
	assertResponseItemRef(t, responseItemProperties(byType["message"])["phase"], "MessagePhase")
	assertResponseItemArrayRef(t, byType["agent_message"], "content", "AgentMessageInputContent")
	assertResponseItemArrayRef(t, byType["reasoning"], "summary", "ReasoningItemReasoningSummary")
	assertResponseItemArrayRef(t, byType["reasoning"], "content", "ReasoningItemContent")
	assertNullableStringSchema(t, responseItemProperties(byType["reasoning"])["encrypted_content"])
	assertNullableStringSchema(t, responseItemProperties(byType["local_shell_call"])["call_id"])
	assertResponseItemRef(t, responseItemProperties(byType["local_shell_call"])["status"], "LocalShellStatus")
	assertResponseItemRef(t, responseItemProperties(byType["local_shell_call"])["action"], "LocalShellAction")
	assertNullableStringSchema(t, responseItemProperties(byType["tool_search_call"])["call_id"])
	if !reflect.DeepEqual(responseItemProperties(byType["tool_search_call"])["arguments"], Schema{}) {
		t.Fatalf("tool_search_call arguments = %#v", responseItemProperties(byType["tool_search_call"])["arguments"])
	}
	assertResponseItemRef(t, responseItemProperties(byType["function_call_output"])["output"], "FunctionCallOutputBody")
	assertResponseItemRef(t, responseItemProperties(byType["custom_tool_call_output"])["output"], "FunctionCallOutputBody")
	assertNullableStringSchema(t, responseItemProperties(byType["tool_search_output"])["call_id"])
	tools := responseItemProperties(byType["tool_search_output"])["tools"].(Schema)
	if tools["type"] != "array" || !reflect.DeepEqual(tools["items"], Schema{}) {
		t.Fatalf("tool_search_output tools = %#v", tools)
	}
	assertResponseItemRef(t, responseItemProperties(byType["web_search_call"])["action"], "WebSearchAction")
	if !reflect.DeepEqual(responseItemProperties(byType["context_compaction"])["encrypted_content"], Schema{"type": "string"}) {
		t.Fatalf("context_compaction encrypted_content = %#v", responseItemProperties(byType["context_compaction"])["encrypted_content"])
	}
}

func TestResponseItemWireValidationAndCanonicalization(t *testing.T) {
	valid := []string{
		`{"type":"message","role":"assistant","content":[]}`,
		`{"type":"message","id":"msg-1","role":"assistant","content":[{"type":"output_text","text":"done"}],"phase":"final_answer","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}`,
		`{"type":"agent_message","author":"agent-a","recipient":"agent-b","content":[]}`,
		`{"type":"agent_message","id":"agent-1","author":"agent-a","recipient":"agent-b","content":[{"type":"input_text","text":"hello"}],"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"reasoning","summary":[],"encrypted_content":null}`,
		`{"type":"reasoning","id":"reason-1","summary":[{"type":"summary_text","text":"summary"}],"content":[{"type":"reasoning_text","text":"thinking"}],"encrypted_content":"cipher","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}`,
		`{"type":"local_shell_call","call_id":null,"status":"in_progress","action":{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}}`,
		`{"type":"local_shell_call","id":"shell-1","call_id":"call-1","status":"completed","action":{"type":"exec","command":["pwd"],"timeout_ms":1000,"working_directory":"/workspace","env":{"CI":"1"},"user":"runner"},"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"function_call","name":"run","arguments":"{}","call_id":"call-1"}`,
		`{"type":"function_call","id":"fn-1","name":"run","namespace":"tools","arguments":"{\"x\":1}","call_id":"call-1","internal_chat_message_metadata_passthrough":{"turn_id":"turn-1"}}`,
		`{"type":"tool_search_call","call_id":null,"execution":"search","arguments":null}`,
		`{"type":"tool_search_call","id":"search-1","call_id":"call-1","status":"completed","execution":"search","arguments":{"limit":10,"query":"go"},"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"function_call_output","call_id":"call-1","output":"done"}`,
		`{"type":"function_call_output","id":"out-1","call_id":"call-1","output":[{"type":"input_text","text":"done"}],"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"custom_tool_call","call_id":"call-1","name":"shell","input":"pwd"}`,
		`{"type":"custom_tool_call","id":"custom-1","status":"completed","call_id":"call-1","name":"shell","namespace":"local","input":"pwd","internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"custom_tool_call_output","call_id":"call-1","output":"done"}`,
		`{"type":"custom_tool_call_output","id":"custom-out-1","call_id":"call-1","name":"shell","output":[],"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"tool_search_output","call_id":null,"status":"completed","execution":"search","tools":[]}`,
		`{"type":"tool_search_output","id":"tools-1","call_id":"call-1","status":"completed","execution":"search","tools":[null,true,1,"tool",[1],{"name":"run"}],"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"web_search_call"}`,
		`{"type":"web_search_call","id":"web-1","status":"completed","action":{"type":"search","query":"gollem","queries":[]},"internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"image_generation_call","status":"completed","result":"base64"}`,
		`{"type":"image_generation_call","id":"image-1","status":"completed","revised_prompt":"draw","result":"base64","internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"compaction","encrypted_content":"cipher"}`,
		`{"type":"compaction","id":"compact-1","encrypted_content":"cipher","internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"compaction_trigger"}`,
		`{"type":"context_compaction"}`,
		`{"type":"context_compaction","id":"context-1","encrypted_content":"cipher","internal_chat_message_metadata_passthrough":{}}`,
		`{"type":"other"}`,
	}
	assertRawJSONRoundTrips[ResponseItem](t, valid)

	invalid := []string{
		`null`, `[]`, `{}`, `{"type":null}`, `{"type":1}`, `{"type":"unknown"}`,
		`{"type":"message","role":"assistant"}`,
		`{"type":"message","id":null,"role":"assistant","content":[]}`,
		`{"type":"message","role":null,"content":[]}`,
		`{"type":"message","role":"assistant","content":null}`,
		`{"type":"message","role":"assistant","content":[],"phase":null}`,
		`{"type":"message","role":"assistant","content":[],"internal_chat_message_metadata_passthrough":null}`,
		`{"type":"message","role":"assistant","content":[],"call_id":"crossed"}`,
		`{"type":"agent_message","author":"agent-a","content":[]}`,
		`{"type":"agent_message","recipient":"agent-b","content":[]}`,
		`{"type":"agent_message","id":null,"author":"agent-a","recipient":"agent-b","content":[]}`,
		`{"type":"agent_message","author":"agent-a","recipient":"agent-b","content":null}`,
		`{"type":"agent_message","author":"agent-a","recipient":"agent-b","content":[],"role":"crossed"}`,
		`{"type":"reasoning","summary":[]}`,
		`{"type":"reasoning","id":null,"summary":[],"encrypted_content":null}`,
		`{"type":"reasoning","summary":null,"encrypted_content":null}`,
		`{"type":"reasoning","summary":[],"content":null,"encrypted_content":null}`,
		`{"type":"reasoning","summary":[],"encrypted_content":1}`,
		`{"type":"reasoning","summary":[],"encrypted_content":null,"role":"crossed"}`,
		`{"type":"local_shell_call","status":"completed","action":{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}}`,
		`{"type":"local_shell_call","id":null,"call_id":null,"status":"completed","action":{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}}`,
		`{"type":"local_shell_call","call_id":1,"status":"completed","action":{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}}`,
		`{"type":"local_shell_call","call_id":null,"status":"failed","action":{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null}}`,
		`{"type":"local_shell_call","call_id":null,"status":"completed","action":null}`,
		`{"type":"local_shell_call","call_id":null,"status":"completed","action":{"type":"exec","command":[],"timeout_ms":null,"working_directory":null,"env":null,"user":null},"role":"crossed"}`,
		`{"type":"function_call","name":"run","call_id":"call-1"}`,
		`{"type":"function_call","arguments":"{}","call_id":"call-1"}`,
		`{"type":"function_call","id":null,"name":"run","arguments":"{}","call_id":"call-1"}`,
		`{"type":"function_call","name":"run","namespace":null,"arguments":"{}","call_id":"call-1"}`,
		`{"type":"function_call","name":"run","arguments":{},"call_id":"call-1"}`,
		`{"type":"function_call","name":"run","arguments":"{}","call_id":null}`,
		`{"type":"function_call","name":"run","arguments":"{}","call_id":"call-1","role":"crossed"}`,
		`{"type":"tool_search_call","execution":"search","arguments":null}`,
		`{"type":"tool_search_call","id":null,"call_id":null,"execution":"search","arguments":null}`,
		`{"type":"tool_search_call","call_id":null,"status":null,"execution":"search","arguments":null}`,
		`{"type":"tool_search_call","call_id":null,"arguments":null}`,
		`{"type":"tool_search_call","call_id":null,"execution":"search"}`,
		`{"type":"tool_search_call","call_id":null,"execution":"search","arguments":null,"role":"crossed"}`,
		`{"type":"function_call_output","call_id":null,"output":"done"}`,
		`{"type":"function_call_output","id":null,"call_id":"call-1","output":"done"}`,
		`{"type":"function_call_output","call_id":"call-1","output":null}`,
		`{"type":"function_call_output","call_id":"call-1","output":"done","role":"crossed"}`,
		`{"type":"custom_tool_call","call_id":"call-1","input":"pwd"}`,
		`{"type":"custom_tool_call","name":"shell","input":"pwd"}`,
		`{"type":"custom_tool_call","id":null,"call_id":"call-1","name":"shell","input":"pwd"}`,
		`{"type":"custom_tool_call","status":null,"call_id":"call-1","name":"shell","input":"pwd"}`,
		`{"type":"custom_tool_call","call_id":"call-1","name":"shell","namespace":null,"input":"pwd"}`,
		`{"type":"custom_tool_call","call_id":"call-1","name":"shell"}`,
		`{"type":"custom_tool_call","call_id":"call-1","name":"shell","input":"pwd","role":"crossed"}`,
		`{"type":"custom_tool_call_output","output":"done"}`,
		`{"type":"custom_tool_call_output","id":null,"call_id":"call-1","output":"done"}`,
		`{"type":"custom_tool_call_output","call_id":"call-1","name":null,"output":"done"}`,
		`{"type":"custom_tool_call_output","call_id":"call-1","output":{}}`,
		`{"type":"custom_tool_call_output","call_id":"call-1","output":"done","role":"crossed"}`,
		`{"type":"tool_search_output","call_id":null,"status":"completed","execution":"search"}`,
		`{"type":"tool_search_output","id":null,"call_id":null,"status":"completed","execution":"search","tools":[]}`,
		`{"type":"tool_search_output","call_id":1,"status":"completed","execution":"search","tools":[]}`,
		`{"type":"tool_search_output","call_id":null,"status":"completed","execution":"search","tools":null}`,
		`{"type":"tool_search_output","call_id":null,"status":"completed","execution":"search","tools":{}}`,
		`{"type":"tool_search_output","call_id":null,"status":null,"execution":"search","tools":[]}`,
		`{"type":"tool_search_output","call_id":null,"status":"completed","tools":[]}`,
		`{"type":"tool_search_output","call_id":null,"status":"completed","execution":"search","tools":[],"role":"crossed"}`,
		`{"type":"web_search_call","status":null}`,
		`{"type":"web_search_call","id":null}`,
		`{"type":"web_search_call","action":null}`,
		`{"type":"web_search_call","action":{"type":"open_page","url":"https://example.test"}}`,
		`{"type":"web_search_call","role":"crossed"}`,
		`{"type":"image_generation_call","status":"completed"}`,
		`{"type":"image_generation_call","id":null,"status":"completed","result":"base64"}`,
		`{"type":"image_generation_call","status":null,"result":"base64"}`,
		`{"type":"image_generation_call","status":"completed","revised_prompt":null,"result":"base64"}`,
		`{"type":"image_generation_call","status":"completed","result":"base64","role":"crossed"}`,
		`{"type":"compaction"}`,
		`{"type":"compaction","id":null,"encrypted_content":"cipher"}`,
		`{"type":"compaction","encrypted_content":null}`,
		`{"type":"compaction","encrypted_content":"cipher","role":"crossed"}`,
		`{"type":"compaction_trigger","id":"forbidden"}`,
		`{"type":"context_compaction","id":null}`,
		`{"type":"context_compaction","encrypted_content":null}`,
		`{"type":"context_compaction","role":"crossed"}`,
		`{"type":"other","extra":true}`,
		`{"type":"other","callId":"camel"}`,
	}
	assertRawJSONRejects[ResponseItem](t, invalid)
}

func TestResponseItemUnknownJSONValuesRemainPreciseAndCanonical(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{
			input: `{ "type": "tool_search_call", "call_id": null, "execution": "search", "arguments": { "z": 1, "a": 18446744073709551616 } }`,
			want:  `{"type":"tool_search_call","call_id":null,"execution":"search","arguments":{"a":18446744073709551616,"z":1}}`,
		},
		{
			input: `{ "type": "tool_search_output", "call_id": null, "status": "completed", "execution": "search", "tools": [ { "z": 1, "a": 2 }, null, 1e100 ] }`,
			want:  `{"type":"tool_search_output","call_id":null,"status":"completed","execution":"search","tools":[{"a":2,"z":1},null,1e100]}`,
		},
	}
	for _, testCase := range cases {
		var item ResponseItem
		if err := json.Unmarshal([]byte(testCase.input), &item); err != nil {
			t.Fatalf("Unmarshal(%s): %v", testCase.input, err)
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			t.Fatalf("Marshal(%s): %v", testCase.input, err)
		}
		if string(encoded) != testCase.want {
			t.Fatalf("round trip %s = %s, want %s", testCase.input, encoded, testCase.want)
		}
	}

	for _, testCase := range []struct {
		input string
		want  string
	}{
		{input: ` null `, want: `null`},
		{input: ` {"z":1,"a":[true,false]} `, want: `{"a":[true,false],"z":1}`},
	} {
		got, err := canonicalizeResponseItemJSONValue([]byte(testCase.input))
		if err != nil || string(got) != testCase.want {
			t.Fatalf("canonicalize(%q) = %s, %v; want %s", testCase.input, got, err, testCase.want)
		}
	}
	for _, input := range []string{"", "1 2", "1 trailing"} {
		if _, err := canonicalizeResponseItemJSONValue([]byte(input)); err == nil {
			t.Errorf("canonicalize(%q) succeeded", input)
		}
	}
	if _, err := decodeRequiredResponseItemJSON(map[string]json.RawMessage{}, "test item", "arguments"); err == nil {
		t.Fatal("missing required unknown JSON succeeded")
	}
	if _, err := decodeRequiredResponseItemJSON(map[string]json.RawMessage{"arguments": json.RawMessage(`invalid`)}, "test item", "arguments"); err == nil {
		t.Fatal("malformed required unknown JSON succeeded")
	}
	if _, err := decodeRequiredResponseItemJSONArray(map[string]json.RawMessage{"tools": json.RawMessage(`[invalid]`)}, "test item", "tools"); err == nil {
		t.Fatal("malformed required unknown array succeeded")
	}
}

func TestResponseItemEmptyAndNilReceiversFailClosed(t *testing.T) {
	assertEmptyRawJSONMarshalRejects(t, ResponseItem{})
	var item *ResponseItem
	if err := item.UnmarshalJSON([]byte(`{"type":"other"}`)); err == nil {
		t.Fatal("nil ResponseItem receiver succeeded")
	}
}

func responseItemProperties(schema Schema) Schema {
	return schema["properties"].(Schema)
}

func assertResponseItemRef(t *testing.T, raw any, name string) {
	t.Helper()
	if !reflect.DeepEqual(raw, Schema{"$ref": "#/$defs/" + name}) {
		t.Fatalf("ResponseItem ref = %#v, want %s", raw, name)
	}
}

func assertResponseItemArrayRef(t *testing.T, variant Schema, fieldName, name string) {
	t.Helper()
	property := responseItemProperties(variant)[fieldName]
	want := Schema{"type": "array", "items": Schema{"$ref": "#/$defs/" + name}}
	if !reflect.DeepEqual(property, want) {
		t.Fatalf("ResponseItem %s = %#v, want %#v", fieldName, property, want)
	}
}
