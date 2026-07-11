package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUserInputSchemaAndBindingAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"ToolRequestUserInputAnswer",
		"ToolRequestUserInputOption",
		"ToolRequestUserInputParams",
		"ToolRequestUserInputQuestion",
		"ToolRequestUserInputResponse",
	} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("$defs missing %s", name)
		}
	}
	params := defs["ToolRequestUserInputParams"].(Schema)
	for _, name := range []string{"threadId", "turnId", "itemId", "questions", "autoResolutionMs"} {
		assertSchemaRequired(t, params, name)
	}
	question := defs["ToolRequestUserInputQuestion"].(Schema)
	for _, name := range []string{"id", "header", "question", "isOther", "isSecret", "options"} {
		assertSchemaRequired(t, question, name)
	}
	answer := defs["ToolRequestUserInputAnswer"].(Schema)
	assertSchemaRequired(t, answer, "answers")
	response := defs["ToolRequestUserInputResponse"].(Schema)
	assertSchemaRequired(t, response, "answers")
	bindings := WireTypeBindings()
	assertBinding(t, bindings, "item/tool/requestUserInput", SurfaceServerRequest, "ToolRequestUserInputParams")
	assertBinding(t, bindings, "item/tool/requestUserInput", SurfaceServerRequest, "ToolRequestUserInputResponse")
}

func TestUserInputResponseWireValidation(t *testing.T) {
	valid := []string{
		`{"answers":{}}`,
		`{"answers":{"question-1":{"answers":[]}}}`,
		`{"answers":{"question-1":{"answers":["safe","other"]}}}`,
	}
	for _, input := range valid {
		var response ToolRequestUserInputResponse
		if err := json.Unmarshal([]byte(input), &response); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
		}
	}
	invalid := []string{
		`[`,
		`{}`,
		`{"answers":null}`,
		`{"answers":[]}`,
		`{"answers":{"question-1":null}}`,
		`{"answers":{"question-1":{}}}`,
		`{"answers":{"question-1":{"answers":null}}}`,
		`{"answers":{"question-1":{"answers":[1]}}}`,
		`{"answers":{"question-1":{"answers":[],"extra":true}}}`,
		`{"answers":{},"extra":true}`,
	}
	for _, input := range invalid {
		var response ToolRequestUserInputResponse
		if err := json.Unmarshal([]byte(input), &response); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var response *ToolRequestUserInputResponse
	if err := response.UnmarshalJSON([]byte(`{"answers":{}}`)); err == nil {
		t.Error("nil response receiver succeeded")
	}
	var answer *ToolRequestUserInputAnswer
	if err := answer.UnmarshalJSON([]byte(`{"answers":[]}`)); err == nil {
		t.Error("nil answer receiver succeeded")
	}
}

func TestUserInputResponseMarshalUsesNonNullCollections(t *testing.T) {
	encoded, err := json.Marshal(ToolRequestUserInputResponse{Answers: map[string]ToolRequestUserInputAnswer{
		"question-1": {Answers: []string{"safe"}},
	}})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"question-1":{"answers":["safe"]}`) {
		t.Fatalf("response = %s", encoded)
	}
	empty, err := json.Marshal(ToolRequestUserInputResponse{})
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	if string(empty) != `{"answers":{}}` {
		t.Fatalf("empty response = %s", empty)
	}
	nilAnswer, err := json.Marshal(ToolRequestUserInputAnswer{})
	if err != nil {
		t.Fatalf("Marshal nil answer: %v", err)
	}
	if string(nilAnswer) != `{"answers":[]}` {
		t.Fatalf("nil answer = %s", nilAnswer)
	}
}
