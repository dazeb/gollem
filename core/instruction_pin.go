package core

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	instructionPinMaxSystemPrompts = 3
	instructionPinMaxFieldBytes    = 320
)

// BuildInstructionPin returns a compact reminder block that re-pins the core
// instructions from the conversation:
// - system prompts from the first message
// - original task/user instruction from the first message
// - latest user instruction from the conversation tail
//
// This is used by context compression paths so critical instructions remain
// salient even after large middle sections are summarized away.
func BuildInstructionPin(messages []ModelMessage) string {
	if len(messages) == 0 {
		return ""
	}

	firstReq, ok := messages[0].(ModelRequest)
	if !ok {
		return ""
	}

	systemPrompts := collectSystemPrompts(firstReq)
	originalUser := compactInstructionText(firstUserPrompt(firstReq), instructionPinMaxFieldBytes)
	latestUser := compactInstructionText(latestUserPrompt(messages), instructionPinMaxFieldBytes)

	if len(systemPrompts) == 0 && originalUser == "" && latestUser == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("[Instruction Pin]\n")

	if len(systemPrompts) > 0 {
		b.WriteString("System directives:\n")
		for i, directive := range systemPrompts {
			fmt.Fprintf(&b, "- S%d: %s\n", i+1, directive)
		}
	}

	if originalUser != "" {
		fmt.Fprintf(&b, "Original task: %s\n", originalUser)
	}
	if latestUser != "" && latestUser != originalUser {
		fmt.Fprintf(&b, "Latest user instruction: %s\n", latestUser)
	}

	b.WriteString("Honor these instructions as authoritative.\n")
	return strings.TrimSpace(b.String())
}

func collectSystemPrompts(req ModelRequest) []string {
	out := make([]string, 0, instructionPinMaxSystemPrompts)
	seen := make(map[string]struct{}, instructionPinMaxSystemPrompts)
	for _, part := range req.Parts {
		sp, ok := part.(SystemPromptPart)
		if !ok {
			continue
		}
		text := compactInstructionText(sp.Content, instructionPinMaxFieldBytes)
		if text == "" {
			continue
		}
		if _, exists := seen[text]; exists {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
		if len(out) >= instructionPinMaxSystemPrompts {
			break
		}
	}
	return out
}

func firstUserPrompt(req ModelRequest) string {
	for _, part := range req.Parts {
		if up, ok := part.(UserPromptPart); ok {
			if strings.TrimSpace(up.Content) != "" {
				return up.Content
			}
		}
	}
	return ""
}

func latestUserPrompt(messages []ModelMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		req, ok := messages[i].(ModelRequest)
		if !ok {
			continue
		}
		for j := len(req.Parts) - 1; j >= 0; j-- {
			if up, ok := req.Parts[j].(UserPromptPart); ok {
				content := strings.TrimSpace(up.Content)
				if content != "" {
					return content
				}
			}
		}
	}
	return ""
}

func compactInstructionText(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	text := strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if text == "" || len(text) <= maxBytes {
		return text
	}
	return truncateUTF8Bytes(text, maxBytes) + "..."
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
