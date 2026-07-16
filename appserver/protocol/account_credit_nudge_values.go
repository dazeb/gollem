package protocol

import "encoding/json"

// AddCreditsNudgeCreditType is the exact closed public credit category. It is
// standalone from account eligibility, billing, and email-delivery runtime.
type AddCreditsNudgeCreditType string

const (
	AddCreditsNudgeCreditTypeCredits    AddCreditsNudgeCreditType = "credits"
	AddCreditsNudgeCreditTypeUsageLimit AddCreditsNudgeCreditType = "usage_limit"
)

func (c AddCreditsNudgeCreditType) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(c, "add-credits nudge credit type", AddCreditsNudgeCreditType.valid)
}

func (c *AddCreditsNudgeCreditType) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, c, "add-credits nudge credit type", AddCreditsNudgeCreditType.valid)
}

func (c AddCreditsNudgeCreditType) valid() bool {
	return c == AddCreditsNudgeCreditTypeCredits || c == AddCreditsNudgeCreditTypeUsageLimit
}

// AddCreditsNudgeEmailStatus is the exact closed public email outcome. It does
// not imply that Gollem sends email or implements a cooldown.
type AddCreditsNudgeEmailStatus string

const (
	AddCreditsNudgeEmailStatusSent           AddCreditsNudgeEmailStatus = "sent"
	AddCreditsNudgeEmailStatusCooldownActive AddCreditsNudgeEmailStatus = "cooldown_active"
)

func (s AddCreditsNudgeEmailStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "add-credits nudge email status", AddCreditsNudgeEmailStatus.valid)
}

func (s *AddCreditsNudgeEmailStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "add-credits nudge email status", AddCreditsNudgeEmailStatus.valid)
}

func (s AddCreditsNudgeEmailStatus) valid() bool {
	return s == AddCreditsNudgeEmailStatusSent || s == AddCreditsNudgeEmailStatusCooldownActive
}

var (
	_ json.Marshaler   = AddCreditsNudgeCreditType("")
	_ json.Unmarshaler = (*AddCreditsNudgeCreditType)(nil)
	_ json.Marshaler   = AddCreditsNudgeEmailStatus("")
	_ json.Unmarshaler = (*AddCreditsNudgeEmailStatus)(nil)
)
