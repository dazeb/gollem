package gollem

import "fmt"

// DeferredToolRequest represents a tool call that requires external resolution.
type DeferredToolRequest struct {
	ToolName   string `json:"tool_name"`
	ToolCallID string `json:"tool_call_id"`
	ArgsJSON   string `json:"args_json"`
}

// DeferredToolResult provides the result for a previously deferred tool call.
type DeferredToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}

// CallDeferred is returned by tool handlers to indicate the tool call
// should be deferred for external resolution.
type CallDeferred struct {
	Message string
}

func (e *CallDeferred) Error() string { return "deferred: " + e.Message }

// RunResultDeferred is returned when an agent run ends with pending deferred tools.
type RunResultDeferred[T any] struct {
	DeferredRequests []DeferredToolRequest
	Messages         []ModelMessage
	Usage            RunUsage
}

// ErrDeferred wraps a RunResultDeferred and is returned from Run when
// tool calls are deferred for external resolution.
type ErrDeferred[T any] struct {
	Result RunResultDeferred[T]
}

func (e *ErrDeferred[T]) Error() string {
	return fmt.Sprintf("agent run deferred: %d tool call(s) require external resolution", len(e.Result.DeferredRequests))
}

// WithDeferredResults injects deferred tool results into the run.
// When provided, the results are sent as ToolReturnParts (or RetryPromptParts
// for error results) before the first model call.
func WithDeferredResults(results ...DeferredToolResult) RunOption {
	return func(c *runConfig) {
		c.deferredResults = results
	}
}
