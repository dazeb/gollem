package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/core/memory"
)

func makeMessages(n int) []core.ModelMessage {
	messages := make([]core.ModelMessage, 0, n)
	for i := range n {
		if i%2 == 0 {
			messages = append(messages, core.ModelRequest{
				Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: "msg-" + string(rune('A'+i)), Timestamp: time.Now()}},
				Timestamp: time.Now(),
			})
		} else {
			messages = append(messages, core.ModelResponse{
				Parts:     []core.ModelResponsePart{core.TextPart{Content: "resp-" + string(rune('A'+i))}},
				Timestamp: time.Now(),
			})
		}
	}
	return messages
}

// msgText extracts a comparable string from a message for testing.
func msgText(msg core.ModelMessage) string {
	switch m := msg.(type) {
	case core.ModelRequest:
		for _, p := range m.Parts {
			if up, ok := p.(core.UserPromptPart); ok {
				return up.Content
			}
			if sp, ok := p.(core.SystemPromptPart); ok {
				return sp.Content
			}
		}
	case core.ModelResponse:
		return m.TextContent()
	}
	return ""
}

func TestSlidingWindowMemory(t *testing.T) {
	proc := memory.SlidingWindowMemory(2)

	// 10 messages (indices 0-9): first + tail.
	// windowSize*2 = 4, start = 10-4 = 6. messages[6] is a ModelRequest (even),
	// so boundary adjustment decrements to 5 (ModelResponse). Tail = messages[5:].
	// Result = messages[0] + messages[5:] = 6 messages.
	messages := makeMessages(10)
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 6 {
		t.Fatalf("expected 6 messages, got %d", len(result))
	}

	// First message should be preserved.
	if msgText(result[0]) != msgText(messages[0]) {
		t.Error("first message not preserved")
	}

	// Second message should be a ModelResponse (for proper alternation).
	if _, ok := result[1].(core.ModelResponse); !ok {
		t.Errorf("expected ModelResponse at index 1, got %T", result[1])
	}
}

func TestSlidingWindowMemory_PreservesFirst(t *testing.T) {
	proc := memory.SlidingWindowMemory(2)

	messages := makeMessages(10)
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	// First message should always be the original first.
	if msgText(result[0]) != msgText(messages[0]) {
		t.Error("first message was not preserved")
	}
}

func TestSlidingWindowMemory_SmallConversation(t *testing.T) {
	proc := memory.SlidingWindowMemory(5)

	messages := makeMessages(4) // Under window
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != len(messages) {
		t.Errorf("expected %d messages (unchanged), got %d", len(messages), len(result))
	}
}

func TestTokenBudgetMemory(t *testing.T) {
	proc := memory.TokenBudgetMemory(50) // Very tight budget

	// Create messages with known content length.
	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: "You are helpful.", Timestamp: time.Now()},
			core.UserPromptPart{Content: "Hello", Timestamp: time.Now()},
		}, Timestamp: time.Now()},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "Hi there! How can I help you today? I am here to assist with anything."}}, Timestamp: time.Now()},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "Tell me about Go programming language and its features.", Timestamp: time.Now()}}, Timestamp: time.Now()},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "Go is a statically typed language designed for simplicity and performance."}}, Timestamp: time.Now()},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "Thanks", Timestamp: time.Now()}}, Timestamp: time.Now()},
	}

	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	// Should have dropped some messages.
	if len(result) >= len(messages) {
		t.Errorf("expected fewer messages, got %d", len(result))
	}

	// First and last should be preserved.
	if msgText(result[0]) != msgText(messages[0]) {
		t.Error("first message not preserved")
	}
	if msgText(result[len(result)-1]) != msgText(messages[len(messages)-1]) {
		t.Error("last message not preserved")
	}
}

func TestTokenBudgetMemory_SmallConversation(t *testing.T) {
	proc := memory.TokenBudgetMemory(10000) // Very generous budget

	messages := makeMessages(4)
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != len(messages) {
		t.Errorf("expected %d messages (unchanged), got %d", len(messages), len(result))
	}
}

func TestSummaryMemory(t *testing.T) {
	// Create a summarizer model that returns a canned summary.
	summarizer := core.NewTestModel(core.TextResponse("User asked about Go and got a helpful response."))

	proc := memory.SummaryMemory(summarizer, 4)

	messages := []core.ModelMessage{
		core.ModelRequest{Parts: []core.ModelRequestPart{
			core.SystemPromptPart{Content: "You are helpful.", Timestamp: time.Now()},
			core.UserPromptPart{Content: "Tell me about Go", Timestamp: time.Now()},
		}, Timestamp: time.Now()},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "Go is great!"}}, Timestamp: time.Now()},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "What about concurrency?", Timestamp: time.Now()}}, Timestamp: time.Now()},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "Goroutines are lightweight threads."}}, Timestamp: time.Now()},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "And channels?", Timestamp: time.Now()}}, Timestamp: time.Now()},
		core.ModelResponse{Parts: []core.ModelResponsePart{core.TextPart{Content: "Channels enable safe communication."}}, Timestamp: time.Now()},
		core.ModelRequest{Parts: []core.ModelRequestPart{core.UserPromptPart{Content: "Thanks!", Timestamp: time.Now()}}, Timestamp: time.Now()},
	}

	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	// Should be shorter than original.
	if len(result) >= len(messages) {
		t.Errorf("expected fewer messages after summary, got %d (was %d)", len(result), len(messages))
	}

	// First message should be preserved.
	if msgText(result[0]) != msgText(messages[0]) {
		t.Error("first message not preserved")
	}

	// Summary should be a ModelResponse (assistant role) at index 1.
	if resp, ok := result[1].(core.ModelResponse); ok {
		text := resp.TextContent()
		if text == "" {
			t.Error("expected summary text in ModelResponse at index 1")
		}
	} else {
		t.Errorf("expected ModelResponse (summary) at index 1, got %T", result[1])
	}

	// Summarizer model should have been called.
	if len(summarizer.Calls()) == 0 {
		t.Error("summarizer was not called")
	}
}

func TestSummaryMemory_ShortConversation(t *testing.T) {
	summarizer := core.NewTestModel(core.TextResponse("summary"))
	proc := memory.SummaryMemory(summarizer, 10)

	messages := makeMessages(4) // Under limit
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != len(messages) {
		t.Errorf("expected %d messages (unchanged), got %d", len(messages), len(result))
	}

	// Summarizer should NOT have been called.
	if len(summarizer.Calls()) != 0 {
		t.Error("summarizer should not be called for short conversations")
	}
}

// verifyAlternation checks that no two adjacent messages have the same role.
func verifyAlternation(t *testing.T, messages []core.ModelMessage) {
	t.Helper()
	for i := 1; i < len(messages); i++ {
		_, prevIsReq := messages[i-1].(core.ModelRequest)
		_, currIsReq := messages[i].(core.ModelRequest)
		_, prevIsResp := messages[i-1].(core.ModelResponse)
		_, currIsResp := messages[i].(core.ModelResponse)

		if prevIsReq && currIsReq {
			t.Errorf("adjacent ModelRequest at indices %d and %d", i-1, i)
		}
		if prevIsResp && currIsResp {
			t.Errorf("adjacent ModelResponse at indices %d and %d", i-1, i)
		}
	}
}

func TestSlidingWindowMemory_Alternation(t *testing.T) {
	// Test with various message counts and window sizes to ensure
	// alternation is maintained in all cases.
	for _, tc := range []struct {
		name       string
		msgCount   int
		windowSize int
	}{
		{"10msgs_w2", 10, 2},
		{"10msgs_w3", 10, 3},
		{"12msgs_w2", 12, 2},
		{"20msgs_w4", 20, 4},
		{"8msgs_w1", 8, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proc := memory.SlidingWindowMemory(tc.windowSize)
			messages := makeMessages(tc.msgCount)
			result, err := proc(context.Background(), messages)
			if err != nil {
				t.Fatal(err)
			}
			verifyAlternation(t, result)

			// First message should be preserved.
			if msgText(result[0]) != msgText(messages[0]) {
				t.Error("first message not preserved")
			}
		})
	}
}

func TestTokenBudgetMemory_Alternation(t *testing.T) {
	proc := memory.TokenBudgetMemory(20) // Very tight to force drops

	// Build messages with enough content to exceed the budget.
	var messages []core.ModelMessage
	for i := range 10 {
		if i%2 == 0 {
			messages = append(messages, core.ModelRequest{
				Parts:     []core.ModelRequestPart{core.UserPromptPart{Content: "This is a user message with enough words to inflate the token count above our threshold number " + string(rune('A'+i))}},
				Timestamp: time.Now(),
			})
		} else {
			messages = append(messages, core.ModelResponse{
				Parts:     []core.ModelResponsePart{core.TextPart{Content: "This is an assistant response with plenty of content to drive up the estimated token count significantly " + string(rune('A'+i))}},
				Timestamp: time.Now(),
			})
		}
	}

	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	// Should have dropped some messages.
	if len(result) >= len(messages) {
		t.Fatalf("expected fewer messages, got %d", len(result))
	}

	verifyAlternation(t, result)

	// First and last should be preserved.
	if msgText(result[0]) != msgText(messages[0]) {
		t.Error("first message not preserved")
	}
	if msgText(result[len(result)-1]) != msgText(messages[len(messages)-1]) {
		t.Error("last message not preserved")
	}
}

func TestSummaryMemory_Alternation(t *testing.T) {
	summarizer := core.NewTestModel(core.TextResponse("Summary of the conversation"))
	proc := memory.SummaryMemory(summarizer, 4)

	// 10 alternating messages (req, resp, req, resp, ...).
	messages := makeMessages(10)
	result, err := proc(context.Background(), messages)
	if err != nil {
		t.Fatal(err)
	}

	verifyAlternation(t, result)

	// First should be ModelRequest, second should be ModelResponse (summary).
	if _, ok := result[0].(core.ModelRequest); !ok {
		t.Errorf("expected ModelRequest at index 0, got %T", result[0])
	}
	if _, ok := result[1].(core.ModelResponse); !ok {
		t.Errorf("expected ModelResponse (summary) at index 1, got %T", result[1])
	}
}
