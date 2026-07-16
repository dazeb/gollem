package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTurnInterruptSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	params, ok := defs["TurnInterruptParams"].(Schema)
	if !ok {
		t.Fatal("$defs missing TurnInterruptParams")
	}
	wantParams := Schema{
		"type":                 "object",
		"additionalProperties": false,
		"properties": Schema{
			"threadId": Schema{"type": "string"},
			"turnId":   Schema{"type": "string"},
		},
		"required": []string{"threadId", "turnId"},
	}
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("TurnInterruptParams = %#v, want %#v", params, wantParams)
	}

	response, ok := defs["TurnInterruptResponse"].(Schema)
	if !ok {
		t.Fatal("$defs missing TurnInterruptResponse")
	}
	wantResponse := Schema{
		"type":                 "object",
		"additionalProperties": false,
	}
	if !reflect.DeepEqual(response, wantResponse) {
		t.Fatalf("TurnInterruptResponse = %#v, want %#v", response, wantResponse)
	}
}

func TestTurnInterruptParamsWireContract(t *testing.T) {
	for _, input := range []string{
		`{"threadId":"","turnId":""}`,
		`{"threadId":"thread","turnId":"turn"}`,
	} {
		var params TurnInterruptParams
		if err := json.Unmarshal([]byte(input), &params); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(params)
		if err != nil || string(encoded) != input {
			t.Errorf("round trip = %s, %v; want %s", encoded, err, input)
		}
	}

	invalid := []string{
		`null`, `[]`, `"value"`, `1`, `{}`,
		`{"threadId":"thread"}`,
		`{"turnId":"turn"}`,
		`{"threadId":null,"turnId":"turn"}`,
		`{"threadId":"thread","turnId":null}`,
		`{"threadId":1,"turnId":"turn"}`,
		`{"threadId":"thread","turnId":1}`,
		`{"threadId":"thread","turnId":"turn","id":"legacy"}`,
		`{"threadId":"thread","turnId":"turn"} {}`,
	}
	for _, input := range invalid {
		var params TurnInterruptParams
		if err := json.Unmarshal([]byte(input), &params); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var nilParams *TurnInterruptParams
	if err := nilParams.UnmarshalJSON([]byte(`{"threadId":"thread","turnId":"turn"}`)); err == nil {
		t.Fatal("nil TurnInterruptParams receiver succeeded")
	}
}

func TestTurnInterruptResponseWireContract(t *testing.T) {
	var response TurnInterruptResponse
	if err := json.Unmarshal([]byte(`{}`), &response); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	encoded, err := json.Marshal(response)
	if err != nil || string(encoded) != `{}` {
		t.Fatalf("Marshal = %s, %v", encoded, err)
	}
	for _, input := range []string{
		`null`, `[]`, `"value"`, `1`,
		`{"ok":true}`,
		`{"turnId":"turn"}`,
		`{} {}`,
	} {
		var value TurnInterruptResponse
		if err := json.Unmarshal([]byte(input), &value); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var nilResponse *TurnInterruptResponse
	if err := nilResponse.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("nil TurnInterruptResponse receiver succeeded")
	}
}

func TestTurnInterruptContractsRemainStandalone(t *testing.T) {
	for _, binding := range WireTypeBindings() {
		if binding.Method == "turn/interrupt" {
			t.Fatalf("turn/interrupt unexpectedly bound: %#v", binding)
		}
	}
	if got := len(JSONSchema()["$defs"].(Schema)); got != 466 {
		t.Fatalf("definition count = %d, want 466", got)
	}
	if got := len(WireTypeBindings()); got != 59 || len(ItemPayloadBindings()) != 5 {
		t.Fatalf("bindings = %d methods/%d items, want 59/5", got, len(ItemPayloadBindings()))
	}
}

func TestTurnInterruptTypeScriptIsExact(t *testing.T) {
	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	source := string(generated)
	for _, want := range []string{
		`export type TurnInterruptParams = {`,
		`"threadId": string;`,
		`"turnId": string;`,
		`export type TurnInterruptResponse = Record<string, never>;`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("generated TypeScript missing %q", want)
		}
	}
}
