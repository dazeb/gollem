package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// CommandExecParams is the exact standalone public command/exec request. The
// existing Gollem command/exec runtime keeps its legacy shell/process contract,
// so this type intentionally has no method binding.
type CommandExecParams struct {
	Command            []string                 `json:"command"`
	ProcessID          *string                  `json:"processId"`
	TTY                bool                     `json:"tty,omitempty"`
	StreamStdin        bool                     `json:"streamStdin,omitempty"`
	StreamStdoutStderr bool                     `json:"streamStdoutStderr,omitempty"`
	OutputBytesCap     *uint                    `json:"outputBytesCap"`
	DisableOutputCap   bool                     `json:"disableOutputCap,omitempty"`
	DisableTimeout     bool                     `json:"disableTimeout,omitempty"`
	TimeoutMS          *int64                   `json:"timeoutMs"`
	CWD                *string                  `json:"cwd"`
	Env                map[string]*string       `json:"env"`
	Size               *CommandExecTerminalSize `json:"size"`
	SandboxPolicy      *SandboxPolicy           `json:"sandboxPolicy"`
}

func (p CommandExecParams) MarshalJSON() ([]byte, error) {
	if p.Command == nil {
		return nil, errors.New("command-exec params command cannot be null")
	}
	type wire struct {
		Command            []string                 `json:"command"`
		ProcessID          *string                  `json:"processId"`
		TTY                bool                     `json:"tty,omitempty"`
		StreamStdin        bool                     `json:"streamStdin,omitempty"`
		StreamStdoutStderr bool                     `json:"streamStdoutStderr,omitempty"`
		OutputBytesCap     *uint                    `json:"outputBytesCap"`
		DisableOutputCap   bool                     `json:"disableOutputCap,omitempty"`
		DisableTimeout     bool                     `json:"disableTimeout,omitempty"`
		TimeoutMS          *int64                   `json:"timeoutMs"`
		CWD                *string                  `json:"cwd"`
		Env                map[string]*string       `json:"env"`
		Size               *CommandExecTerminalSize `json:"size"`
		SandboxPolicy      *SandboxPolicy           `json:"sandboxPolicy"`
	}
	return json.Marshal(wire(p))
}

func (p *CommandExecParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode command-exec params into nil receiver")
	}
	const objectName = "command-exec params"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"command", "processId", "tty", "streamStdin", "streamStdoutStderr", "outputBytesCap",
		"disableOutputCap", "disableTimeout", "timeoutMs", "cwd", "env", "size", "sandboxPolicy",
	)
	if err != nil {
		return err
	}
	command, err := decodeRequiredThreadItemArray[string](payload, objectName, "command")
	if err != nil {
		return err
	}
	processID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "processId")
	if err != nil {
		return err
	}
	tty, err := decodeOptionalConfigBool(payload, objectName, "tty")
	if err != nil {
		return err
	}
	streamStdin, err := decodeOptionalConfigBool(payload, objectName, "streamStdin")
	if err != nil {
		return err
	}
	streamStdoutStderr, err := decodeOptionalConfigBool(payload, objectName, "streamStdoutStderr")
	if err != nil {
		return err
	}
	outputBytesCap, err := decodeOptionalNullableConfigValue[uint](payload, objectName, "outputBytesCap")
	if err != nil {
		return err
	}
	disableOutputCap, err := decodeOptionalConfigBool(payload, objectName, "disableOutputCap")
	if err != nil {
		return err
	}
	disableTimeout, err := decodeOptionalConfigBool(payload, objectName, "disableTimeout")
	if err != nil {
		return err
	}
	timeoutMS, err := decodeOptionalNullableConfigValue[int64](payload, objectName, "timeoutMs")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	env, err := decodeOptionalNullableCommandExecEnv(payload, objectName, "env")
	if err != nil {
		return err
	}
	size, err := decodeOptionalNullableConfigValue[CommandExecTerminalSize](payload, objectName, "size")
	if err != nil {
		return err
	}
	sandboxPolicy, err := decodeOptionalNullableConfigValue[SandboxPolicy](payload, objectName, "sandboxPolicy")
	if err != nil {
		return err
	}
	*p = CommandExecParams{
		Command: command, ProcessID: processID, TTY: tty, StreamStdin: streamStdin,
		StreamStdoutStderr: streamStdoutStderr, OutputBytesCap: outputBytesCap,
		DisableOutputCap: disableOutputCap, DisableTimeout: disableTimeout, TimeoutMS: timeoutMS,
		CWD: cwd, Env: env, Size: size, SandboxPolicy: sandboxPolicy,
	}
	return nil
}

func decodeOptionalNullableCommandExecEnv(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (map[string]*string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var values map[string]*string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return values, nil
}

// CommandExecResponse is the exact standalone final buffered command result.
// Gollem's live command/exec result remains a process snapshot and is not
// widened to this incompatible response.
type CommandExecResponse struct {
	ExitCode int32  `json:"exitCode"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

func (r *CommandExecResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode command-exec response into nil receiver")
	}
	const objectName = "command-exec response"
	payload, err := decodeRustSerdeObject(data, objectName, "exitCode", "stdout", "stderr")
	if err != nil {
		return err
	}
	exitCode, err := decodeRequiredThreadItemValue[int32](payload, objectName, "exitCode")
	if err != nil {
		return err
	}
	stdout, err := decodeRequiredThreadItemValue[string](payload, objectName, "stdout")
	if err != nil {
		return err
	}
	stderr, err := decodeRequiredThreadItemValue[string](payload, objectName, "stderr")
	if err != nil {
		return err
	}
	*r = CommandExecResponse{ExitCode: exitCode, Stdout: stdout, Stderr: stderr}
	return nil
}

func commandExecContractSchemas() map[string]Schema {
	return map[string]Schema{
		"CommandExecParams": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "Run a standalone command (argv vector) in the server sandbox without creating a thread or turn.\n\nThe final `command/exec` response is deferred until the process exits and is sent only after all `command/exec/outputDelta` notifications for that connection have been emitted.",
			"properties": Schema{
				"command":            Schema{"description": "Command argv vector. Empty arrays are rejected.", "items": Schema{"type": "string"}, "type": "array"},
				"cwd":                Schema{"description": "Optional working directory. Defaults to the server cwd.", "type": []string{"string", "null"}},
				"disableOutputCap":   Schema{"description": "Disable stdout/stderr capture truncation for this request.\n\nCannot be combined with `outputBytesCap`.", "type": "boolean"},
				"disableTimeout":     Schema{"description": "Disable the timeout entirely for this request.\n\nCannot be combined with `timeoutMs`.", "type": "boolean"},
				"env":                Schema{"additionalProperties": Schema{"type": []string{"string", "null"}}, "description": "Optional environment overrides merged into the server-computed environment.\n\nMatching names override inherited values. Set a key to `null` to unset an inherited variable.", "type": []string{"object", "null"}},
				"outputBytesCap":     Schema{"description": "Optional per-stream stdout/stderr capture cap in bytes.\n\nWhen omitted, the server default applies. Cannot be combined with `disableOutputCap`.", "format": "uint", "minimum": float64(0), "type": []string{"integer", "null"}},
				"processId":          Schema{"description": "Optional client-supplied, connection-scoped process id.\n\nRequired for `tty`, `streamStdin`, `streamStdoutStderr`, and follow-up `command/exec/write`, `command/exec/resize`, and `command/exec/terminate` calls. When omitted, buffered execution gets an internal id that is not exposed to the client.", "type": []string{"string", "null"}},
				"sandboxPolicy":      Schema{"anyOf": []any{Schema{"$ref": "#/$defs/SandboxPolicy"}, Schema{"type": "null"}}, "description": "Optional sandbox policy for this command.\n\nUses the same shape as thread/turn execution sandbox configuration and defaults to the user's configured policy when omitted. Cannot be combined with `permissionProfile`."},
				"size":               Schema{"anyOf": []any{Schema{"$ref": "#/$defs/CommandExecTerminalSize"}, Schema{"type": "null"}}, "description": "Optional initial PTY size in character cells. Only valid when `tty` is true."},
				"streamStdin":        Schema{"description": "Allow follow-up `command/exec/write` requests to write stdin bytes.\n\nRequires a client-supplied `processId`.", "type": "boolean"},
				"streamStdoutStderr": Schema{"description": "Stream stdout/stderr via `command/exec/outputDelta` notifications.\n\nStreamed bytes are not duplicated into the final response and require a client-supplied `processId`.", "type": "boolean"},
				"timeoutMs":          Schema{"description": "Optional timeout in milliseconds.\n\nWhen omitted, the server default applies. Cannot be combined with `disableTimeout`.", "format": "int64", "type": []string{"integer", "null"}},
				"tty":                Schema{"description": "Enable PTY mode.\n\nThis implies `streamStdin` and `streamStdoutStderr`.", "type": "boolean"},
			},
			"required": []string{"command"},
			"title":    "CommandExecParams",
			"type":     "object",
		},
		"CommandExecResponse": {
			"$schema":     "http://json-schema.org/draft-07/schema#",
			"description": "Final buffered result for `command/exec`.",
			"properties": Schema{
				"exitCode": Schema{"description": "Process exit code.", "format": "int32", "type": "integer"},
				"stderr":   Schema{"description": "Buffered stderr capture.\n\nEmpty when stderr was streamed via `command/exec/outputDelta`.", "type": "string"},
				"stdout":   Schema{"description": "Buffered stdout capture.\n\nEmpty when stdout was streamed via `command/exec/outputDelta`.", "type": "string"},
			},
			"required": []string{"exitCode", "stderr", "stdout"},
			"title":    "CommandExecResponse",
			"type":     "object",
		},
	}
}

var (
	_ json.Marshaler   = CommandExecParams{}
	_ json.Unmarshaler = (*CommandExecParams)(nil)
	_ json.Unmarshaler = (*CommandExecResponse)(nil)
)
