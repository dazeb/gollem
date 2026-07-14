package protocol

import "encoding/json"

// AuthMode is the exact closed public authentication-mode value. It is
// standalone from Gollem's account, credential, and provider runtime.
type AuthMode string

const (
	AuthModeAPIKey              AuthMode = "apikey"
	AuthModeChatGPT             AuthMode = "chatgpt"
	AuthModeChatGPTAuthTokens   AuthMode = "chatgptAuthTokens"
	AuthModeHeaders             AuthMode = "headers"
	AuthModeAgentIdentity       AuthMode = "agentIdentity"
	AuthModePersonalAccessToken AuthMode = "personalAccessToken"
	AuthModeBedrockAPIKey       AuthMode = "bedrockApiKey"
)

func (m AuthMode) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "auth mode", AuthMode.valid)
}

func (m *AuthMode) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "auth mode", AuthMode.valid)
}

func (m AuthMode) valid() bool {
	switch m {
	case AuthModeAPIKey,
		AuthModeChatGPT,
		AuthModeChatGPTAuthTokens,
		AuthModeHeaders,
		AuthModeAgentIdentity,
		AuthModePersonalAccessToken,
		AuthModeBedrockAPIKey:
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = AuthMode("")
	_ json.Unmarshaler = (*AuthMode)(nil)
)
