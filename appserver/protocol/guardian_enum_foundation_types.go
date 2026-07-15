package protocol

import "encoding/json"

// GuardianApprovalReviewStatus is the exact closed public lifecycle state for
// an approval auto-review. It does not imply a live Guardian review producer.
type GuardianApprovalReviewStatus string

const (
	GuardianApprovalReviewStatusInProgress GuardianApprovalReviewStatus = "inProgress"
	GuardianApprovalReviewStatusApproved   GuardianApprovalReviewStatus = "approved"
	GuardianApprovalReviewStatusDenied     GuardianApprovalReviewStatus = "denied"
	GuardianApprovalReviewStatusTimedOut   GuardianApprovalReviewStatus = "timedOut"
	GuardianApprovalReviewStatusAborted    GuardianApprovalReviewStatus = "aborted"
)

func (s GuardianApprovalReviewStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "Guardian approval review status", GuardianApprovalReviewStatus.valid)
}

func (s *GuardianApprovalReviewStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "Guardian approval review status", GuardianApprovalReviewStatus.valid)
}

func (s GuardianApprovalReviewStatus) valid() bool {
	switch s {
	case GuardianApprovalReviewStatusInProgress, GuardianApprovalReviewStatusApproved,
		GuardianApprovalReviewStatusDenied, GuardianApprovalReviewStatusTimedOut,
		GuardianApprovalReviewStatusAborted:
		return true
	default:
		return false
	}
}

// GuardianCommandSource is the exact closed public source of a command under
// Guardian review. It does not grant command execution authority.
type GuardianCommandSource string

const (
	GuardianCommandSourceShell       GuardianCommandSource = "shell"
	GuardianCommandSourceUnifiedExec GuardianCommandSource = "unifiedExec"
)

func (s GuardianCommandSource) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "Guardian command source", GuardianCommandSource.valid)
}

func (s *GuardianCommandSource) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "Guardian command source", GuardianCommandSource.valid)
}

func (s GuardianCommandSource) valid() bool {
	return s == GuardianCommandSourceShell || s == GuardianCommandSourceUnifiedExec
}

// GuardianRiskLevel is the exact closed public risk classification assigned by
// an approval auto-review. It does not perform or authorize risk assessment.
type GuardianRiskLevel string

const (
	GuardianRiskLevelLow      GuardianRiskLevel = "low"
	GuardianRiskLevelMedium   GuardianRiskLevel = "medium"
	GuardianRiskLevelHigh     GuardianRiskLevel = "high"
	GuardianRiskLevelCritical GuardianRiskLevel = "critical"
)

func (l GuardianRiskLevel) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(l, "Guardian risk level", GuardianRiskLevel.valid)
}

func (l *GuardianRiskLevel) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, l, "Guardian risk level", GuardianRiskLevel.valid)
}

func (l GuardianRiskLevel) valid() bool {
	switch l {
	case GuardianRiskLevelLow, GuardianRiskLevelMedium,
		GuardianRiskLevelHigh, GuardianRiskLevelCritical:
		return true
	default:
		return false
	}
}

// GuardianUserAuthorization is the exact closed public user-authorization
// classification. It does not infer or grant user authorization.
type GuardianUserAuthorization string

const (
	GuardianUserAuthorizationUnknown GuardianUserAuthorization = "unknown"
	GuardianUserAuthorizationLow     GuardianUserAuthorization = "low"
	GuardianUserAuthorizationMedium  GuardianUserAuthorization = "medium"
	GuardianUserAuthorizationHigh    GuardianUserAuthorization = "high"
)

func (a GuardianUserAuthorization) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(a, "Guardian user authorization", GuardianUserAuthorization.valid)
}

func (a *GuardianUserAuthorization) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, a, "Guardian user authorization", GuardianUserAuthorization.valid)
}

func (a GuardianUserAuthorization) valid() bool {
	switch a {
	case GuardianUserAuthorizationUnknown, GuardianUserAuthorizationLow,
		GuardianUserAuthorizationMedium, GuardianUserAuthorizationHigh:
		return true
	default:
		return false
	}
}

// NetworkApprovalProtocol is the exact closed public protocol classification
// for network approval review. It does not grant or enforce network access.
type NetworkApprovalProtocol string

const (
	NetworkApprovalProtocolHTTP      NetworkApprovalProtocol = "http"
	NetworkApprovalProtocolHTTPS     NetworkApprovalProtocol = "https"
	NetworkApprovalProtocolSocks5TCP NetworkApprovalProtocol = "socks5Tcp"
	NetworkApprovalProtocolSocks5UDP NetworkApprovalProtocol = "socks5Udp"
)

func (p NetworkApprovalProtocol) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(p, "network approval protocol", NetworkApprovalProtocol.valid)
}

func (p *NetworkApprovalProtocol) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, p, "network approval protocol", NetworkApprovalProtocol.valid)
}

func (p NetworkApprovalProtocol) valid() bool {
	switch p {
	case NetworkApprovalProtocolHTTP, NetworkApprovalProtocolHTTPS,
		NetworkApprovalProtocolSocks5TCP, NetworkApprovalProtocolSocks5UDP:
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = GuardianApprovalReviewStatus("")
	_ json.Unmarshaler = (*GuardianApprovalReviewStatus)(nil)
	_ json.Marshaler   = GuardianCommandSource("")
	_ json.Unmarshaler = (*GuardianCommandSource)(nil)
	_ json.Marshaler   = GuardianRiskLevel("")
	_ json.Unmarshaler = (*GuardianRiskLevel)(nil)
	_ json.Marshaler   = GuardianUserAuthorization("")
	_ json.Unmarshaler = (*GuardianUserAuthorization)(nil)
	_ json.Marshaler   = NetworkApprovalProtocol("")
	_ json.Unmarshaler = (*NetworkApprovalProtocol)(nil)
)
