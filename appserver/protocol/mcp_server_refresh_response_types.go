package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// McpServerRefreshResponse is the exact empty public MCP refresh response. It
// remains standalone from Gollem's broader live registry reload result.
type McpServerRefreshResponse struct{}

func (r *McpServerRefreshResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode MCP server refresh response into nil receiver")
	}

	// Serde ignores unknown fields for this empty struct. Decode through a map
	// to preserve that input compatibility while still requiring one object.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("decode MCP server refresh response: %w", err)
	}
	if object == nil {
		return errors.New("MCP server refresh response must be an object")
	}

	*r = McpServerRefreshResponse{}
	return nil
}

var _ json.Unmarshaler = (*McpServerRefreshResponse)(nil)
