package vertexai

import (
	"io"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

// TestParseSSEStreamFunctionCallWithThoughtSignature verifies that thought
// signatures from Gemini 3.x models are preserved through streaming.
func TestParseSSEStreamFunctionCallWithThoughtSignature(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"command":"ls -la"}},"thoughtSignature":"abc123sigXYZ"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":10}}

`
	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gemini-3-flash")

	event1, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	start, ok := event1.(core.PartStartEvent)
	if !ok {
		t.Fatalf("expected PartStartEvent, got %T", event1)
	}
	tc, ok := start.Part.(core.ToolCallPart)
	if !ok {
		t.Fatal("expected ToolCallPart")
	}
	if tc.ToolName != "bash" {
		t.Errorf("ToolName = %q, want bash", tc.ToolName)
	}
	if tc.Metadata == nil {
		t.Fatal("expected Metadata to be set for thought signature")
	}
	if sig := tc.Metadata["thoughtSignature"]; sig != "abc123sigXYZ" {
		t.Errorf("thoughtSignature = %q, want %q", sig, "abc123sigXYZ")
	}

	_, err = stream.Next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Verify final response preserves thought signature.
	resp := stream.Response()
	if len(resp.Parts) != 1 {
		t.Fatalf("expected 1 part, got %d", len(resp.Parts))
	}
	finalTc, ok := resp.Parts[0].(core.ToolCallPart)
	if !ok {
		t.Fatal("expected ToolCallPart in final response")
	}
	if finalTc.Metadata == nil {
		t.Fatal("expected Metadata in final response")
	}
	if sig := finalTc.Metadata["thoughtSignature"]; sig != "abc123sigXYZ" {
		t.Errorf("final thoughtSignature = %q, want %q", sig, "abc123sigXYZ")
	}
}

// TestParseSSEStreamMultipleFunctionCallsWithThoughtSignatures verifies
// that multiple function calls with different thought signatures are
// tracked independently.
func TestParseSSEStreamMultipleFunctionCallsWithThoughtSignatures(t *testing.T) {
	// Gemini can return multiple function calls in separate chunks.
	sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"command":"ls"}},"thoughtSignature":"sig_1"},{"functionCall":{"name":"view","args":{"file":"main.go"}},"thoughtSignature":"sig_2"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":10}}

`
	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gemini-3-flash")

	// First event: first tool call.
	event1, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	start1, ok := event1.(core.PartStartEvent)
	if !ok {
		t.Fatalf("expected PartStartEvent, got %T", event1)
	}
	tc1, ok := start1.Part.(core.ToolCallPart)
	if !ok {
		t.Fatal("expected ToolCallPart")
	}
	if tc1.ToolName != "bash" {
		t.Errorf("tc1 ToolName = %q", tc1.ToolName)
	}
	if tc1.Metadata == nil || tc1.Metadata["thoughtSignature"] != "sig_1" {
		t.Errorf("tc1 Metadata = %v", tc1.Metadata)
	}

	// Second event: second tool call.
	event2, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	start2, ok := event2.(core.PartStartEvent)
	if !ok {
		t.Fatalf("expected PartStartEvent for second call, got %T", event2)
	}
	tc2, ok := start2.Part.(core.ToolCallPart)
	if !ok {
		t.Fatal("expected ToolCallPart for second call")
	}
	if tc2.ToolName != "view" {
		t.Errorf("tc2 ToolName = %q", tc2.ToolName)
	}
	if tc2.Metadata == nil || tc2.Metadata["thoughtSignature"] != "sig_2" {
		t.Errorf("tc2 Metadata = %v", tc2.Metadata)
	}

	// Verify unique synthetic tool call IDs.
	if tc1.ToolCallID == tc2.ToolCallID {
		t.Errorf("tool call IDs should be unique, both are %q", tc1.ToolCallID)
	}

	_, err = stream.Next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Verify final response.
	resp := stream.Response()
	if len(resp.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(resp.Parts))
	}
	for i, part := range resp.Parts {
		tc, ok := part.(core.ToolCallPart)
		if !ok {
			t.Fatalf("part[%d]: expected ToolCallPart, got %T", i, part)
		}
		expectedSig := "sig_1"
		if i == 1 {
			expectedSig = "sig_2"
		}
		if tc.Metadata == nil || tc.Metadata["thoughtSignature"] != expectedSig {
			t.Errorf("part[%d] thoughtSignature = %v, want %q", i, tc.Metadata, expectedSig)
		}
	}
}

// TestParseSSEStreamFunctionCallNoThoughtSignature verifies that function
// calls without thought signatures (pre-3.x models) have nil Metadata.
func TestParseSSEStreamFunctionCallNoThoughtSignature(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"search","args":{"q":"test"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}

`
	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gemini-2.5-flash")

	event1, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	start, ok := event1.(core.PartStartEvent)
	if !ok {
		t.Fatalf("expected PartStartEvent, got %T", event1)
	}
	tc, ok := start.Part.(core.ToolCallPart)
	if !ok {
		t.Fatal("expected ToolCallPart")
	}
	if tc.Metadata != nil {
		t.Errorf("expected nil Metadata for non-3.x model, got %v", tc.Metadata)
	}
}

// TestParseSSEStreamTextThenFunctionCallWithSignature tests mixed text and
// function call responses where the function call has a thought signature.
func TestParseSSEStreamTextThenFunctionCallWithSignature(t *testing.T) {
	sseData := `data: {"candidates":[{"content":{"role":"model","parts":[{"text":"Let me check that."}]},"finishReason":""}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":5}}

data: {"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"bash","args":{"command":"ls"}},"thoughtSignature":"think_sig"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":50,"candidatesTokenCount":10}}

`
	body := io.NopCloser(strings.NewReader(sseData))
	stream := newStreamedResponse(body, "gemini-3-flash")

	// First event: text start.
	event1, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	start1, ok := event1.(core.PartStartEvent)
	if !ok {
		t.Fatalf("expected PartStartEvent, got %T", event1)
	}
	if _, ok := start1.Part.(core.TextPart); !ok {
		t.Fatalf("expected TextPart, got %T", start1.Part)
	}

	// Second event: function call.
	event2, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	start2, ok := event2.(core.PartStartEvent)
	if !ok {
		t.Fatalf("expected PartStartEvent, got %T", event2)
	}
	tc, ok := start2.Part.(core.ToolCallPart)
	if !ok {
		t.Fatalf("expected ToolCallPart, got %T", start2.Part)
	}
	if tc.Metadata == nil || tc.Metadata["thoughtSignature"] != "think_sig" {
		t.Errorf("Metadata = %v, want thoughtSignature=think_sig", tc.Metadata)
	}

	_, err = stream.Next()
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}

	// Verify final response order: text first, tool call second.
	resp := stream.Response()
	if len(resp.Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(resp.Parts))
	}
	if _, ok := resp.Parts[0].(core.TextPart); !ok {
		t.Fatalf("part[0] expected TextPart, got %T", resp.Parts[0])
	}
	finalTc, ok := resp.Parts[1].(core.ToolCallPart)
	if !ok {
		t.Fatalf("part[1] expected ToolCallPart, got %T", resp.Parts[1])
	}
	if finalTc.Metadata == nil || finalTc.Metadata["thoughtSignature"] != "think_sig" {
		t.Errorf("final Metadata = %v", finalTc.Metadata)
	}
}
