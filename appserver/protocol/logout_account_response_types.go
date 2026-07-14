package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// LogoutAccountResponse is the exact empty public logout response. It remains
// standalone until Gollem implements account state and logout behavior.
type LogoutAccountResponse struct{}

func (r *LogoutAccountResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode logout account response into nil receiver")
	}

	// Serde ignores unknown fields for this empty struct. Decode through a map
	// to preserve that input compatibility while still requiring one object.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("decode logout account response: %w", err)
	}
	if object == nil {
		return errors.New("logout account response must be an object")
	}

	*r = LogoutAccountResponse{}
	return nil
}

var _ json.Unmarshaler = (*LogoutAccountResponse)(nil)
