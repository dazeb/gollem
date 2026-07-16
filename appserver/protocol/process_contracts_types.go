package protocol

import (
	"encoding/json"
	"errors"
)

// ProcessTerminalSize is the exact standalone PTY size contract used by
// process/spawn. The process runtime remains on its existing legacy envelope.
type ProcessTerminalSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

func (s ProcessTerminalSize) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Rows uint16 `json:"rows"`
		Cols uint16 `json:"cols"`
	}{Rows: s.Rows, Cols: s.Cols})
}

func (s *ProcessTerminalSize) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode process terminal size into nil receiver")
	}
	const objectName = "process terminal size"
	payload, err := decodeRustSerdeObject(data, objectName, "rows", "cols")
	if err != nil {
		return err
	}
	rows, err := decodeRequiredThreadItemValue[uint16](payload, objectName, "rows")
	if err != nil {
		return err
	}
	cols, err := decodeRequiredThreadItemValue[uint16](payload, objectName, "cols")
	if err != nil {
		return err
	}
	*s = ProcessTerminalSize{Rows: rows, Cols: cols}
	return nil
}

// ProcessOutputStream identifies the source stream for a process output chunk.
type ProcessOutputStream string

const (
	ProcessOutputStreamStdout ProcessOutputStream = "stdout"
	ProcessOutputStreamStderr ProcessOutputStream = "stderr"
)

func (s ProcessOutputStream) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "process output stream", ProcessOutputStream.valid)
}

func (s *ProcessOutputStream) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "process output stream", ProcessOutputStream.valid)
}

func (s ProcessOutputStream) valid() bool {
	return s == ProcessOutputStreamStdout || s == ProcessOutputStreamStderr
}

// ProcessOutputDeltaNotification is the standalone public streaming-output
// payload. It is intentionally not bound to the legacy runtime notification.
type ProcessOutputDeltaNotification struct {
	ProcessHandle string              `json:"processHandle"`
	Stream        ProcessOutputStream `json:"stream"`
	DeltaBase64   string              `json:"deltaBase64"`
	CapReached    bool                `json:"capReached"`
}

func (n ProcessOutputDeltaNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ProcessHandle string              `json:"processHandle"`
		Stream        ProcessOutputStream `json:"stream"`
		DeltaBase64   string              `json:"deltaBase64"`
		CapReached    bool                `json:"capReached"`
	}{
		ProcessHandle: n.ProcessHandle,
		Stream:        n.Stream,
		DeltaBase64:   n.DeltaBase64,
		CapReached:    n.CapReached,
	})
}

func (n *ProcessOutputDeltaNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode process output delta notification into nil receiver")
	}
	const objectName = "process output delta notification"
	payload, err := decodeRustSerdeObject(
		data, objectName, "processHandle", "stream", "deltaBase64", "capReached",
	)
	if err != nil {
		return err
	}
	processHandle, err := decodeRequiredThreadItemValue[string](payload, objectName, "processHandle")
	if err != nil {
		return err
	}
	stream, err := decodeRequiredThreadItemValue[ProcessOutputStream](payload, objectName, "stream")
	if err != nil {
		return err
	}
	deltaBase64, err := decodeRequiredThreadItemValue[string](payload, objectName, "deltaBase64")
	if err != nil {
		return err
	}
	capReached, err := decodeRequiredThreadItemValue[bool](payload, objectName, "capReached")
	if err != nil {
		return err
	}
	*n = ProcessOutputDeltaNotification{
		ProcessHandle: processHandle,
		Stream:        stream,
		DeltaBase64:   deltaBase64,
		CapReached:    capReached,
	}
	return nil
}

// ProcessExitedNotification is the standalone public final process payload.
// It remains unbound until the process runtime emits this exact wire shape.
type ProcessExitedNotification struct {
	ProcessHandle    string `json:"processHandle"`
	ExitCode         int32  `json:"exitCode"`
	Stdout           string `json:"stdout"`
	StdoutCapReached bool   `json:"stdoutCapReached"`
	Stderr           string `json:"stderr"`
	StderrCapReached bool   `json:"stderrCapReached"`
}

func (n ProcessExitedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		ProcessHandle    string `json:"processHandle"`
		ExitCode         int32  `json:"exitCode"`
		Stdout           string `json:"stdout"`
		StdoutCapReached bool   `json:"stdoutCapReached"`
		Stderr           string `json:"stderr"`
		StderrCapReached bool   `json:"stderrCapReached"`
	}{
		ProcessHandle:    n.ProcessHandle,
		ExitCode:         n.ExitCode,
		Stdout:           n.Stdout,
		StdoutCapReached: n.StdoutCapReached,
		Stderr:           n.Stderr,
		StderrCapReached: n.StderrCapReached,
	})
}

func (n *ProcessExitedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode process exited notification into nil receiver")
	}
	const objectName = "process exited notification"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"processHandle", "exitCode", "stdout", "stdoutCapReached", "stderr", "stderrCapReached",
	)
	if err != nil {
		return err
	}
	processHandle, err := decodeRequiredThreadItemValue[string](payload, objectName, "processHandle")
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
	stdoutCapReached, err := decodeRequiredThreadItemValue[bool](payload, objectName, "stdoutCapReached")
	if err != nil {
		return err
	}
	stderr, err := decodeRequiredThreadItemValue[string](payload, objectName, "stderr")
	if err != nil {
		return err
	}
	stderrCapReached, err := decodeRequiredThreadItemValue[bool](payload, objectName, "stderrCapReached")
	if err != nil {
		return err
	}
	*n = ProcessExitedNotification{
		ProcessHandle:    processHandle,
		ExitCode:         exitCode,
		Stdout:           stdout,
		StdoutCapReached: stdoutCapReached,
		Stderr:           stderr,
		StderrCapReached: stderrCapReached,
	}
	return nil
}

func processTerminalSizeSchema() Schema {
	return Schema{
		"description": "PTY size in character cells for `process/spawn` PTY sessions.",
		"properties": Schema{
			"cols": Schema{
				"description": "Terminal width in character cells.",
				"format":      "uint16", "minimum": float64(0), "type": "integer",
			},
			"rows": Schema{
				"description": "Terminal height in character cells.",
				"format":      "uint16", "minimum": float64(0), "type": "integer",
			},
		},
		"required": []string{"cols", "rows"},
		"type":     "object",
	}
}

func processOutputStreamSchema() Schema {
	return Schema{
		"description": "Stream label for `process/outputDelta` notifications.",
		"oneOf": []any{
			Schema{
				"description": "stdout stream. PTY mode multiplexes terminal output here.",
				"enum":        []any{"stdout"}, "type": "string",
			},
			Schema{
				"description": "stderr stream.",
				"enum":        []any{"stderr"}, "type": "string",
			},
		},
	}
}

func processOutputDeltaNotificationSchema() Schema {
	return Schema{
		"description": "Base64-encoded output chunk emitted for a streaming `process/spawn` request.",
		"properties": Schema{
			"capReached": Schema{
				"description": "True on the final streamed chunk for this stream when output was truncated by `outputBytesCap`.",
				"type":        "boolean",
			},
			"deltaBase64": Schema{
				"description": "Base64-encoded output bytes.",
				"type":        "string",
			},
			"processHandle": Schema{
				"description": "Client-supplied, connection-scoped `processHandle` from `process/spawn`.",
				"type":        "string",
			},
			"stream": Schema{
				"allOf":       []any{Schema{"$ref": "#/$defs/ProcessOutputStream"}},
				"description": "Output stream this chunk belongs to.",
			},
		},
		"required": []string{"capReached", "deltaBase64", "processHandle", "stream"},
		"type":     "object",
	}
}

func processExitedNotificationSchema() Schema {
	return Schema{
		"description": "Final process exit notification for `process/spawn`.",
		"properties": Schema{
			"exitCode": Schema{
				"description": "Process exit code.",
				"format":      "int32", "type": "integer",
			},
			"processHandle": Schema{
				"description": "Client-supplied, connection-scoped `processHandle` from `process/spawn`.",
				"type":        "string",
			},
			"stderr": Schema{
				"description": "Buffered stderr capture.\n\nEmpty when stderr was streamed via `process/outputDelta`.",
				"type":        "string",
			},
			"stderrCapReached": Schema{
				"description": "Whether stderr reached `outputBytesCap`.\n\nIn streaming mode, stderr is empty and cap state is also reported on the final stderr `process/outputDelta` notification.",
				"type":        "boolean",
			},
			"stdout": Schema{
				"description": "Buffered stdout capture.\n\nEmpty when stdout was streamed via `process/outputDelta`.",
				"type":        "string",
			},
			"stdoutCapReached": Schema{
				"description": "Whether stdout reached `outputBytesCap`.\n\nIn streaming mode, stdout is empty and cap state is also reported on the final stdout `process/outputDelta` notification.",
				"type":        "boolean",
			},
		},
		"required": []string{
			"exitCode", "processHandle", "stderr", "stderrCapReached", "stdout", "stdoutCapReached",
		},
		"type": "object",
	}
}

var (
	_ json.Marshaler   = ProcessTerminalSize{}
	_ json.Unmarshaler = (*ProcessTerminalSize)(nil)
	_ json.Marshaler   = ProcessOutputStream("")
	_ json.Unmarshaler = (*ProcessOutputStream)(nil)
	_ json.Marshaler   = ProcessOutputDeltaNotification{}
	_ json.Unmarshaler = (*ProcessOutputDeltaNotification)(nil)
	_ json.Marshaler   = ProcessExitedNotification{}
	_ json.Unmarshaler = (*ProcessExitedNotification)(nil)
)
