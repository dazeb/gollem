package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMcpElicitationSchemaAndBindingAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	for _, name := range []string{
		"McpElicitationArrayType",
		"McpElicitationBooleanSchema",
		"McpElicitationBooleanType",
		"McpElicitationConstOption",
		"McpElicitationEnumSchema",
		"McpElicitationLegacyTitledEnumSchema",
		"McpElicitationMultiSelectEnumSchema",
		"McpElicitationNumberSchema",
		"McpElicitationNumberType",
		"McpElicitationObjectType",
		"McpElicitationPrimitiveSchema",
		"McpElicitationSchema",
		"McpElicitationSingleSelectEnumSchema",
		"McpElicitationStringFormat",
		"McpElicitationStringSchema",
		"McpElicitationStringType",
		"McpElicitationTitledEnumItems",
		"McpElicitationTitledMultiSelectEnumSchema",
		"McpElicitationTitledSingleSelectEnumSchema",
		"McpElicitationUntitledEnumItems",
		"McpElicitationUntitledMultiSelectEnumSchema",
		"McpElicitationUntitledSingleSelectEnumSchema",
		"McpServerElicitationAction",
		"McpServerElicitationRequestParams",
		"McpServerElicitationRequestResponse",
	} {
		if _, ok := defs[name]; !ok {
			t.Fatalf("$defs missing %s", name)
		}
	}
	schema := defs["McpElicitationSchema"].(Schema)
	for _, name := range []string{"type", "properties"} {
		assertSchemaRequired(t, schema, name)
	}
	params := defs["McpServerElicitationRequestParams"].(Schema)
	for _, name := range []string{"threadId", "serverName"} {
		assertSchemaRequired(t, params, name)
	}
	if _, required := params["additionalProperties"]; required {
		t.Fatalf("McpServerElicitationRequestParams unexpectedly closes the serde-open record: %#v", params)
	}
	variants, ok := params["oneOf"].([]any)
	if !ok || len(variants) != 3 {
		t.Fatalf("McpServerElicitationRequestParams oneOf = %#v", params["oneOf"])
	}
	for _, variant := range variants {
		item := variant.(Schema)
		for _, name := range []string{"mode", "message"} {
			assertSchemaRequired(t, item, name)
		}
		if _, required := item["additionalProperties"]; required {
			t.Fatalf("McpServerElicitationRequestParams variant unexpectedly closes the serde-open record: %#v", item)
		}
	}
	formProperties := variants[0].(Schema)["properties"].(Schema)
	openAIFormProperties := variants[1].(Schema)["properties"].(Schema)
	if formProperties["_meta"] != true || openAIFormProperties["_meta"] != true ||
		openAIFormProperties["requestedSchema"] != true {
		t.Fatalf("McpServerElicitationRequestParams JSON-value fields = %#v / %#v", formProperties, openAIFormProperties)
	}
	urlRequired := variants[2].(Schema)["required"].([]string)
	if got, want := strings.Join(urlRequired, ","), "elicitationId,message,mode,url"; got != want {
		t.Fatalf("URL request required order = %q, want %q", got, want)
	}
	response := defs["McpServerElicitationRequestResponse"].(Schema)
	assertSchemaRequired(t, response, "action")
	if _, required := response["additionalProperties"]; required {
		t.Fatalf("McpServerElicitationRequestResponse unexpectedly closes the serde-open record: %#v", response)
	}
	enumSchema := defs["McpElicitationEnumSchema"].(Schema)
	if variants, ok := enumSchema["anyOf"].([]any); !ok || len(variants) != 3 {
		t.Fatalf("McpElicitationEnumSchema anyOf = %#v", enumSchema["anyOf"])
	}
	stringSchema := defs["McpElicitationStringSchema"].(Schema)
	stringProperties := stringSchema["properties"].(Schema)
	for _, name := range []string{"minLength", "maxLength"} {
		property := stringProperties[name].(Schema)
		if property["minimum"] != 0 {
			t.Fatalf("McpElicitationStringSchema.%s minimum = %#v", name, property["minimum"])
		}
	}
	bindings := WireTypeBindings()
	assertBinding(t, bindings, "mcpServer/elicitation/request", SurfaceServerRequest, "McpServerElicitationRequestParams")
	assertBinding(t, bindings, "mcpServer/elicitation/request", SurfaceServerRequest, "McpServerElicitationRequestResponse")
	typescript, err := MarshalTypeScript()
	if err != nil {
		t.Fatalf("MarshalTypeScript: %v", err)
	}
	for _, declaration := range []string{
		`export type McpServerElicitationRequestParams = { "threadId": string; "turnId": string | null; "serverName": string; } &`,
		`"requestedSchema": JsonValue;`,
		`export type McpServerElicitationRequestResponse = { "action": McpServerElicitationAction; "content": JsonValue | null; "_meta": JsonValue | null; };`,
	} {
		if !strings.Contains(string(typescript), declaration) {
			t.Fatalf("generated TypeScript missing %q", declaration)
		}
	}
}

func TestMcpElicitationSchemaAcceptsEveryPublicPrimitiveVariant(t *testing.T) {
	input := `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{
			"plain":{"type":"string","title":"Name","description":"Display name","minLength":1,"maxLength":64,"format":"email","default":"safe@example.com"},
			"count":{"type":"integer","title":"Count","minimum":0,"maximum":10,"default":1},
			"ratio":{"type":"number","default":0.5},
			"enabled":{"type":"boolean","default":true},
			"legacy":{"type":"string","enum":["safe","fast"],"enumNames":["Safe","Fast"],"default":"safe"},
			"single":{"type":"string","oneOf":[{"const":"safe","title":"Safe"},{"const":"fast","title":"Fast"}]},
			"multi":{"type":"array","minItems":1,"maxItems":2,"items":{"type":"string","enum":["read","write"]},"default":["read"]},
			"titledMulti":{"type":"array","items":{"oneOf":[{"const":"read","title":"Read"}]}}
		},
		"required":["plain","enabled"]
	}`
	var schema McpElicitationSchema
	if err := json.Unmarshal([]byte(input), &schema); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"items":{"anyOf":[{"const":"read","title":"Read"}]}`) ||
		strings.Contains(string(encoded), `"items":{"oneOf"`) {
		t.Fatalf("canonical schema = %s", encoded)
	}
}

func TestMcpElicitationSchemaRejectsMalformedVariants(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "malformed", input: `[`},
		{name: "null schema", input: `null`},
		{name: "unknown schema field", input: `{"type":"object","properties":{},"extra":true}`},
		{name: "missing type", input: `{"properties":{}}`},
		{name: "wrong object type", input: `{"type":"array","properties":{}}`},
		{name: "missing properties", input: `{"type":"object"}`},
		{name: "null properties", input: `{"type":"object","properties":null}`},
		{name: "invalid required", input: `{"type":"object","properties":{},"required":[1]}`},
		{name: "null primitive", input: `{"type":"object","properties":{"value":null}}`},
		{name: "unknown primitive type", input: `{"type":"object","properties":{"value":{"type":"object"}}}`},
		{name: "unknown primitive field", input: `{"type":"object","properties":{"value":{"type":"string","extra":true}}}`},
		{name: "invalid string format", input: `{"type":"object","properties":{"value":{"type":"string","format":"uuid"}}}`},
		{name: "negative string length", input: `{"type":"object","properties":{"value":{"type":"string","minLength":-1}}}`},
		{name: "fractional string length", input: `{"type":"object","properties":{"value":{"type":"string","maxLength":1.5}}}`},
		{name: "invalid number default", input: `{"type":"object","properties":{"value":{"type":"number","default":"one"}}}`},
		{name: "invalid boolean default", input: `{"type":"object","properties":{"value":{"type":"boolean","default":"yes"}}}`},
		{name: "missing enum", input: `{"type":"object","properties":{"value":{"type":"string","enum":null}}}`},
		{name: "invalid enum values", input: `{"type":"object","properties":{"value":{"type":"string","enum":[1]}}}`},
		{name: "invalid enum names", input: `{"type":"object","properties":{"value":{"type":"string","enum":["a"],"enumNames":[1]}}}`},
		{name: "missing titled const", input: `{"type":"object","properties":{"value":{"type":"string","oneOf":[{"title":"A"}]}}}`},
		{name: "extra titled option", input: `{"type":"object","properties":{"value":{"type":"string","oneOf":[{"const":"a","title":"A","extra":true}]}}}`},
		{name: "missing multi items", input: `{"type":"object","properties":{"value":{"type":"array"}}}`},
		{name: "negative multi limit", input: `{"type":"object","properties":{"value":{"type":"array","minItems":-1,"items":{"type":"string","enum":[]}}}}`},
		{name: "invalid multi default", input: `{"type":"object","properties":{"value":{"type":"array","items":{"type":"string","enum":[]},"default":[1]}}}`},
		{name: "invalid multi items", input: `{"type":"object","properties":{"value":{"type":"array","items":{"type":"number","enum":[]}}}}`},
		{name: "crossed titled items", input: `{"type":"object","properties":{"value":{"type":"array","items":{"anyOf":[],"oneOf":[]}}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema McpElicitationSchema
			if err := json.Unmarshal([]byte(tt.input), &schema); err == nil {
				t.Fatalf("Unmarshal(%s) succeeded", tt.input)
			}
		})
	}
}

func TestMcpElicitationUnionWrappersAreStrict(t *testing.T) {
	tests := []struct {
		name  string
		input string
		new   func() json.Unmarshaler
	}{
		{name: "primitive", input: `{"type":"boolean"}`, new: func() json.Unmarshaler { return new(McpElicitationPrimitiveSchema) }},
		{name: "enum", input: `{"type":"string","enum":["safe"]}`, new: func() json.Unmarshaler { return new(McpElicitationEnumSchema) }},
		{name: "single", input: `{"type":"string","oneOf":[{"const":"safe","title":"Safe"}]}`, new: func() json.Unmarshaler { return new(McpElicitationSingleSelectEnumSchema) }},
		{name: "multi", input: `{"type":"array","items":{"type":"string","enum":["safe"]}}`, new: func() json.Unmarshaler { return new(McpElicitationMultiSelectEnumSchema) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := tt.new()
			if err := value.UnmarshalJSON([]byte(tt.input)); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if _, err := json.Marshal(value); err != nil {
				t.Fatalf("Marshal: %v", err)
			}
		})
	}
	for _, input := range []string{
		`{"type":"string","oneOf":[{"const":"safe","title":"Safe"}]}`,
		`{"type":"array","items":{"anyOf":[{"const":"safe","title":"Safe"}]}}`,
	} {
		var value McpElicitationEnumSchema
		if err := json.Unmarshal([]byte(input), &value); err != nil {
			t.Errorf("enum Unmarshal(%s): %v", input, err)
		}
	}
	var untitledSingle McpElicitationSingleSelectEnumSchema
	if err := json.Unmarshal([]byte(`{"type":"string","enum":["safe"]}`), &untitledSingle); err != nil {
		t.Fatalf("untitled single: %v", err)
	}
	var titledMulti McpElicitationMultiSelectEnumSchema
	if err := json.Unmarshal([]byte(`{"type":"array","items":{"oneOf":[{"const":"safe","title":"Safe"}]}}`), &titledMulti); err != nil {
		t.Fatalf("titled multi: %v", err)
	}
	for _, value := range []json.Marshaler{
		McpElicitationPrimitiveSchema{},
		McpElicitationEnumSchema{},
		McpElicitationSingleSelectEnumSchema{},
		McpElicitationMultiSelectEnumSchema{},
	} {
		if _, err := value.MarshalJSON(); err == nil {
			t.Errorf("zero-value %T marshal succeeded", value)
		}
	}
	var invalidEnum McpElicitationEnumSchema
	if err := json.Unmarshal([]byte(`{"type":"boolean"}`), &invalidEnum); err == nil {
		t.Error("boolean enum succeeded")
	}
	var invalidSingle McpElicitationSingleSelectEnumSchema
	if err := json.Unmarshal([]byte(`{"type":"string","enum":["safe"],"enumNames":["Safe"]}`), &invalidSingle); err == nil {
		t.Error("legacy enum succeeded as a single-select schema")
	}
	var invalidMulti McpElicitationMultiSelectEnumSchema
	if err := json.Unmarshal([]byte(`{"type":"string","enum":["safe"]}`), &invalidMulti); err == nil {
		t.Error("string enum succeeded as a multi-select schema")
	}
	var primitive *McpElicitationPrimitiveSchema
	if err := primitive.UnmarshalJSON([]byte(`{"type":"string"}`)); err == nil {
		t.Error("nil primitive receiver succeeded")
	}
	var enum *McpElicitationEnumSchema
	if err := enum.UnmarshalJSON([]byte(`{"type":"string","enum":[]}`)); err == nil {
		t.Error("nil enum receiver succeeded")
	}
	var single *McpElicitationSingleSelectEnumSchema
	if err := single.UnmarshalJSON([]byte(`{"type":"string","enum":[]}`)); err == nil {
		t.Error("nil single receiver succeeded")
	}
	var multi *McpElicitationMultiSelectEnumSchema
	if err := multi.UnmarshalJSON([]byte(`{"type":"array","items":{"type":"string","enum":[]}}`)); err == nil {
		t.Error("nil multi receiver succeeded")
	}
}

func TestMcpServerElicitationParamsValidateAllModes(t *testing.T) {
	valid := []string{
		`{"threadId":"thread-1","serverName":"repo","mode":"form","message":"Choose","requestedSchema":{"type":"object","properties":{"scopes":{"type":"array","items":{"oneOf":[{"const":"read","title":"Read"}]}}}},"ignored":true}`,
		`{"threadId":"thread-1","turnId":"turn-1","serverName":"repo","mode":"openai/form","_meta":{"source":"test"},"message":"Choose","requestedSchema":{"type":"custom"},"itemId":"legacy-is-ignored"}`,
		`{"threadId":"thread-1","turnId":null,"serverName":"repo","mode":"url","_meta":null,"message":"Open","url":"https://example.com","elicitationId":"elicit-1"}`,
	}
	for _, input := range valid {
		var params McpServerElicitationRequestParams
		if err := json.Unmarshal([]byte(input), &params); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(params)
		if err != nil {
			t.Errorf("Marshal(%s): %v", input, err)
			continue
		}
		var roundTrip McpServerElicitationRequestParams
		if err := json.Unmarshal(encoded, &roundTrip); err != nil {
			t.Errorf("round-trip %s: %v", encoded, err)
		}
		if params.Mode == McpServerElicitationModeForm && strings.Contains(string(encoded), `"items":{"oneOf"`) {
			t.Errorf("form params were not canonicalized: %s", encoded)
		}
	}
	var canonical McpServerElicitationRequestParams
	if err := json.Unmarshal([]byte(`{"threadId":"thread-1","serverName":"repo","mode":"url","message":"Open","url":"https://example.com","elicitationId":"elicit-1","requestId":"legacy","extra":true}`), &canonical); err != nil {
		t.Fatalf("source-open params: %v", err)
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("marshal source-open params: %v", err)
	}
	if strings.Contains(string(encoded), `"requestId"`) || strings.Contains(string(encoded), `"extra"`) ||
		!strings.Contains(string(encoded), `"turnId":null`) || !strings.Contains(string(encoded), `"_meta":null`) {
		t.Fatalf("source-open params canonicalized as %s", encoded)
	}
	invalid := []string{
		`{}`,
		`{"threadId":"thread-1","turnId":null,"serverName":"repo","mode":"form","_meta":null,"message":"Choose"}`,
		`{"threadId":"thread-1","turnId":null,"serverName":"repo","mode":"form","_meta":null,"message":"Choose","requestedSchema":{"type":"object"}}`,
		`{"threadId":"thread-1","turnId":null,"serverName":"repo","mode":"url","_meta":null,"message":"Open","url":"https://example.com"}`,
		`{"threadId":"thread-1","turnId":null,"serverName":"repo","mode":"other","_meta":null,"message":"Choose"}`,
		`{"threadId":"thread-1","serverName":"repo","mode":"url","message":"Open","url":"https://example.com","elicitationId":"id","threadId":"duplicate"}`,
	}
	for _, input := range invalid {
		var params McpServerElicitationRequestParams
		if err := json.Unmarshal([]byte(input), &params); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	var params *McpServerElicitationRequestParams
	if err := params.UnmarshalJSON([]byte(valid[0])); err == nil {
		t.Error("nil params receiver succeeded")
	}
	if _, err := json.Marshal(McpServerElicitationRequestParams{}); err == nil {
		t.Error("zero-value params marshal succeeded")
	}
}

func TestMcpServerElicitationResponseMatchesSourceSerde(t *testing.T) {
	for _, input := range []string{
		`{"action":"accept","content":{"choice":"safe"},"_meta":null}`,
		`{"action":"decline","content":null,"_meta":{"reason":"policy"}}`,
		`{"action":"cancel","content":null,"_meta":null}`,
	} {
		var response McpServerElicitationRequestResponse
		if err := json.Unmarshal([]byte(input), &response); err != nil {
			t.Errorf("Unmarshal(%s): %v", input, err)
		}
	}
	for _, input := range []string{
		`{"action":"accept"}`,
		`{"action":"cancel","content":null,"_meta":null,"extra":true}`,
	} {
		var response McpServerElicitationRequestResponse
		if err := json.Unmarshal([]byte(input), &response); err != nil {
			t.Errorf("source-open Unmarshal(%s): %v", input, err)
			continue
		}
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Errorf("source-open Marshal(%s): %v", input, err)
			continue
		}
		if string(encoded) != `{"action":"accept","content":null,"_meta":null}` &&
			string(encoded) != `{"action":"cancel","content":null,"_meta":null}` {
			t.Errorf("source-open response canonicalized as %s", encoded)
		}
	}
	for _, input := range []string{
		`[`,
		`{}`,
		`{"action":"approve","content":null,"_meta":null}`,
		`{"action":null,"content":null,"_meta":null}`,
		`{"action":"accept","action":"cancel"}`,
	} {
		var response McpServerElicitationRequestResponse
		if err := json.Unmarshal([]byte(input), &response); err == nil {
			t.Errorf("Unmarshal(%s) succeeded", input)
		}
	}
	encoded, err := json.Marshal(McpServerElicitationRequestResponse{Action: McpServerElicitationCancel})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(encoded) != `{"action":"cancel","content":null,"_meta":null}` {
		t.Fatalf("response = %s", encoded)
	}
	if _, err := json.Marshal(McpServerElicitationRequestResponse{Action: "approve"}); err == nil {
		t.Error("invalid response marshal succeeded")
	}
	var response *McpServerElicitationRequestResponse
	if err := response.UnmarshalJSON([]byte(`{"action":"cancel","content":null,"_meta":null}`)); err == nil {
		t.Error("nil response receiver succeeded")
	}
}

func TestMcpElicitationSchemaMarshalRejectsInvalidValues(t *testing.T) {
	if _, err := json.Marshal(McpElicitationSchema{}); err == nil {
		t.Error("zero-value schema marshal succeeded")
	}
	var schema *McpElicitationSchema
	if err := schema.UnmarshalJSON([]byte(`{"type":"object","properties":{}}`)); err == nil {
		t.Error("nil schema receiver succeeded")
	}
}
