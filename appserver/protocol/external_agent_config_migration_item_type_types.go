package protocol

import "encoding/json"

// ExternalAgentConfigMigrationItemType is the exact closed public category for
// an external-agent migration item. It remains standalone from discovery,
// import, configuration, and runtime behavior.
type ExternalAgentConfigMigrationItemType string

const (
	ExternalAgentConfigMigrationItemTypeAgentsMD        ExternalAgentConfigMigrationItemType = "AGENTS_MD"
	ExternalAgentConfigMigrationItemTypeConfig          ExternalAgentConfigMigrationItemType = "CONFIG"
	ExternalAgentConfigMigrationItemTypeSkills          ExternalAgentConfigMigrationItemType = "SKILLS"
	ExternalAgentConfigMigrationItemTypePlugins         ExternalAgentConfigMigrationItemType = "PLUGINS"
	ExternalAgentConfigMigrationItemTypeMCPServerConfig ExternalAgentConfigMigrationItemType = "MCP_SERVER_CONFIG"
	ExternalAgentConfigMigrationItemTypeSubagents       ExternalAgentConfigMigrationItemType = "SUBAGENTS"
	ExternalAgentConfigMigrationItemTypeHooks           ExternalAgentConfigMigrationItemType = "HOOKS"
	ExternalAgentConfigMigrationItemTypeCommands        ExternalAgentConfigMigrationItemType = "COMMANDS"
	ExternalAgentConfigMigrationItemTypeSessions        ExternalAgentConfigMigrationItemType = "SESSIONS"
)

func (t ExternalAgentConfigMigrationItemType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(t, "external-agent migration item type", ExternalAgentConfigMigrationItemType.valid)
}

func (t *ExternalAgentConfigMigrationItemType) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, t, "external-agent migration item type", ExternalAgentConfigMigrationItemType.valid)
}

func (t ExternalAgentConfigMigrationItemType) valid() bool {
	switch t {
	case ExternalAgentConfigMigrationItemTypeAgentsMD,
		ExternalAgentConfigMigrationItemTypeConfig,
		ExternalAgentConfigMigrationItemTypeSkills,
		ExternalAgentConfigMigrationItemTypePlugins,
		ExternalAgentConfigMigrationItemTypeMCPServerConfig,
		ExternalAgentConfigMigrationItemTypeSubagents,
		ExternalAgentConfigMigrationItemTypeHooks,
		ExternalAgentConfigMigrationItemTypeCommands,
		ExternalAgentConfigMigrationItemTypeSessions:
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = ExternalAgentConfigMigrationItemType("")
	_ json.Unmarshaler = (*ExternalAgentConfigMigrationItemType)(nil)
)
