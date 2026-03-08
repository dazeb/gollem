package core

import (
	"strings"
	"testing"
)

func TestTruncateToolOutput_UnderBudget(t *testing.T) {
	content := "short content"
	config := TruncationConfig{MaxTokens: 100}
	result := TruncateToolOutput(content, config)
	if result != content {
		t.Errorf("expected unchanged content, got %q", result)
	}
}

func TestTruncateToolOutput_OverBudget(t *testing.T) {
	// 10 tokens = 40 bytes budget. Create content well over that.
	content := strings.Repeat("abcdefgh\n", 100) // 900 bytes
	config := TruncationConfig{MaxTokens: 10}
	result := TruncateToolOutput(content, config)
	if len(result) >= len(content) {
		t.Errorf("expected truncated result shorter than original (%d >= %d)", len(result), len(content))
	}
	if !strings.Contains(result, "... [truncated") {
		t.Error("expected truncation marker in result")
	}
	if !strings.Contains(result, "tokens] ...") {
		t.Error("expected 'tokens] ...' in truncation marker")
	}
}

func TestTruncateToolOutput_ExactBudget(t *testing.T) {
	// Exactly at budget should not be truncated.
	content := strings.Repeat("a", 40) // 40 bytes = 10 tokens
	config := TruncationConfig{MaxTokens: 10}
	result := TruncateToolOutput(content, config)
	if result != content {
		t.Errorf("expected unchanged content at exact budget, len=%d vs %d", len(result), len(content))
	}
}

func TestTruncateToolOutput_Empty(t *testing.T) {
	result := TruncateToolOutput("", TruncationConfig{MaxTokens: 100})
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestTruncateToolOutput_LineBreaks(t *testing.T) {
	// Create content with clear line boundaries.
	var lines []string
	for range 200 {
		lines = append(lines, strings.Repeat("x", 20)+"\n")
	}
	content := strings.Join(lines, "")
	config := TruncationConfig{MaxTokens: 100} // 400 bytes budget
	result := TruncateToolOutput(content, config)
	if !strings.Contains(result, "truncated") {
		t.Error("expected truncation marker")
	}
	// Head and tail should end/start at line boundaries (contain newlines).
	parts := strings.SplitN(result, "... [truncated", 2)
	if len(parts) != 2 {
		t.Fatal("expected split around marker")
	}
	head := parts[0]
	if !strings.HasSuffix(head, "\n\n") {
		t.Errorf("expected head to end at line boundary, got suffix %q", head[len(head)-10:])
	}
}

func TestTruncateToolOutput_StructuredJSON(t *testing.T) {
	// Simulate a large JSON tool output.
	entries := make([]string, 500)
	for i := range entries {
		entries[i] = `{"key":"value","index":` + strings.Repeat("0", 20) + `}`
	}
	content := "[\n" + strings.Join(entries, ",\n") + "\n]"
	config := TruncationConfig{MaxTokens: 200} // 800 bytes budget
	result := TruncateToolOutput(content, config)
	if !strings.Contains(result, "truncated") {
		t.Error("expected truncation")
	}
	// Should preserve start (the opening bracket) and end (the closing bracket).
	if !strings.HasPrefix(result, "[") {
		t.Error("expected JSON opening preserved")
	}
	if !strings.HasSuffix(strings.TrimSpace(result), "]") {
		t.Error("expected JSON closing preserved")
	}
}

func TestTruncateToolOutput_ZeroBudget(t *testing.T) {
	content := "some content"
	result := TruncateToolOutput(content, TruncationConfig{MaxTokens: 0})
	if result != content {
		t.Errorf("expected unchanged content with zero budget, got %q", result)
	}
}
