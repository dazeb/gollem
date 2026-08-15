package mcp

import (
	"encoding/json"
	"testing"
)

func TestCompleteToolInputSchema(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty becomes bare object schema", "", `{"type":"object"}`},
		{"whitespace becomes bare object schema", " \n\t ", `{"type":"object"}`},
		{"already typed passes through", `{"type":"object","properties":{"a":{"type":"string"}}}`, `{"type":"object","properties":{"a":{"type":"string"}}}`},
		{"bare properties gains object type", `{"properties":{"run_id":{"type":"string"}},"required":["run_id"]}`, `{"properties":{"run_id":{"type":"string"}},"required":["run_id"],"type":"object"}`},
		{"malformed passes through", `{`, `{`},
		{"array root passes through", `[1,2]`, `[1,2]`},
		{"string root passes through", `"hi"`, `"hi"`},
		{"null root completes", `null`, `{"type":"object"}`},
		{"whitespace padded null completes", ` null 
`, `{"type":"object"}`},
		{"boolean root passes through", `true`, `true`},
		{"ref root passes through", `{"$ref":"#/definitions/x"}`, `{"$ref":"#/definitions/x"}`},
		{"oneOf root passes through", `{"oneOf":[{"type":"string"},{"type":"number"}]}`, `{"oneOf":[{"type":"string"},{"type":"number"}]}`},
		{"anyOf root passes through", `{"anyOf":[{"type":"string"}]}`, `{"anyOf":[{"type":"string"}]}`},
		{"allOf root passes through", `{"allOf":[{"type":"object"}]}`, `{"allOf":[{"type":"object"}]}`},
		{"not root passes through", `{"not":{"type":"null"}}`, `{"not":{"type":"null"}}`},
		{"enum root passes through", `{"enum":[1,2]}`, `{"enum":[1,2]}`},
		{"const root passes through", `{"const":5}`, `{"const":5}`},
		{"minimum root passes through", `{"minimum":0}`, `{"minimum":0}`},
		{"items root passes through", `{"items":{"type":"string"}}`, `{"items":{"type":"string"}}`},
		{"if root passes through", `{"if":{"const":5},"then":false}`, `{"if":{"const":5},"then":false}`},
		{"empty object completes", `{}`, `{"type":"object"}`},
		{"number root passes through", `42`, `42`},
		{"false schema passes through", `false`, `false`},
		{"properties only completes", `{"properties":{}}`, `{"properties":{},"type":"object"}`},
		{"required only completes", `{"required":["a"]}`, `{"required":["a"],"type":"object"}`},
		{"additionalProperties only completes", `{"additionalProperties":true}`, `{"additionalProperties":true,"type":"object"}`},
		{"patternProperties only completes", `{"patternProperties":{"^x":{}}}`, `{"patternProperties":{"^x":{}},"type":"object"}`},
		{"minProperties only completes", `{"minProperties":1}`, `{"minProperties":1,"type":"object"}`},
		{"draft7 dependencies completes", `{"dependencies":{"a":["b"]}}`, `{"dependencies":{"a":["b"]},"type":"object"}`},
		{"annotations with object keyword complete", `{"title":"t","properties":{}}`, `{"properties":{},"title":"t","type":"object"}`},
		{"mixed object and value keyword passes through", `{"properties":{},"minimum":0}`, `{"properties":{},"minimum":0}`},
		{"mixed object and combinator passes through", `{"required":["a"],"oneOf":[{"type":"string"}]}`, `{"required":["a"],"oneOf":[{"type":"string"}]}`},
		{"annotations alone pass through", `{"title":"t"}`, `{"title":"t"}`},
		{"array form type passes through", `{"type":["object","null"]}`, `{"type":["object","null"]}`},
		{"dependentRequired only completes", `{"dependentRequired":{"a":["b"]}}`, `{"dependentRequired":{"a":["b"]},"type":"object"}`},
		{"dependentSchemas only completes", `{"dependentSchemas":{"a":{"type":"string"}}}`, `{"dependentSchemas":{"a":{"type":"string"}},"type":"object"}`},
		{"maxProperties only completes", `{"maxProperties":2}`, `{"maxProperties":2,"type":"object"}`},
		{"propertyNames only completes", `{"propertyNames":{"minLength":1}}`, `{"propertyNames":{"minLength":1},"type":"object"}`},
		{"unevaluatedProperties only completes", `{"unevaluatedProperties":false}`, `{"type":"object","unevaluatedProperties":false}`},
		{"schema and defs with object keyword complete", `{"$schema":"x","$defs":{},"properties":{}}`, `{"$defs":{},"$schema":"x","properties":{},"type":"object"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(completeToolInputSchema(json.RawMessage(tc.in)))
			if got != tc.want {
				t.Errorf("completeToolInputSchema(%q) = %s, want %s", tc.in, got, tc.want)
			}
		})
	}
}
