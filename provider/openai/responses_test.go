package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestChatGPTAuth_PromptCacheKey(t *testing.T) {
	// Verify that ChatGPT auth mode auto-generates a prompt cache key
	// and that applyChatGPTRequirements preserves it.
	p := New(WithChatGPTAuth("token", "acct"))
	if p.promptCacheKey == "" {
		t.Fatal("expected auto-generated promptCacheKey for ChatGPT endpoint")
	}

	// Build a request and apply ChatGPT requirements.
	req := &responsesRequest{
		Model:                "gpt-5",
		PromptCacheKey:       p.promptCacheKey,
		MaxOutputTokens:      128000,
		ServiceTier:          "default",
		PromptCacheRetention: "24h",
	}
	p.applyChatGPTRequirements(req)

	// prompt_cache_key should be preserved.
	if req.PromptCacheKey != p.promptCacheKey {
		t.Errorf("expected prompt_cache_key preserved as %q, got %q", p.promptCacheKey, req.PromptCacheKey)
	}
	// Other unsupported fields should be cleared.
	if req.PromptCacheRetention != "" {
		t.Errorf("expected prompt_cache_retention cleared, got %q", req.PromptCacheRetention)
	}
	if req.ServiceTier != "" {
		t.Errorf("expected service_tier cleared, got %q", req.ServiceTier)
	}
	if req.MaxOutputTokens != 0 {
		t.Errorf("expected max_output_tokens cleared, got %d", req.MaxOutputTokens)
	}
}

func TestChatGPTAuth_PromptCacheKeyInRequest(t *testing.T) {
	// Verify that prompt_cache_key is sent in the request body when using
	// ChatGPT auth, even with a custom base URL (proxy/test scenario).
	// The hasChatGPTAuth() fallback ensures caching works through proxies.
	var receivedCacheKey string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		json.Unmarshal(body, &req)
		if key, ok := req["prompt_cache_key"].(string); ok {
			receivedCacheKey = key
		}
		// Return plain JSON (test server, not chatgpt.com).
		json.NewEncoder(w).Encode(responsesAPIResponse{
			Output: []responsesOutputItem{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	defer ts.Close()

	// WithBaseURL overrides chatgptBaseURL, so isChatGPTEndpoint() is false.
	// But hasChatGPTAuth() is true via chatgptAccountID, so caching still works.
	p := New(
		WithChatGPTAuth("token", "acct"),
		WithBaseURL(ts.URL),
		WithPromptCacheKey("test-cache-key"),
	)

	_, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if receivedCacheKey != "test-cache-key" {
		t.Errorf("expected prompt_cache_key='test-cache-key' in request body, got %q", receivedCacheKey)
	}
}

func TestParseSSEResponsesAcceptsResponseDone(t *testing.T) {
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(`data: {"type":"response.done","response":{"id":"resp_done","model":"gpt-5","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done-ok"}]}],"usage":{"input_tokens":4,"output_tokens":2}}}


data: [DONE]
`))}
	p := New(WithModel("gpt-5"))

	got, err := p.parseSSEResponses(resp)
	if err != nil {
		t.Fatalf("parseSSEResponses: %v", err)
	}
	if text := got.TextContent(); text != "done-ok" {
		t.Fatalf("response text = %q, want done-ok", text)
	}
	if got.Usage.InputTokens != 4 || got.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %+v", got.Usage)
	}
}

func TestCompatibleEndpoint_NoCacheKey(t *testing.T) {
	// Verify that non-OpenAI/non-ChatGPT endpoints (xAI, etc.) do NOT
	// get prompt_cache_key, even if one is configured on the provider.
	var receivedBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedBody)
		json.NewEncoder(w).Encode(responsesAPIResponse{
			Output: []responsesOutputItem{
				{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "ok"}}},
			},
		})
	}))
	defer ts.Close()

	p := New(
		WithModel("grok-3"),
		WithAPIKey("test-key"),
		WithBaseURL(ts.URL),
		WithPromptCacheKey("should-not-appear"),
	)
	p.useResponses = true

	_, err := p.Request(context.Background(), []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "hi"}}},
	}, nil, nil)
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if _, has := receivedBody["prompt_cache_key"]; has {
		t.Error("expected no prompt_cache_key for non-OpenAI endpoint, but it was present")
	}
}

// TestStreamReasoningSummaryDeltas verifies the Responses stream surfaces
// reasoning summary text as ThinkingPart via start + delta events, and that
// the final Response contains the aggregated ThinkingPart.
func TestStreamReasoningSummaryDeltas(t *testing.T) {
	sse := `data: {"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","summary":[]}}

data: {"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"I will "}

data: {"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"think."}

data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","summary":[{"type":"summary_text","text":"I will think."}]}}

data: {"type":"response.output_text.delta","delta":"done"}

data: {"type":"response.completed","response":{"id":"resp_r","model":"gpt-5","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"I will think."}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":1,"output_tokens":2,"output_tokens_details":{"reasoning_tokens":2}}}}

data: [DONE]
`
	stream := newResponsesStreamedResponse(io.NopCloser(strings.NewReader(sse)), "gpt-5")

	var (
		gotStart  bool
		gotDeltas strings.Builder
	)
	for {
		ev, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		switch e := ev.(type) {
		case core.PartStartEvent:
			if tp, ok := e.Part.(core.ThinkingPart); ok {
				gotStart = true
				gotDeltas.WriteString(tp.Content)
			}
		case core.PartDeltaEvent:
			if td, ok := e.Delta.(core.ThinkingPartDelta); ok {
				gotDeltas.WriteString(td.ContentDelta)
			}
		}
	}

	if !gotStart {
		t.Fatal("expected PartStartEvent with ThinkingPart")
	}
	if got := gotDeltas.String(); got != "I will think." {
		t.Fatalf("delta-accumulated content = %q, want %q", got, "I will think.")
	}

	resp := stream.Response()
	var thinking string
	for _, p := range resp.Parts {
		if tp, ok := p.(core.ThinkingPart); ok {
			thinking = tp.Content
		}
	}
	if thinking != "I will think." {
		t.Errorf("ThinkingPart.Content = %q, want %q", thinking, "I will think.")
	}
}

// TestStreamReasoningDoneOnly verifies the non-streaming fallback path —
// when only output_item.done arrives (no deltas), the summary text is still
// surfaced as a ThinkingPart via a single PartStartEvent.
func TestStreamReasoningDoneOnly(t *testing.T) {
	sse := `data: {"type":"response.output_item.done","output_index":0,"item":{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm."}]}}

data: {"type":"response.completed","response":{"id":"r","model":"gpt-5","output":[{"type":"reasoning","summary":[{"type":"summary_text","text":"hmm."}]}],"usage":{"input_tokens":1,"output_tokens":1}}}

data: [DONE]
`
	stream := newResponsesStreamedResponse(io.NopCloser(strings.NewReader(sse)), "gpt-5")

	var thinkingFromStart string
	for {
		ev, err := stream.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if start, ok := ev.(core.PartStartEvent); ok {
			if tp, ok := start.Part.(core.ThinkingPart); ok {
				thinkingFromStart = tp.Content
			}
		}
	}
	if thinkingFromStart != "hmm." {
		t.Errorf("PartStartEvent ThinkingPart = %q, want %q", thinkingFromStart, "hmm.")
	}
}

// TestParseResponsesReasoningItem verifies the non-streaming JSON response
// path surfaces reasoning items as ThinkingPart.
func TestParseResponsesReasoningItem(t *testing.T) {
	resp := &responsesAPIResponse{
		Output: []responsesOutputItem{
			{Type: "reasoning", Summary: []responsesSummaryItem{
				{Type: "summary_text", Text: "step 1 "},
				{Type: "summary_text", Text: "step 2"},
			}},
			{Type: "message", Content: []responsesContentItem{{Type: "output_text", Text: "answer"}}},
		},
	}
	got := parseResponsesResponse(resp, "gpt-5")
	if len(got.Parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(got.Parts))
	}
	tp, ok := got.Parts[0].(core.ThinkingPart)
	if !ok {
		t.Fatalf("parts[0] = %T, want ThinkingPart", got.Parts[0])
	}
	if tp.Content != "step 1 step 2" {
		t.Errorf("ThinkingPart.Content = %q, want %q", tp.Content, "step 1 step 2")
	}
	if tx, ok := got.Parts[1].(core.TextPart); !ok || tx.Content != "answer" {
		t.Errorf("parts[1] = %+v, want TextPart{answer}", got.Parts[1])
	}
}

// TestBuildResponsesRequestAudio verifies AudioPart with a data URI becomes
// an input_audio content item with base64 data + format.
func TestBuildResponsesRequestAudio(t *testing.T) {
	dataURI := core.BinaryContent([]byte{1, 2, 3}, "audio/mp3")
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.AudioPart{URL: dataURI},
		}},
	}
	req, err := buildResponsesRequest(messages, nil, nil, "gpt-4o", 1024, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}
	if len(req.Input) != 1 {
		t.Fatalf("expected 1 input item, got %d", len(req.Input))
	}
	msg := req.Input[0]
	content := msg["content"].([]map[string]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(content))
	}
	if content[0]["type"] != "input_audio" {
		t.Errorf("type = %v, want input_audio", content[0]["type"])
	}
	audio := content[0]["input_audio"].(map[string]any)
	if audio["format"] != "mp3" {
		t.Errorf("format = %v, want mp3", audio["format"])
	}
	if audio["data"] == "" || audio["data"] == nil {
		t.Error("expected non-empty base64 data")
	}
}

// TestBuildResponsesRequestAudioRejectsNonDataURI verifies a plain URL fails
// — OpenAI requires inline base64 for audio.
func TestBuildResponsesRequestAudioRejectsNonDataURI(t *testing.T) {
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.AudioPart{URL: "https://example.com/a.mp3"},
		}},
	}
	_, err := buildResponsesRequest(messages, nil, nil, "gpt-4o", 1024, false)
	if err == nil {
		t.Fatal("expected error for non-data-URI audio")
	}
	if !strings.Contains(err.Error(), "base64 data URI") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestBuildResponsesRequestDocumentFileID verifies DocumentPart with a bare
// file-id emits {type: input_file, file_id: ...}.
func TestBuildResponsesRequestDocumentFileID(t *testing.T) {
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.DocumentPart{URL: "file-abc123"},
		}},
	}
	req, err := buildResponsesRequest(messages, nil, nil, "gpt-4o", 1024, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}
	content := req.Input[0]["content"].([]map[string]any)
	if content[0]["type"] != "input_file" || content[0]["file_id"] != "file-abc123" {
		t.Errorf("item = %+v", content[0])
	}
}

// TestBuildResponsesRequestDocumentDataURI verifies DocumentPart with a
// data URI emits {type: input_file, file_data, filename}.
func TestBuildResponsesRequestDocumentDataURI(t *testing.T) {
	dataURI := core.BinaryContent([]byte("%PDF-1.4"), "application/pdf")
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.DocumentPart{URL: dataURI, Title: "report.pdf"},
		}},
	}
	req, err := buildResponsesRequest(messages, nil, nil, "gpt-4o", 1024, false)
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}
	content := req.Input[0]["content"].([]map[string]any)
	if content[0]["type"] != "input_file" {
		t.Errorf("type = %v", content[0]["type"])
	}
	if content[0]["filename"] != "report.pdf" {
		t.Errorf("filename = %v", content[0]["filename"])
	}
	fileData, ok := content[0]["file_data"].(string)
	if !ok || !strings.HasPrefix(fileData, "data:application/pdf;base64,") {
		t.Errorf("file_data = %v", content[0]["file_data"])
	}
}
