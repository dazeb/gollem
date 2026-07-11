package protocol

import "encoding/json"

type CommandExecTerminalSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type CommandExecOutputStream string

const (
	CommandExecOutputStdout CommandExecOutputStream = "stdout"
	CommandExecOutputStderr CommandExecOutputStream = "stderr"
)

type CommandExecOutputDeltaNotification struct {
	ProcessID   string                  `json:"processId"`
	Stream      CommandExecOutputStream `json:"stream"`
	DeltaBase64 string                  `json:"deltaBase64"`
	CapReached  bool                    `json:"capReached"`
}

type CommandExecWriteParams struct {
	ProcessID   string  `json:"processId,omitempty"`
	DeltaBase64 *string `json:"deltaBase64,omitempty"`
	CloseStdin  bool    `json:"closeStdin,omitempty"`

	ID       string `json:"id,omitempty"`
	Data     string `json:"data,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	Close    bool   `json:"close,omitempty"`

	processIDPresent   bool
	deltaBase64Present bool
	closeStdinPresent  bool
}

func (p *CommandExecWriteParams) UnmarshalJSON(data []byte) error {
	type wire CommandExecWriteParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = CommandExecWriteParams(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, p.processIDPresent = fields["processId"]
	_, p.deltaBase64Present = fields["deltaBase64"]
	_, p.closeStdinPresent = fields["closeStdin"]
	return nil
}

func (p CommandExecWriteParams) MarshalJSON() ([]byte, error) {
	type wire CommandExecWriteParams
	explicit := make(map[string]json.RawMessage, 3)
	if p.processIDPresent && p.ProcessID == "" {
		explicit["processId"] = json.RawMessage(`""`)
	}
	if p.HasDeltaBase64() && p.DeltaBase64 == nil {
		explicit["deltaBase64"] = json.RawMessage("null")
	}
	if p.closeStdinPresent && !p.CloseStdin {
		explicit["closeStdin"] = json.RawMessage("false")
	}
	return marshalCommandExecWithExplicitFields(wire(p), explicit)
}

func (p CommandExecWriteParams) HasProcessID() bool {
	return p.processIDPresent || p.ProcessID != ""
}

func (p *CommandExecWriteParams) SetProcessID(value string) {
	p.ProcessID = value
	p.processIDPresent = true
}

func (p CommandExecWriteParams) HasDeltaBase64() bool {
	return p.deltaBase64Present || p.DeltaBase64 != nil
}

func (p *CommandExecWriteParams) SetDeltaBase64(value *string) {
	p.DeltaBase64 = value
	p.deltaBase64Present = true
}

func (p CommandExecWriteParams) HasCloseStdin() bool {
	return p.closeStdinPresent || p.CloseStdin
}

func (p *CommandExecWriteParams) SetCloseStdin(value bool) {
	p.CloseStdin = value
	p.closeStdinPresent = true
}

func (p CommandExecWriteParams) EffectiveProcessID() string {
	return effectiveCommandExecProcessID(p.HasProcessID(), p.ProcessID, p.ID)
}

func (p CommandExecWriteParams) EffectiveInput() (string, string) {
	if p.HasDeltaBase64() {
		if p.DeltaBase64 == nil {
			return "", "base64"
		}
		return *p.DeltaBase64, "base64"
	}
	return p.Data, p.Encoding
}

func (p CommandExecWriteParams) ShouldCloseStdin() bool {
	if p.HasCloseStdin() {
		return p.CloseStdin
	}
	return p.Close
}

type CommandExecWriteResponse struct {
	OK   bool   `json:"ok,omitempty"`
	Path string `json:"path,omitempty"`
}

type CommandExecTerminateParams struct {
	ProcessID string `json:"processId,omitempty"`
	ID        string `json:"id,omitempty"`

	processIDPresent bool
}

func (p *CommandExecTerminateParams) UnmarshalJSON(data []byte) error {
	type wire CommandExecTerminateParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = CommandExecTerminateParams(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, p.processIDPresent = fields["processId"]
	return nil
}

func (p CommandExecTerminateParams) MarshalJSON() ([]byte, error) {
	type wire CommandExecTerminateParams
	explicit := make(map[string]json.RawMessage, 1)
	if p.processIDPresent && p.ProcessID == "" {
		explicit["processId"] = json.RawMessage(`""`)
	}
	return marshalCommandExecWithExplicitFields(wire(p), explicit)
}

func (p CommandExecTerminateParams) HasProcessID() bool {
	return p.processIDPresent || p.ProcessID != ""
}

func (p *CommandExecTerminateParams) SetProcessID(value string) {
	p.ProcessID = value
	p.processIDPresent = true
}

func (p CommandExecTerminateParams) EffectiveProcessID() string {
	return effectiveCommandExecProcessID(p.HasProcessID(), p.ProcessID, p.ID)
}

type CommandExecTerminateResponse struct {
	OK   bool   `json:"ok,omitempty"`
	Path string `json:"path,omitempty"`
}

type CommandExecResizeParams struct {
	ProcessID string                   `json:"processId,omitempty"`
	Size      *CommandExecTerminalSize `json:"size,omitempty" jsonschema:"nonnullable=true"`

	ID   string `json:"id,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`

	processIDPresent bool
	sizePresent      bool
}

func (p *CommandExecResizeParams) UnmarshalJSON(data []byte) error {
	type wire CommandExecResizeParams
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = CommandExecResizeParams(decoded)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	_, p.processIDPresent = fields["processId"]
	_, p.sizePresent = fields["size"]
	return nil
}

func (p CommandExecResizeParams) MarshalJSON() ([]byte, error) {
	type wire CommandExecResizeParams
	explicit := make(map[string]json.RawMessage, 2)
	if p.processIDPresent && p.ProcessID == "" {
		explicit["processId"] = json.RawMessage(`""`)
	}
	if p.HasSize() && p.Size == nil {
		explicit["size"] = json.RawMessage("null")
	}
	return marshalCommandExecWithExplicitFields(wire(p), explicit)
}

func (p CommandExecResizeParams) HasProcessID() bool {
	return p.processIDPresent || p.ProcessID != ""
}

func (p *CommandExecResizeParams) SetProcessID(value string) {
	p.ProcessID = value
	p.processIDPresent = true
}

func (p CommandExecResizeParams) HasSize() bool {
	return p.sizePresent || p.Size != nil
}

func (p *CommandExecResizeParams) SetSize(value *CommandExecTerminalSize) {
	p.Size = value
	p.sizePresent = true
}

func (p CommandExecResizeParams) EffectiveProcessID() string {
	return effectiveCommandExecProcessID(p.HasProcessID(), p.ProcessID, p.ID)
}

func (p CommandExecResizeParams) EffectiveSize() (cols, rows int, valid bool) {
	if p.HasSize() {
		if p.Size == nil {
			return 0, 0, false
		}
		return int(p.Size.Cols), int(p.Size.Rows), true
	}
	return p.Cols, p.Rows, true
}

type CommandExecResizeResponse struct {
	OK   bool   `json:"ok,omitempty"`
	Path string `json:"path,omitempty"`
}

func effectiveCommandExecProcessID(publicPresent bool, processID, legacyID string) string {
	if publicPresent {
		return processID
	}
	return legacyID
}

func marshalCommandExecWithExplicitFields(value any, explicit map[string]json.RawMessage) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil || len(explicit) == 0 {
		return data, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for name, raw := range explicit {
		fields[name] = raw
	}
	return json.Marshal(fields)
}
