package core

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
)

// Regression tests for bugs found in the June 2026 audit.

// collectToolCallPairing returns the set of tool call IDs issued in
// responses and the set of tool call IDs answered by a ToolReturnPart or
// RetryPromptPart in requests.
func collectToolCallPairing(messages []ModelMessage) (called, answered map[string]bool) {
	called = make(map[string]bool)
	answered = make(map[string]bool)
	for _, msg := range messages {
		switch m := msg.(type) {
		case ModelResponse:
			for _, tc := range m.ToolCalls() {
				called[tc.ToolCallID] = true
			}
		case ModelRequest:
			for _, part := range m.Parts {
				switch p := part.(type) {
				case ToolReturnPart:
					answered[p.ToolCallID] = true
				case RetryPromptPart:
					if p.ToolCallID != "" {
						answered[p.ToolCallID] = true
					}
				}
			}
		}
	}
	return called, answered
}

func assertNoDanglingToolCalls(t *testing.T, messages []ModelMessage) {
	t.Helper()
	called, answered := collectToolCallPairing(messages)
	for id := range called {
		if !answered[id] {
			t.Errorf("tool call %q has no tool result in history", id)
		}
	}
}

type bugfixAnswer struct {
	Answer string `json:"answer"`
}

// Run must record a tool result for the final_result call so
// RunResult.Messages round-trips through WithMessages without dangling
// tool_use blocks.
func TestRunFinalResultToolReturnRecorded(t *testing.T) {
	for _, strategy := range []EndStrategy{EndStrategyEarly, EndStrategyExhaustive} {
		t.Run(string(strategy), func(t *testing.T) {
			model := NewTestModel(ToolCallResponse("final_result", `{"answer":"42"}`))
			agent := NewAgent[bugfixAnswer](model, WithEndStrategy[bugfixAnswer](strategy))

			result, err := agent.Run(context.Background(), "q")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			assertNoDanglingToolCalls(t, result.Messages)

			last, ok := result.Messages[len(result.Messages)-1].(ModelRequest)
			if !ok {
				t.Fatalf("last message = %T, want ModelRequest with tool result", result.Messages[len(result.Messages)-1])
			}
			found := false
			for _, part := range last.Parts {
				if p, ok := part.(ToolReturnPart); ok && p.ToolName == "final_result" {
					found = true
				}
			}
			if !found {
				t.Error("final_result tool call was not answered in history")
			}
		})
	}
}

// RunStream must record the same final-turn tool results as Run.
func TestRunStreamFinalResultToolReturnRecorded(t *testing.T) {
	model := NewTestModel(ToolCallResponse("final_result", `{"answer":"42"}`))
	agent := NewAgent[bugfixAnswer](model)

	stream, err := agent.RunStream(context.Background(), "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	result, err := stream.Result()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertNoDanglingToolCalls(t, result.Messages)
}

// In exhaustive mode, function tools executed in the final turn must have
// their results recorded in history alongside the final_result answer.
func TestRunExhaustiveFinalTurnToolResultsRecorded(t *testing.T) {
	type addParams struct {
		A int `json:"a"`
		B int `json:"b"`
	}
	executed := false
	addTool := FuncTool[addParams]("add", "Add numbers",
		func(_ context.Context, p addParams) (string, error) {
			executed = true
			return "3", nil
		},
	)
	model := NewTestModel(
		MultiToolCallResponse(
			ToolCallPart{ToolName: "add", ArgsJSON: `{"a":1,"b":2}`, ToolCallID: "call_add"},
			ToolCallPart{ToolName: "final_result", ArgsJSON: `{"answer":"3"}`, ToolCallID: "call_final"},
		),
	)
	agent := NewAgent[bugfixAnswer](model,
		WithTools[bugfixAnswer](addTool),
		WithEndStrategy[bugfixAnswer](EndStrategyExhaustive),
	)

	result, err := agent.Run(context.Background(), "add 1+2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !executed {
		t.Fatal("add tool was not executed in exhaustive mode")
	}
	assertNoDanglingToolCalls(t, result.Messages)

	_, answered := collectToolCallPairing(result.Messages)
	if !answered["call_add"] {
		t.Error("executed tool's result missing from history")
	}
	if !answered["call_final"] {
		t.Error("final_result call missing its result in history")
	}
}

// Run must not mutate the agent's stored ModelSettings when injecting the
// agent-level tool choice, and concurrent runs must not race on it.
func TestRunDoesNotMutateAgentModelSettings(t *testing.T) {
	model := NewTestModel(TextResponse("done"))
	agent := NewAgent[string](model,
		WithModelSettings[string](ModelSettings{}),
		WithToolChoice[string](ToolChoiceAuto()),
	)

	if _, err := agent.Run(context.Background(), "q"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent.modelSettings.ToolChoice != nil {
		t.Error("Run leaked ToolChoice into the agent's shared ModelSettings")
	}

	// Concurrent runs against one agent must be race-free (run with -race).
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = agent.Run(context.Background(), "q")
		}()
	}
	wg.Wait()
	if agent.modelSettings.ToolChoice != nil {
		t.Error("concurrent Run leaked ToolChoice into the agent's ModelSettings")
	}
}

// ProviderMetadataPart must round-trip through message serialization.
func TestProviderMetadataPartSerializeRoundTrip(t *testing.T) {
	original := []ModelMessage{
		ModelResponse{
			Parts: []ModelResponsePart{
				TextPart{Content: "hi"},
				ProviderMetadataPart{
					Provider: "anthropic",
					Kind:     "server_tool_use",
					Payload:  json.RawMessage(`{"id":"srvtoolu_1","name":"web_search"}`),
				},
				ProviderMetadataPart{Provider: "openai", Kind: "annotation"},
			},
		},
	}

	data, err := MarshalMessages(original)
	if err != nil {
		t.Fatalf("MarshalMessages: %v", err)
	}
	decoded, err := UnmarshalMessages(data)
	if err != nil {
		t.Fatalf("UnmarshalMessages: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded %d messages, want 1", len(decoded))
	}
	resp, ok := decoded[0].(ModelResponse)
	if !ok {
		t.Fatalf("decoded message = %T, want ModelResponse", decoded[0])
	}
	if len(resp.Parts) != 3 {
		t.Fatalf("decoded %d parts, want 3", len(resp.Parts))
	}
	meta, ok := resp.Parts[1].(ProviderMetadataPart)
	if !ok {
		t.Fatalf("part 1 = %T, want ProviderMetadataPart", resp.Parts[1])
	}
	if meta.Provider != "anthropic" || meta.Kind != "server_tool_use" {
		t.Errorf("provider/kind = %q/%q, want anthropic/server_tool_use", meta.Provider, meta.Kind)
	}
	if string(meta.Payload) != `{"id":"srvtoolu_1","name":"web_search"}` {
		t.Errorf("payload = %s", meta.Payload)
	}
	empty, ok := resp.Parts[2].(ProviderMetadataPart)
	if !ok || empty.Provider != "openai" {
		t.Errorf("empty-payload part did not round-trip: %#v", resp.Parts[2])
	}
}

// Clone must preserve the tool output truncation config.
func TestCloneCopiesTruncationConfig(t *testing.T) {
	model := NewTestModel(TextResponse("done"))
	agent := NewAgent[string](model,
		WithToolOutputTruncation[string](DefaultTruncationConfig()),
	)
	clone := agent.Clone()
	if clone.truncationConfig == nil {
		t.Fatal("Clone dropped truncationConfig")
	}
	if *clone.truncationConfig != *agent.truncationConfig {
		t.Errorf("clone truncationConfig = %+v, want %+v", *clone.truncationConfig, *agent.truncationConfig)
	}
}

type selfEmbed struct {
	*selfEmbed
	Name string `json:"name"`
}

type mutualA struct {
	*mutualB
	A string `json:"a"`
}

type mutualB struct {
	*mutualA
	B string `json:"b"`
}

// SchemaFor must not overflow the stack on self- or mutually-embedded types.
func TestSchemaSelfEmbeddedNoOverflow(t *testing.T) {
	var se selfEmbed
	se.selfEmbed = &selfEmbed{Name: "parent"}
	if se.selfEmbed == nil {
		t.Fatal("self-embedded field should be addressable")
	}

	schema := SchemaFor[selfEmbed]()
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Errorf("schema missing 'name' property: %v", schema)
	}

	schema = SchemaFor[mutualA]()
	props, _ = schema["properties"].(map[string]any)
	if _, ok := props["a"]; !ok {
		t.Errorf("schema missing 'a' property: %v", schema)
	}
	if _, ok := props["b"]; !ok {
		t.Errorf("schema missing promoted 'b' property: %v", schema)
	}
}

type embedInner struct {
	Val string `json:"val"`
}

type embedTagged struct {
	embedInner `json:"inner"`
}

type embedFlat struct {
	embedInner
	Extra string `json:"extra"`
}

type embedUnexported struct {
	hiddenInner
}

type hiddenInner struct {
	Shown string `json:"shown"`
}

// Schema generation must match encoding/json embedding semantics: a name
// tag nests the embedded struct, no tag flattens it, and exported fields
// of unexported embedded struct types are promoted.
func TestSchemaEmbeddedMatchesEncodingJSON(t *testing.T) {
	schema := SchemaFor[embedTagged]()
	props, _ := schema["properties"].(map[string]any)
	inner, ok := props["inner"].(Schema)
	if !ok {
		t.Fatalf("tagged embedded struct not nested under 'inner': %v", schema)
	}
	innerProps, _ := inner["properties"].(map[string]any)
	if _, ok := innerProps["val"]; !ok {
		t.Errorf("nested schema missing 'val': %v", inner)
	}
	if _, leaked := props["val"]; leaked {
		t.Errorf("tagged embedded struct was flattened: %v", schema)
	}
	// Sanity-check against encoding/json behavior.
	encoded, _ := json.Marshal(embedTagged{embedInner{Val: "x"}})
	var asMap map[string]any
	_ = json.Unmarshal(encoded, &asMap)
	if _, ok := asMap["inner"]; !ok {
		t.Fatalf("encoding/json baseline changed: %s", encoded)
	}

	schema = SchemaFor[embedFlat]()
	props, _ = schema["properties"].(map[string]any)
	if _, ok := props["val"]; !ok {
		t.Errorf("untagged embedded struct not flattened: %v", schema)
	}

	schema = SchemaFor[embedUnexported]()
	props, _ = schema["properties"].(map[string]any)
	if _, ok := props["shown"]; !ok {
		t.Errorf("exported field of unexported embedded struct not promoted: %v", schema)
	}
}

// Variables must see fields in trim-marker actions, control flow, and
// pipelines, as its contract promises.
func TestTemplateVariablesTreeWalk(t *testing.T) {
	cases := []struct {
		tmpl string
		want []string
	}{
		{"Hello {{.Name}}!", []string{"Name"}},
		{"Hello {{- .Name -}} !", []string{"Name"}},
		{"{{if .Flag}}yes{{else}}{{.Alt}}{{end}}", []string{"Alt", "Flag"}},
		{"{{range .Items}}{{.}}{{end}}", []string{"Items"}},
		{"{{with .X}}{{.}}{{end}}", []string{"X"}},
		{`{{.X | printf "%s"}}`, []string{"X"}},
		{`{{printf "%s %s" .A .B}}`, []string{"A", "B"}},
		{"{{.Outer.Inner}}", []string{"Outer"}},
		{"no vars", nil},
	}
	for _, tc := range cases {
		pt, err := NewPromptTemplate("t", tc.tmpl)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.tmpl, err)
		}
		got := pt.Variables()
		if len(got) == 0 && len(tc.want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Variables(%q) = %v, want %v", tc.tmpl, got, tc.want)
		}
	}
}
