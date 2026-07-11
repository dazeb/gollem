package protocol

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestRawResponseContentPrerequisiteSchemasAreExact(t *testing.T) {
	defs := JSONSchema()["$defs"].(Schema)
	assertRawResponseContentSchema(t, defs, "AgentMessageInputContent", []rawResponseContentVariantExpectation{
		{contentType: "input_text", field: "text"},
		{contentType: "encrypted_content", field: "encrypted_content"},
	})
	assertRawResponseContentSchema(t, defs, "ReasoningItemContent", []rawResponseContentVariantExpectation{
		{contentType: "reasoning_text", field: "text"},
		{contentType: "text", field: "text"},
	})
	assertRawResponseContentSchema(t, defs, "ReasoningItemReasoningSummary", []rawResponseContentVariantExpectation{
		{contentType: "summary_text", field: "text"},
	})
}

func TestRawResponseContentPrerequisiteWireValidation(t *testing.T) {
	tests := []struct {
		name    string
		valid   []string
		invalid []string
		decode  func([]byte) (json.Marshaler, error)
		empty   json.Marshaler
		nilCall func() error
	}{
		{
			name:  "agent message input content",
			valid: []string{`{"type":"input_text","text":"hello"}`, `{"type":"input_text","text":""}`, `{"type":"encrypted_content","encrypted_content":"cipher"}`},
			invalid: []string{
				`null`, `[]`, `{}`,
				`{"type":"input_text"}`,
				`{"type":"input_text","text":null}`,
				`{"type":"input_text","text":"hello","encrypted_content":"cipher"}`,
				`{"type":"encrypted_content"}`,
				`{"type":"encrypted_content","encrypted_content":null}`,
				`{"type":"encrypted_content","encryptedContent":"cipher"}`,
				`{"type":"text","text":"hello"}`,
				`{"type":1,"text":"hello"}`,
				`{"type":"input_text","text":"hello","extra":true}`,
			},
			decode: func(data []byte) (json.Marshaler, error) {
				var value AgentMessageInputContent
				err := json.Unmarshal(data, &value)
				return value, err
			},
			empty: AgentMessageInputContent{},
			nilCall: func() error {
				var value *AgentMessageInputContent
				return value.UnmarshalJSON([]byte(`{"type":"input_text","text":"hello"}`))
			},
		},
		{
			name:  "reasoning item content",
			valid: []string{`{"type":"reasoning_text","text":"thinking"}`, `{"type":"text","text":"visible"}`, `{"type":"text","text":""}`},
			invalid: []string{
				`null`, `[]`, `{}`,
				`{"type":"reasoning_text"}`,
				`{"type":"reasoning_text","text":null}`,
				`{"type":"reasoningText","text":"thinking"}`,
				`{"type":"text","encrypted_content":"cipher"}`,
				`{"type":"text","text":"visible","extra":true}`,
			},
			decode: func(data []byte) (json.Marshaler, error) {
				var value ReasoningItemContent
				err := json.Unmarshal(data, &value)
				return value, err
			},
			empty: ReasoningItemContent{},
			nilCall: func() error {
				var value *ReasoningItemContent
				return value.UnmarshalJSON([]byte(`{"type":"text","text":"visible"}`))
			},
		},
		{
			name:  "reasoning summary content",
			valid: []string{`{"type":"summary_text","text":"summary"}`, `{"type":"summary_text","text":""}`},
			invalid: []string{
				`null`, `[]`, `{}`,
				`{"type":"summary_text"}`,
				`{"type":"summary_text","text":null}`,
				`{"type":"summaryText","text":"summary"}`,
				`{"type":"text","text":"summary"}`,
				`{"type":"summary_text","text":"summary","extra":true}`,
			},
			decode: func(data []byte) (json.Marshaler, error) {
				var value ReasoningItemReasoningSummary
				err := json.Unmarshal(data, &value)
				return value, err
			},
			empty: ReasoningItemReasoningSummary{},
			nilCall: func() error {
				var value *ReasoningItemReasoningSummary
				return value.UnmarshalJSON([]byte(`{"type":"summary_text","text":"summary"}`))
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			for _, input := range testCase.valid {
				value, err := testCase.decode([]byte(input))
				if err != nil {
					t.Errorf("Unmarshal(%s): %v", input, err)
					continue
				}
				encoded, err := json.Marshal(value)
				if err != nil || string(encoded) != input {
					t.Errorf("round trip %s = %s, %v", input, encoded, err)
				}
			}
			for _, input := range testCase.invalid {
				if _, err := testCase.decode([]byte(input)); err == nil {
					t.Errorf("Unmarshal(%s) succeeded", input)
				}
			}
			if _, err := json.Marshal(testCase.empty); err == nil {
				t.Fatal("Marshal empty value succeeded")
			}
			if err := testCase.nilCall(); err == nil {
				t.Fatal("nil receiver succeeded")
			}
		})
	}
}

type rawResponseContentVariantExpectation struct {
	contentType string
	field       string
}

func assertRawResponseContentSchema(t *testing.T, defs Schema, name string, want []rawResponseContentVariantExpectation) {
	t.Helper()
	schema, ok := defs[name].(Schema)
	if !ok {
		t.Fatalf("$defs missing %s", name)
	}
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) != len(want) {
		t.Fatalf("%s variants = %#v", name, schema["oneOf"])
	}
	for index, expected := range want {
		variant := variants[index].(Schema)
		if variant["additionalProperties"] != false {
			t.Fatalf("%s %s allows extra fields", name, expected.contentType)
		}
		properties := variant["properties"].(Schema)
		if !reflect.DeepEqual(properties["type"].(Schema)["enum"], []any{expected.contentType}) {
			t.Fatalf("%s variant %d type = %#v", name, index, properties["type"])
		}
		if !slices.Equal(schemaRequiredNames(variant), []string{"type", expected.field}) {
			t.Fatalf("%s %s required = %v", name, expected.contentType, schemaRequiredNames(variant))
		}
		if !reflect.DeepEqual(properties[expected.field], Schema{"type": "string"}) {
			t.Fatalf("%s %s field = %#v", name, expected.contentType, properties[expected.field])
		}
	}
}
