package protocol

import "encoding/json"

// AppTemplateUnavailableReason is the exact closed reason a public app
// template cannot be materialized. It remains standalone from workspace and
// plugin runtime behavior.
type AppTemplateUnavailableReason string

const (
	AppTemplateUnavailableReasonNotConfiguredForWorkspace AppTemplateUnavailableReason = "NOT_CONFIGURED_FOR_WORKSPACE"
	AppTemplateUnavailableReasonNoActiveWorkspace         AppTemplateUnavailableReason = "NO_ACTIVE_WORKSPACE"
)

func (r AppTemplateUnavailableReason) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "app template unavailable reason", AppTemplateUnavailableReason.valid)
}

func (r *AppTemplateUnavailableReason) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "app template unavailable reason", AppTemplateUnavailableReason.valid)
}

func (r AppTemplateUnavailableReason) valid() bool {
	switch r {
	case AppTemplateUnavailableReasonNotConfiguredForWorkspace,
		AppTemplateUnavailableReasonNoActiveWorkspace:
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = AppTemplateUnavailableReason("")
	_ json.Unmarshaler = (*AppTemplateUnavailableReason)(nil)
)
