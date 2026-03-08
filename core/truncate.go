package core

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// bytesPerToken is the approximate bytes-per-token heuristic (same as Codex CLI).
const bytesPerToken = 4

// TruncationConfig controls how tool outputs are truncated before being
// recorded into conversation history.
type TruncationConfig struct {
	// MaxTokens is the maximum estimated tokens per tool output.
	// Content exceeding this budget is truncated with a head/tail strategy.
	// Default: 16000 (~64KB of text).
	MaxTokens int
}

// DefaultTruncationConfig returns the default truncation settings.
func DefaultTruncationConfig() TruncationConfig {
	return TruncationConfig{MaxTokens: 16000}
}

// TruncateToolOutput applies head/tail truncation to tool output text.
// It keeps the first 60% and last 40% of the token budget, dropping the
// middle with a marker indicating how many tokens were removed.
// Returns the original string if within budget.
func TruncateToolOutput(content string, config TruncationConfig) string {
	if config.MaxTokens <= 0 {
		return content
	}
	maxBytes := config.MaxTokens * bytesPerToken
	if len(content) <= maxBytes {
		return content
	}
	headBytes := maxBytes * 60 / 100
	tailBytes := maxBytes - headBytes

	// Try to truncate on line boundaries.
	headEnd := truncateToLineEnd(content, headBytes)
	tailStart := truncateToLineStart(content, len(content)-tailBytes)
	if tailStart <= headEnd {
		// Overlap: fall back to raw byte offsets.
		headEnd = headBytes
		tailStart = len(content) - tailBytes
	}

	// Ensure we don't split multi-byte UTF-8 characters at the cut points.
	for headEnd > 0 && !utf8.RuneStart(content[headEnd]) {
		headEnd--
	}
	for tailStart < len(content) && !utf8.RuneStart(content[tailStart]) {
		tailStart++
	}

	droppedTokens := (tailStart - headEnd) / bytesPerToken
	marker := fmt.Sprintf("\n\n... [truncated %d tokens] ...\n\n", droppedTokens)
	return content[:headEnd] + marker + content[tailStart:]
}

// truncateToLineEnd finds the last newline at or before pos in s.
// If no newline is found, returns pos.
func truncateToLineEnd(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	idx := strings.LastIndex(s[:pos], "\n")
	if idx < 0 {
		return pos
	}
	return idx + 1 // include the newline
}

// truncateToLineStart finds the first newline at or after pos in s.
// If no newline is found, returns pos.
func truncateToLineStart(s string, pos int) int {
	if pos <= 0 {
		return 0
	}
	if pos >= len(s) {
		return len(s)
	}
	idx := strings.Index(s[pos:], "\n")
	if idx < 0 {
		return pos
	}
	return pos + idx + 1 // start after the newline
}
