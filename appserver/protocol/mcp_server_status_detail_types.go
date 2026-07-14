package protocol

import "encoding/json"

// McpServerStatusDetail is the exact closed public MCP status detail selector.
// It remains standalone from Gollem's broader status request and runtime.
type McpServerStatusDetail string

const (
	McpServerStatusDetailFull             McpServerStatusDetail = "full"
	McpServerStatusDetailToolsAndAuthOnly McpServerStatusDetail = "toolsAndAuthOnly"
)

func (d McpServerStatusDetail) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(d, "MCP server status detail", McpServerStatusDetail.valid)
}

func (d *McpServerStatusDetail) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, d, "MCP server status detail", McpServerStatusDetail.valid)
}

func (d McpServerStatusDetail) valid() bool {
	return d == McpServerStatusDetailFull ||
		d == McpServerStatusDetailToolsAndAuthOnly
}

var (
	_ json.Marshaler   = McpServerStatusDetail("")
	_ json.Unmarshaler = (*McpServerStatusDetail)(nil)
)
