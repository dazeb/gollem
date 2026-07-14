package protocol

import "encoding/json"

// McpServerStartupFailureReason is the exact closed public MCP startup failure
// reason. It does not imply that Gollem performs reauthentication.
type McpServerStartupFailureReason string

const (
	McpServerStartupFailureReasonReauthenticationRequired McpServerStartupFailureReason = "reauthenticationRequired"
)

func (r McpServerStartupFailureReason) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "MCP server startup failure reason", McpServerStartupFailureReason.valid)
}

func (r *McpServerStartupFailureReason) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "MCP server startup failure reason", McpServerStartupFailureReason.valid)
}

func (r McpServerStartupFailureReason) valid() bool {
	return r == McpServerStartupFailureReasonReauthenticationRequired
}

// McpServerStartupState is the exact closed public MCP startup lifecycle. It
// remains standalone from Gollem's MCP registration and runtime state.
type McpServerStartupState string

const (
	McpServerStartupStateStarting  McpServerStartupState = "starting"
	McpServerStartupStateReady     McpServerStartupState = "ready"
	McpServerStartupStateFailed    McpServerStartupState = "failed"
	McpServerStartupStateCancelled McpServerStartupState = "cancelled"
)

func (s McpServerStartupState) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "MCP server startup state", McpServerStartupState.valid)
}

func (s *McpServerStartupState) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "MCP server startup state", McpServerStartupState.valid)
}

func (s McpServerStartupState) valid() bool {
	return s == McpServerStartupStateStarting ||
		s == McpServerStartupStateReady ||
		s == McpServerStartupStateFailed ||
		s == McpServerStartupStateCancelled
}

var (
	_ json.Marshaler   = McpServerStartupFailureReason("")
	_ json.Unmarshaler = (*McpServerStartupFailureReason)(nil)
	_ json.Marshaler   = McpServerStartupState("")
	_ json.Unmarshaler = (*McpServerStartupState)(nil)
)
