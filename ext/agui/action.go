package agui

// Action types represent commands sent from the UI back to the AGUI session.
const (
	ActionApproveToolCall      = "approve_tool_call"
	ActionDenyToolCall         = "deny_tool_call"
	ActionSubmitDeferredResult = "submit_deferred_result"
	ActionAbortSession         = "abort_session"
	ActionResumeSession        = "resume_session"
	ActionReconnectStream      = "reconnect_stream"
)

// Action is a command sent from the client to the AGUI session.
type Action struct {
	// Type is one of the Action* constants.
	Type string `json:"type"`

	// SessionID identifies the target session.
	SessionID string `json:"session_id"`

	// ToolCallID identifies the tool call for approval/deferred actions.
	ToolCallID string `json:"tool_call_id,omitempty"`

	// ToolName is the tool name for approval/deferred actions.
	ToolName string `json:"tool_name,omitempty"`

	// Approved is set for approve/deny actions.
	Approved *bool `json:"approved,omitempty"`

	// Content is the result content for deferred result submissions.
	Content string `json:"content,omitempty"`

	// IsError indicates the deferred result is an error.
	IsError bool `json:"is_error,omitempty"`

	// Message is an optional human-readable message (e.g., denial reason).
	Message string `json:"message,omitempty"`

	// LastSeq is the last seen event sequence for reconnect actions.
	LastSeq uint64 `json:"last_seq,omitempty"`
}
