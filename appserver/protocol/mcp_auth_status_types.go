package protocol

import "encoding/json"

// McpAuthStatus is the exact closed public MCP authentication status. It is
// standalone from Gollem's broader MCP status wire and authentication runtime.
type McpAuthStatus string

const (
	McpAuthStatusUnsupported McpAuthStatus = "unsupported"
	McpAuthStatusNotLoggedIn McpAuthStatus = "notLoggedIn"
	McpAuthStatusBearerToken McpAuthStatus = "bearerToken"
	McpAuthStatusOAuth       McpAuthStatus = "oAuth"
)

func (s McpAuthStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "MCP auth status", McpAuthStatus.valid)
}

func (s *McpAuthStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "MCP auth status", McpAuthStatus.valid)
}

func (s McpAuthStatus) valid() bool {
	return s == McpAuthStatusUnsupported ||
		s == McpAuthStatusNotLoggedIn ||
		s == McpAuthStatusBearerToken ||
		s == McpAuthStatusOAuth
}

var (
	_ json.Marshaler   = McpAuthStatus("")
	_ json.Unmarshaler = (*McpAuthStatus)(nil)
)
