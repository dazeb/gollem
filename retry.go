package gollem

import (
	"fmt"
	"time"
)

// incrementRetries increments the result retry counter and returns an error
// if the maximum has been exceeded. It also detects incomplete tool calls
// caused by the model hitting its token limit.
func incrementRetries(retries *int, maxRetries int, messages []ModelMessage) error {
	*retries++

	if *retries > maxRetries {
		// Check for incomplete tool call (finish_reason == length + malformed args).
		if lastResp := lastModelResponse(messages); lastResp != nil {
			if lastResp.FinishReason == FinishReasonLength {
				toolCalls := lastResp.ToolCalls()
				if len(toolCalls) > 0 {
					lastCall := toolCalls[len(toolCalls)-1]
					if _, err := lastCall.ArgsAsMap(); err != nil {
						return &IncompleteToolCall{
							UnexpectedModelBehavior: UnexpectedModelBehavior{
								Message: "model hit token limit while generating tool call arguments, consider increasing max_tokens",
							},
						}
					}
				}
			}
		}

		return &UnexpectedModelBehavior{
			Message: fmt.Sprintf("exceeded maximum retries (%d) for result validation", maxRetries),
		}
	}
	return nil
}

// buildRetryParts creates a RetryPromptPart with the given error message.
func buildRetryParts(msg string, toolName, toolCallID string) RetryPromptPart {
	return RetryPromptPart{
		Content:    msg,
		ToolName:   toolName,
		ToolCallID: toolCallID,
		Timestamp:  time.Now(),
	}
}

// lastModelResponse returns the last ModelResponse in the message history, or nil.
func lastModelResponse(messages []ModelMessage) *ModelResponse {
	for i := len(messages) - 1; i >= 0; i-- {
		if resp, ok := messages[i].(ModelResponse); ok {
			return &resp
		}
	}
	return nil
}
