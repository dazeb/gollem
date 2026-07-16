package protocol

import "encoding/json"

// AppToolApproval is the exact closed descriptive approval mode for app-tool
// configuration. Permission evaluation and app-tool execution remain absent.
type AppToolApproval string

const (
	AppToolApprovalAuto    AppToolApproval = "auto"
	AppToolApprovalPrompt  AppToolApproval = "prompt"
	AppToolApprovalWrites  AppToolApproval = "writes"
	AppToolApprovalApprove AppToolApproval = "approve"
)

func (a AppToolApproval) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(a, "app tool approval", AppToolApproval.valid)
}

func (a *AppToolApproval) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, a, "app tool approval", AppToolApproval.valid)
}

func (a AppToolApproval) valid() bool {
	switch a {
	case AppToolApprovalAuto,
		AppToolApprovalPrompt,
		AppToolApprovalWrites,
		AppToolApprovalApprove:
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = AppToolApproval("")
	_ json.Unmarshaler = (*AppToolApproval)(nil)
)
