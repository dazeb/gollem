package protocol

import (
	"encoding/json"
	"errors"
)

// ThreadShellCommandParams is the exact public app-server host-shell request.
// It remains standalone because Gollem accepts a legacy id alias and executes
// its own durable, approval-gated process/turn workflow.
type ThreadShellCommandParams struct {
	ThreadID string `json:"threadId"`
	Command  string `json:"command"`
}

func (p *ThreadShellCommandParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode thread-shell-command params into nil receiver")
	}
	const objectName = "thread-shell-command params"
	payload, err := decodeRustSerdeObject(data, objectName, "threadId", "command")
	if err != nil {
		return err
	}
	threadID, err := decodeRequiredThreadItemValue[string](payload, objectName, "threadId")
	if err != nil {
		return err
	}
	command, err := decodeRequiredThreadItemValue[string](payload, objectName, "command")
	if err != nil {
		return err
	}
	*p = ThreadShellCommandParams{ThreadID: threadID, Command: command}
	return nil
}

// ThreadShellCommandResponse is the exact empty public response. It remains
// standalone because no public binding may imply that Gollem's durable process
// operation has the source contract's unsandboxed host-shell semantics.
type ThreadShellCommandResponse struct{}

func (r *ThreadShellCommandResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode thread-shell-command response into nil receiver")
	}
	if _, err := decodeRustSerdeObject(data, "thread-shell-command response"); err != nil {
		return err
	}
	*r = ThreadShellCommandResponse{}
	return nil
}

func threadShellCommandSchemas() map[string]Schema {
	return map[string]Schema{
		"ThreadShellCommandParams": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"properties": Schema{
				"command": Schema{
					"description": "Shell command string evaluated by the thread's configured shell. Unlike `command/exec`, this intentionally preserves shell syntax such as pipes, redirects, and quoting. This runs unsandboxed with full access rather than inheriting the thread sandbox policy.",
					"type":        "string",
				},
				"threadId": Schema{"type": "string"},
			},
			"required": []string{"command", "threadId"},
			"title":    "ThreadShellCommandParams",
			"type":     "object",
		},
		"ThreadShellCommandResponse": {
			"$schema": "http://json-schema.org/draft-07/schema#",
			"title":   "ThreadShellCommandResponse",
			"type":    "object",
		},
	}
}

var (
	_ json.Unmarshaler = (*ThreadShellCommandParams)(nil)
	_ json.Unmarshaler = (*ThreadShellCommandResponse)(nil)
)
