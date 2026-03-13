//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
)

func openAIOnly(t *testing.T) {
	t.Helper()
	skipIfNoCredentials(t, "OPENAI_API_KEY")
}

func TestOpenAIRunStreamStructuredOutputToolMode(t *testing.T) {
	openAIOnly(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var addCalls int
	var validatorCalls int

	addTool := core.FuncTool[CalcParams]("add", "Add two numbers together", func(ctx context.Context, rc *core.RunContext, p CalcParams) (string, error) {
		addCalls++
		return fmt.Sprintf("%d", p.A+p.B), nil
	})

	agent := core.NewAgent[MathAnswer](newOpenAIProvider(),
		core.WithTools[MathAnswer](addTool),
		core.WithMaxTokens[MathAnswer](250),
		core.WithOutputValidator(func(ctx context.Context, rc *core.RunContext, output MathAnswer) (MathAnswer, error) {
			validatorCalls++
			return output, nil
		}),
	)

	stream, err := agent.RunStream(ctx, "Use the add tool exactly once to add 20 and 22. After the tool returns, provide structured output with answer=42 and a brief explanation.")
	if err != nil {
		skipOnAccountError(t, err)
		t.Fatalf("RunStream failed: %v", err)
	}
	defer stream.Close()

	var (
		partialEvents int
		sawToolCall   bool
	)
	for event, err := range stream.StreamEvents() {
		if err != nil {
			t.Fatalf("StreamEvents error: %v", err)
		}
		partialEvents++
		if start, ok := event.(core.PartStartEvent); ok {
			if tc, ok := start.Part.(core.ToolCallPart); ok && tc.ToolName == "add" {
				sawToolCall = true
			}
		}
		if partialEvents >= 3 {
			break
		}
	}

	result, err := stream.Result()
	if err != nil {
		skipOnAccountError(t, err)
		t.Fatalf("Result failed: %v", err)
	}
	if result.Output.Answer != 42 {
		t.Fatalf("expected answer=42, got %d", result.Output.Answer)
	}
	if result.Output.Explanation == "" {
		t.Fatal("expected non-empty explanation")
	}
	if addCalls != 1 {
		t.Fatalf("expected add tool to run once, got %d", addCalls)
	}
	if validatorCalls == 0 {
		t.Fatal("expected output validator to be called")
	}
	if result.Usage.ToolCalls < 1 {
		t.Fatalf("expected at least 1 tool call, got %d", result.Usage.ToolCalls)
	}
	if result.Usage.Requests < 2 {
		t.Fatalf("expected at least 2 model requests, got %d", result.Usage.Requests)
	}
	if !sawToolCall {
		t.Fatal("expected to observe the add tool call in the streamed events")
	}
	if !hasResponseToolCall(result.Messages, "add") {
		t.Fatal("expected result messages to contain the add tool call")
	}
	if !hasRequestToolReturn(result.Messages, "add") {
		t.Fatal("expected result messages to contain the add tool return")
	}
	if !hasResponseToolCall(result.Messages, core.DefaultOutputToolName) {
		t.Fatal("expected result messages to contain the final_result tool call")
	}

	resp, err := stream.GetOutput()
	if err != nil {
		t.Fatalf("GetOutput failed after Result: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil final response")
	}
	if resp.ModelName == "" {
		t.Fatal("expected final response model name to be set")
	}
	if len(resp.ToolCalls()) == 0 {
		t.Fatal("expected final response to contain at least one tool call")
	}
	if resp.ToolCalls()[0].ToolName != core.DefaultOutputToolName {
		t.Fatalf("expected final response tool call %q, got %q", core.DefaultOutputToolName, resp.ToolCalls()[0].ToolName)
	}

	t.Logf("Tool mode stream passed: partialEvents=%d requests=%d toolCalls=%d answer=%d explanation=%q",
		partialEvents, result.Usage.Requests, result.Usage.ToolCalls, result.Output.Answer, result.Output.Explanation)
}

func TestOpenAIRunStreamStructuredOutputNativeMode(t *testing.T) {
	openAIOnly(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var validatorCalls int

	agent := core.NewAgent[MathAnswer](newOpenAIProvider(),
		core.WithOutputOptions[MathAnswer](core.WithOutputMode(core.OutputModeNative)),
		core.WithMaxTokens[MathAnswer](250),
		core.WithOutputValidator(func(ctx context.Context, rc *core.RunContext, output MathAnswer) (MathAnswer, error) {
			validatorCalls++
			return output, nil
		}),
	)

	stream, err := agent.RunStream(ctx, "What is 15 + 27? Return structured output with answer=42 and a brief explanation.")
	if err != nil {
		skipOnAccountError(t, err)
		t.Fatalf("RunStream failed: %v", err)
	}
	defer stream.Close()

	var (
		partialEvents int
		partialText   string
	)
	for event, err := range stream.StreamEvents() {
		if err != nil {
			t.Fatalf("StreamEvents error: %v", err)
		}
		partialEvents++
		if start, ok := event.(core.PartStartEvent); ok {
			if tp, ok := start.Part.(core.TextPart); ok {
				partialText += tp.Content
			}
		}
		if delta, ok := event.(core.PartDeltaEvent); ok {
			if td, ok := delta.Delta.(core.TextPartDelta); ok {
				partialText += td.ContentDelta
			}
		}
		if partialEvents >= 2 {
			break
		}
	}

	result, err := stream.Result()
	if err != nil {
		skipOnAccountError(t, err)
		t.Fatalf("Result failed: %v", err)
	}
	if result.Output.Answer != 42 {
		t.Fatalf("expected answer=42, got %d", result.Output.Answer)
	}
	if result.Output.Explanation == "" {
		t.Fatal("expected non-empty explanation")
	}
	if validatorCalls == 0 {
		t.Fatal("expected output validator to be called")
	}
	if partialEvents == 0 {
		t.Fatal("expected to consume at least one partial stream event")
	}
	usedOutputTool := hasResponseToolCall(result.Messages, core.DefaultOutputToolName)

	resp, err := stream.GetOutput()
	if err != nil {
		t.Fatalf("GetOutput failed after Result: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil final response")
	}
	if usedOutputTool {
		if len(resp.ToolCalls()) == 0 {
			t.Fatal("expected final_result tool call in final response when native mode falls back to output tool")
		}
		if resp.ToolCalls()[0].ToolName != core.DefaultOutputToolName {
			t.Fatalf("expected fallback tool call %q, got %q", core.DefaultOutputToolName, resp.ToolCalls()[0].ToolName)
		}
	} else {
		if len(resp.ToolCalls()) != 0 {
			t.Fatalf("expected no tool calls in native text path, got %d", len(resp.ToolCalls()))
		}
		if resp.TextContent() == "" {
			t.Fatal("expected native text path final response to contain text content")
		}
	}

	path := "native-text"
	if usedOutputTool {
		path = "output-tool-fallback"
	}
	t.Logf("Native mode stream passed: path=%s partialEvents=%d partialText=%q answer=%d explanation=%q",
		path, partialEvents, partialText, result.Output.Answer, result.Output.Explanation)
}

func hasResponseToolCall(messages []core.ModelMessage, toolName string) bool {
	for _, msg := range messages {
		resp, ok := msg.(core.ModelResponse)
		if !ok {
			continue
		}
		for _, part := range resp.Parts {
			tc, ok := part.(core.ToolCallPart)
			if ok && tc.ToolName == toolName {
				return true
			}
		}
	}
	return false
}

func hasRequestToolReturn(messages []core.ModelMessage, toolName string) bool {
	for _, msg := range messages {
		req, ok := msg.(core.ModelRequest)
		if !ok {
			continue
		}
		for _, part := range req.Parts {
			tr, ok := part.(core.ToolReturnPart)
			if ok && tr.ToolName == toolName {
				return true
			}
		}
	}
	return false
}
