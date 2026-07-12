package protocol

// Personality is the exact public request-side response-style preference. It
// does not by itself change Gollem model behavior.
type Personality string

const (
	PersonalityNone      Personality = "none"
	PersonalityFriendly  Personality = "friendly"
	PersonalityPragmatic Personality = "pragmatic"
)

func (p Personality) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(p, "personality", Personality.valid)
}

func (p *Personality) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, p, "personality", Personality.valid)
}

func (p Personality) valid() bool {
	switch p {
	case PersonalityNone, PersonalityFriendly, PersonalityPragmatic:
		return true
	default:
		return false
	}
}

// SandboxMode is the exact public request-side mode. The tagged response-side
// SandboxPolicy and Gollem runtime permission policies remain separate types.
type SandboxMode string

const (
	SandboxModeReadOnly         SandboxMode = "read-only"
	SandboxModeWorkspaceWrite   SandboxMode = "workspace-write"
	SandboxModeDangerFullAccess SandboxMode = "danger-full-access"
)

func (m SandboxMode) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "sandbox mode", SandboxMode.valid)
}

func (m *SandboxMode) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "sandbox mode", SandboxMode.valid)
}

func (m SandboxMode) valid() bool {
	switch m {
	case SandboxModeReadOnly, SandboxModeWorkspaceWrite, SandboxModeDangerFullAccess:
		return true
	default:
		return false
	}
}

// ThreadStartSource is the exact start-request source enum. It is distinct
// from public provenance, list-filter source kinds, and session provenance.
type ThreadStartSource string

const (
	ThreadStartSourceStartup ThreadStartSource = "startup"
	ThreadStartSourceClear   ThreadStartSource = "clear"
)

func (s ThreadStartSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "thread start source", ThreadStartSource.valid)
}

func (s *ThreadStartSource) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "thread start source", ThreadStartSource.valid)
}

func (s ThreadStartSource) valid() bool {
	return s == ThreadStartSourceStartup || s == ThreadStartSourceClear
}
