package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// CommandExecutionSource is the exact public command-origin contract. Gollem's
// narrower live v1 command item intentionally remains a separate inline field.
type CommandExecutionSource string

const (
	CommandExecutionSourceAgent                  CommandExecutionSource = "agent"
	CommandExecutionSourceUserShell              CommandExecutionSource = "userShell"
	CommandExecutionSourceUnifiedExecStartup     CommandExecutionSource = "unifiedExecStartup"
	CommandExecutionSourceUnifiedExecInteraction CommandExecutionSource = "unifiedExecInteraction"
)

func (s CommandExecutionSource) MarshalJSON() ([]byte, error) {
	if !s.valid() {
		return nil, fmt.Errorf("invalid command execution source %q", s)
	}
	return json.Marshal(string(s))
}

func (s *CommandExecutionSource) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode command execution source into nil receiver")
	}
	if isJSONNull(data) {
		return errors.New("command execution source cannot be null")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode command execution source: %w", err)
	}
	source := CommandExecutionSource(value)
	if !source.valid() {
		return fmt.Errorf("invalid command execution source %q", value)
	}
	*s = source
	return nil
}

func (s CommandExecutionSource) valid() bool {
	switch s {
	case CommandExecutionSourceAgent,
		CommandExecutionSourceUserShell,
		CommandExecutionSourceUnifiedExecStartup,
		CommandExecutionSourceUnifiedExecInteraction:
		return true
	default:
		return false
	}
}
