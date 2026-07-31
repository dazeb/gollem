package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ConversationTextRole is the role associated with realtime conversation text.
// Gollem does not currently expose the source realtime methods, so this remains
// a standalone public definition.
type ConversationTextRole string

const (
	ConversationTextRoleUser      ConversationTextRole = "user"
	ConversationTextRoleDeveloper ConversationTextRole = "developer"
	ConversationTextRoleAssistant ConversationTextRole = "assistant"
)

func (r ConversationTextRole) MarshalJSON() ([]byte, error) {
	if !r.valid() {
		return nil, fmt.Errorf("unknown conversation text role %q", r)
	}
	return json.Marshal(string(r))
}

func (r *ConversationTextRole) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode conversation text role into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("conversation text role cannot be null")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode conversation text role: %w", err)
	}
	role := ConversationTextRole(value)
	if !role.valid() {
		return fmt.Errorf("unknown conversation text role %q", value)
	}
	*r = role
	return nil
}

func (r ConversationTextRole) valid() bool {
	switch r {
	case ConversationTextRoleUser, ConversationTextRoleDeveloper, ConversationTextRoleAssistant:
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = ConversationTextRole("")
	_ json.Unmarshaler = (*ConversationTextRole)(nil)
)
