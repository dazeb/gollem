package protocol

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestToolRequestUserInputSourceSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	wants := map[string]Schema{
		"ToolRequestUserInputOption": {
			"description": "EXPERIMENTAL. Defines a single selectable option for request_user_input.",
			"properties":  Schema{"description": Schema{"type": "string"}, "label": Schema{"type": "string"}},
			"required":    []string{"description", "label"},
			"type":        "object",
		},
		"ToolRequestUserInputQuestion": {
			"description": "EXPERIMENTAL. Represents one request_user_input question and its required options.",
			"properties": Schema{
				"header":   Schema{"type": "string"},
				"id":       Schema{"type": "string"},
				"isOther":  Schema{"default": false, "type": "boolean"},
				"isSecret": Schema{"default": false, "type": "boolean"},
				"options":  Schema{"items": Schema{"$ref": "#/$defs/ToolRequestUserInputOption"}, "type": []any{"array", "null"}},
				"question": Schema{"type": "string"},
			},
			"required": []string{"header", "id", "question"},
			"type":     "object",
		},
		"ToolRequestUserInputParams": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "EXPERIMENTAL. Params sent with a request_user_input event.",
			"properties": Schema{
				"autoResolutionMs": Schema{
					"default":     nil,
					"description": "@deprecated Use `isBlocking` to decide whether the request should block.",
					"format":      "uint64",
					"minimum":     json.Number("0.0"),
					"type":        []any{"integer", "null"},
				},
				"isBlocking": Schema{"type": "boolean"},
				"itemId":     Schema{"type": "string"},
				"questions":  Schema{"items": Schema{"$ref": "#/$defs/ToolRequestUserInputQuestion"}, "type": "array"},
				"threadId":   Schema{"type": "string"},
				"turnId":     Schema{"type": "string"},
			},
			"required": []string{"isBlocking", "itemId", "questions", "threadId", "turnId"},
			"title":    "ToolRequestUserInputParams",
			"type":     "object",
		},
		"ToolRequestUserInputAnswer": {
			"description": "EXPERIMENTAL. Captures a user's answer to a request_user_input question.",
			"properties":  Schema{"answers": Schema{"items": Schema{"type": "string"}, "type": "array"}},
			"required":    []string{"answers"},
			"type":        "object",
		},
		"ToolRequestUserInputResponse": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "EXPERIMENTAL. Response payload mapping question ids to answers.",
			"properties":  Schema{"answers": Schema{"additionalProperties": Schema{"$ref": "#/$defs/ToolRequestUserInputAnswer"}, "type": "object"}},
			"required":    []string{"answers"},
			"title":       "ToolRequestUserInputResponse",
			"type":        "object",
		},
	}
	for name, want := range wants {
		if got := defs[name]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %#v, want %#v", name, got, want)
		}
	}
}

func TestToolRequestUserInputParamsMatchSerdeWireForms(t *testing.T) {
	for _, test := range []struct {
		input string
		want  string
	}{
		{
			input: `{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","future":true}],"future":1}`,
			want:  `{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","isOther":false,"isSecret":false,"options":null}],"isBlocking":true,"autoResolutionMs":null}`,
		},
		{
			input: `{"threadId":"thread","turnId":"turn","itemId":"item","questions":[],"isBlocking":null,"autoResolutionMs":null}`,
			want:  `{"threadId":"thread","turnId":"turn","itemId":"item","questions":[],"isBlocking":true,"autoResolutionMs":null}`,
		},
		{
			input: `{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","isOther":true,"isSecret":true,"options":[{"label":"yes","description":"Continue","future":"ignored"}]}],"isBlocking":false,"autoResolutionMs":5,"future":2}`,
			want:  `{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","isOther":true,"isSecret":true,"options":[{"label":"yes","description":"Continue"}]}],"isBlocking":false,"autoResolutionMs":5}`,
		},
	} {
		var params ToolRequestUserInputParams
		if err := json.Unmarshal([]byte(test.input), &params); err != nil {
			t.Errorf("Unmarshal(%s): %v", test.input, err)
			continue
		}
		encoded, err := json.Marshal(params)
		if err != nil || string(encoded) != test.want {
			t.Errorf("round trip %s = %s, %v; want %s", test.input, encoded, err, test.want)
		}
	}
}

func TestToolRequestUserInputParamsRejectMalformedSerdeWire(t *testing.T) {
	valid := `"threadId":"thread","turnId":"turn","itemId":"item","questions":[]`
	for _, input := range []string{
		``, `null`, `[]`, `{`, `{}`,
		`{"turnId":"turn","itemId":"item","questions":[]}`,
		`{"threadId":"thread","itemId":"item","questions":[]}`,
		`{"threadId":"thread","turnId":"turn","questions":[]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item"}`,
		`{"threadId":null,"turnId":"turn","itemId":"item","questions":[]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":null}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{}]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","isOther":null}]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","options":[{"label":"yes"}]}]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[],"isBlocking":1}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[],"autoResolutionMs":-1}`,
		`{"threadId":"one","threadId":"two","turnId":"turn","itemId":"item","questions":[]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[],"isBlocking":true,"isBlocking":false}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","id":"duplicate","header":"Header","question":"Choose"}]}`,
		`{"threadId":"thread","turnId":"turn","itemId":"item","questions":[{"id":"question","header":"Header","question":"Choose","options":[{"label":"one","label":"two","description":"Continue"}]}]}`,
		`{` + valid + `} {}`,
		`{` + valid + `} x`,
	} {
		assertJSONRejects[ToolRequestUserInputParams](t, input)
	}

	var params *ToolRequestUserInputParams
	if err := params.UnmarshalJSON([]byte(`{` + valid + `}`)); err == nil {
		t.Fatal("nil ToolRequestUserInputParams receiver succeeded")
	}
}

func TestToolRequestUserInputTypeScriptIsExact(t *testing.T) {
	generated, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	want := "export type ToolRequestUserInputParams = {\n" +
		"  \"autoResolutionMs\": number | null;\n" +
		"  \"isBlocking\": boolean;\n" +
		"  \"itemId\": string;\n" +
		"  \"questions\": Array<ToolRequestUserInputQuestion>;\n" +
		"  \"threadId\": string;\n" +
		"  \"turnId\": string;\n" +
		"};"
	if !strings.Contains(string(generated), want) {
		t.Fatalf("generated TypeScript missing %q", want)
	}
}
