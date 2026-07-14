package protocol

import "encoding/json"

// AutoReviewDecisionSource is the exact closed public source for a terminal
// approval auto-review decision. It is standalone from Guardian review runtime.
type AutoReviewDecisionSource string

const (
	AutoReviewDecisionSourceAgent AutoReviewDecisionSource = "agent"
)

func (s AutoReviewDecisionSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(
		s,
		"auto-review decision source",
		AutoReviewDecisionSource.valid,
	)
}

func (s *AutoReviewDecisionSource) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(
		data,
		s,
		"auto-review decision source",
		AutoReviewDecisionSource.valid,
	)
}

func (s AutoReviewDecisionSource) valid() bool {
	return s == AutoReviewDecisionSourceAgent
}

var (
	_ json.Marshaler   = AutoReviewDecisionSource("")
	_ json.Unmarshaler = (*AutoReviewDecisionSource)(nil)
)
