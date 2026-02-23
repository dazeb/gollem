package core

import (
	"testing"
	"time"
)

// TestMarshalMessages_ToolCallMetadata verifies that ToolCallPart.Metadata
// (e.g., Gemini 3.x thought signatures) survives serialization round-trip.
func TestMarshalMessages_ToolCallMetadata(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	messages := []ModelMessage{
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{
					ToolName:   "bash",
					ArgsJSON:   `{"command":"ls"}`,
					ToolCallID: "call_0",
					Metadata:   map[string]string{"thoughtSignature": "abc123sig"},
				},
			},
			ModelName:    "gemini-3-flash",
			FinishReason: FinishReasonToolCall,
			Timestamp:    now,
		},
	}

	data, err := MarshalMessages(messages)
	if err != nil {
		t.Fatalf("MarshalMessages failed: %v", err)
	}

	got, err := UnmarshalMessages(data)
	if err != nil {
		t.Fatalf("UnmarshalMessages failed: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("message count = %d, want 1", len(got))
	}

	resp, ok := got[0].(ModelResponse)
	if !ok {
		t.Fatal("expected ModelResponse")
	}
	if len(resp.Parts) != 1 {
		t.Fatalf("parts count = %d, want 1", len(resp.Parts))
	}

	tc, ok := resp.Parts[0].(ToolCallPart)
	if !ok {
		t.Fatal("expected ToolCallPart")
	}
	if tc.Metadata == nil {
		t.Fatal("expected Metadata to be preserved, got nil")
	}
	if sig := tc.Metadata["thoughtSignature"]; sig != "abc123sig" {
		t.Errorf("thoughtSignature = %q, want %q", sig, "abc123sig")
	}
}

// TestMarshalMessages_ToolCallNilMetadata verifies that nil Metadata
// round-trips correctly (not turned into an empty map).
func TestMarshalMessages_ToolCallNilMetadata(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	messages := []ModelMessage{
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{
					ToolName:   "search",
					ArgsJSON:   `{"q":"test"}`,
					ToolCallID: "call_1",
					// No Metadata set.
				},
			},
			Timestamp: now,
		},
	}

	data, err := MarshalMessages(messages)
	if err != nil {
		t.Fatalf("MarshalMessages failed: %v", err)
	}

	got, err := UnmarshalMessages(data)
	if err != nil {
		t.Fatalf("UnmarshalMessages failed: %v", err)
	}

	resp := got[0].(ModelResponse)
	tc := resp.Parts[0].(ToolCallPart)
	if tc.Metadata != nil {
		t.Errorf("expected nil Metadata, got %v", tc.Metadata)
	}
}

// TestMarshalMessages_MultipleToolCallsWithMetadata tests round-trip of
// multiple tool calls where some have Metadata and some don't.
func TestMarshalMessages_MultipleToolCallsWithMetadata(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	messages := []ModelMessage{
		ModelResponse{
			Parts: []ModelResponsePart{
				ToolCallPart{
					ToolName:   "bash",
					ArgsJSON:   `{"command":"ls"}`,
					ToolCallID: "call_0",
					Metadata:   map[string]string{"thoughtSignature": "sig1"},
				},
				ToolCallPart{
					ToolName:   "search",
					ArgsJSON:   `{"q":"test"}`,
					ToolCallID: "call_1",
					// No metadata.
				},
				ToolCallPart{
					ToolName:   "edit",
					ArgsJSON:   `{"file":"main.go"}`,
					ToolCallID: "call_2",
					Metadata:   map[string]string{"thoughtSignature": "sig2", "extra": "data"},
				},
			},
			Timestamp: now,
		},
	}

	data, err := MarshalMessages(messages)
	if err != nil {
		t.Fatalf("MarshalMessages failed: %v", err)
	}

	got, err := UnmarshalMessages(data)
	if err != nil {
		t.Fatalf("UnmarshalMessages failed: %v", err)
	}

	resp := got[0].(ModelResponse)
	if len(resp.Parts) != 3 {
		t.Fatalf("parts count = %d, want 3", len(resp.Parts))
	}

	// First: has metadata.
	tc0 := resp.Parts[0].(ToolCallPart)
	if tc0.Metadata == nil || tc0.Metadata["thoughtSignature"] != "sig1" {
		t.Errorf("tc0 metadata = %v, want thoughtSignature=sig1", tc0.Metadata)
	}

	// Second: no metadata.
	tc1 := resp.Parts[1].(ToolCallPart)
	if tc1.Metadata != nil {
		t.Errorf("tc1 metadata = %v, want nil", tc1.Metadata)
	}

	// Third: has multiple metadata keys.
	tc2 := resp.Parts[2].(ToolCallPart)
	if tc2.Metadata == nil {
		t.Fatal("tc2 metadata is nil")
	}
	if tc2.Metadata["thoughtSignature"] != "sig2" {
		t.Errorf("tc2 thoughtSignature = %q, want sig2", tc2.Metadata["thoughtSignature"])
	}
	if tc2.Metadata["extra"] != "data" {
		t.Errorf("tc2 extra = %q, want data", tc2.Metadata["extra"])
	}
}
