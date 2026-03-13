//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/provider/anthropic"
	"github.com/fugue-labs/gollem/provider/openai"
)

// TestSmokeTextRequest verifies basic non-streaming text request for all providers.
func TestSmokeTextRequest(t *testing.T) {
	for _, p := range allProviders() {
		t.Run(p.name, func(t *testing.T) {
			skipIfNoCredentials(t, p.credEnvVar)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			model := p.newFn()

			messages := []core.ModelMessage{
				core.ModelRequest{
					Parts: []core.ModelRequestPart{
						core.UserPromptPart{Content: "Respond with exactly the word 'hello' and nothing else."},
					},
				},
			}

			resp, err := model.Request(ctx, messages, nil, nil)
			if err != nil {
				skipOnAccountError(t, err)
				t.Fatalf("Request failed: %v", err)
			}

			if resp == nil {
				t.Fatal("response is nil")
			}
			if len(resp.Parts) == 0 {
				t.Fatal("response has no parts")
			}

			text := resp.TextContent()
			if !strings.Contains(strings.ToLower(text), "hello") {
				t.Errorf("expected response to contain 'hello', got: %q", text)
			}

			if resp.ModelName == "" {
				t.Error("ModelName is empty")
			}

			t.Logf("Provider=%s Model=%s Text=%q InputTokens=%d OutputTokens=%d",
				p.name, resp.ModelName, text, resp.Usage.InputTokens, resp.Usage.OutputTokens)
		})
	}
}

// TestSmokeStream verifies basic streaming for all providers.
func TestSmokeStream(t *testing.T) {
	for _, p := range allProviders() {
		t.Run(p.name, func(t *testing.T) {
			skipIfNoCredentials(t, p.credEnvVar)

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			model := p.newFn()

			messages := []core.ModelMessage{
				core.ModelRequest{
					Parts: []core.ModelRequestPart{
						core.UserPromptPart{Content: "Respond with exactly the word 'hello' and nothing else."},
					},
				},
			}

			stream, err := model.RequestStream(ctx, messages, nil, nil)
			if err != nil {
				skipOnAccountError(t, err)
				t.Fatalf("RequestStream failed: %v", err)
			}
			defer stream.Close()

			var (
				partStarts int
				partDeltas int
			)

			for {
				event, err := stream.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("stream.Next() error: %v", err)
				}
				switch event.(type) {
				case core.PartStartEvent:
					partStarts++
				case core.PartDeltaEvent:
					partDeltas++
				}
			}

			resp := stream.Response()
			if resp == nil {
				t.Fatal("stream.Response() is nil after consuming stream")
			}

			text := resp.TextContent()
			if !strings.Contains(strings.ToLower(text), "hello") {
				t.Errorf("expected streamed response to contain 'hello', got: %q", text)
			}

			if partStarts == 0 {
				t.Error("received zero PartStartEvents")
			}

			usage := stream.Usage()
			t.Logf("Provider=%s Text=%q PartStarts=%d PartDeltas=%d InputTokens=%d OutputTokens=%d",
				p.name, text, partStarts, partDeltas, usage.InputTokens, usage.OutputTokens)
		})
	}
}

// TestSmokeAgentRunStream verifies the full public streaming agent path:
// partial event consumption, tool execution, structured output, and final response access.
func TestSmokeAgentRunStream(t *testing.T) {
	addTool := core.FuncTool[CalcParams]("add", "Add two numbers together", func(ctx context.Context, rc *core.RunContext, p CalcParams) (string, error) {
		return fmt.Sprintf("%d", p.A+p.B), nil
	})

	for _, p := range allProviders() {
		t.Run(p.name, func(t *testing.T) {
			skipIfNoCredentials(t, p.credEnvVar)

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			agent := core.NewAgent[MathAnswer](p.newFn(),
				core.WithTools[MathAnswer](addTool),
				core.WithMaxTokens[MathAnswer](250),
			)

			stream, err := agent.RunStream(ctx, "Use the add tool exactly once to add 20 and 22. Then return structured output with answer=42 and a brief explanation.")
			if err != nil {
				skipOnAccountError(t, err)
				t.Fatalf("agent.RunStream failed: %v", err)
			}
			defer stream.Close()

			var (
				eventCount   int
				sawToolCall  bool
				sawTextStart bool
			)

			// Consume only part of the stream first, then switch to Result().
			for event, err := range stream.StreamEvents() {
				if err != nil {
					t.Fatalf("StreamEvents error: %v", err)
				}
				eventCount++
				switch e := event.(type) {
				case core.PartStartEvent:
					switch e.Part.(type) {
					case core.ToolCallPart:
						sawToolCall = true
					case core.TextPart:
						sawTextStart = true
					}
				case core.PartDeltaEvent:
					if _, ok := e.Delta.(core.TextPartDelta); ok {
						sawTextStart = true
					}
				}
				if eventCount >= 3 {
					break
				}
			}

			result, err := stream.Result()
			if err != nil {
				skipOnAccountError(t, err)
				t.Fatalf("stream.Result failed: %v", err)
			}
			if result == nil {
				t.Fatal("stream.Result returned nil result")
			}
			if result.Output.Answer != 42 {
				t.Errorf("expected answer=42, got %d", result.Output.Answer)
			}
			if result.Output.Explanation == "" {
				t.Error("expected non-empty explanation")
			}

			resp, err := stream.GetOutput()
			if err != nil {
				t.Fatalf("stream.GetOutput failed after Result: %v", err)
			}
			if resp == nil {
				t.Fatal("stream.GetOutput returned nil response")
			}
			if resp.ModelName == "" {
				t.Error("final streamed response has empty model name")
			}
			if len(resp.ToolCalls()) == 0 && resp.TextContent() == "" {
				t.Error("expected final streamed response to contain tool calls or text")
			}

			if !sawToolCall && !sawTextStart {
				t.Error("expected partial stream consumption to observe tool or text events")
			}

			t.Logf("Provider=%s PartialEvents=%d SawToolCall=%v Answer=%d Explanation=%q",
				p.name, eventCount, sawToolCall, result.Output.Answer, result.Output.Explanation)
		})
	}
}

// TestProviderAuthError verifies that invalid credentials produce proper errors.
func TestProviderAuthError(t *testing.T) {
	tests := []struct {
		name  string
		model core.Model
	}{
		{
			name:  "Anthropic",
			model: anthropic.New(anthropic.WithAPIKey("invalid-key"), anthropic.WithMaxTokens(10)),
		},
		{
			name:  "OpenAI",
			model: openai.New(openai.WithAPIKey("invalid-key"), openai.WithMaxTokens(10)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			messages := []core.ModelMessage{
				core.ModelRequest{
					Parts: []core.ModelRequestPart{
						core.UserPromptPart{Content: "test"},
					},
				},
			}

			_, err := tt.model.Request(ctx, messages, nil, nil)
			if err == nil {
				t.Fatal("expected error with invalid API key, got nil")
			}

			var httpErr *core.ModelHTTPError
			if !errors.As(err, &httpErr) {
				t.Logf("error type: %T, message: %v", err, err)
				// Non-HTTP errors are also acceptable (e.g., connection refused)
			} else {
				if httpErr.StatusCode < 400 {
					t.Errorf("expected HTTP error status >= 400, got %d", httpErr.StatusCode)
				}
				t.Logf("Got expected HTTP error: status=%d", httpErr.StatusCode)
			}
		})
	}
}
