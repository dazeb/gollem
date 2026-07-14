package protocol

import (
	"encoding/json"
	"errors"
)

// CancelLoginAccountParams is the exact public login-cancellation target. It
// remains standalone from Gollem account and authentication runtime.
type CancelLoginAccountParams struct {
	LoginID string `json:"loginId"`
}

func (p *CancelLoginAccountParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode cancel-login account params into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"cancel-login account params",
		"loginId",
	)
	if err != nil {
		return err
	}
	loginID, err := decodeRequiredThreadItemValue[string](
		payload,
		"cancel-login account params",
		"loginId",
	)
	if err != nil {
		return err
	}
	*p = CancelLoginAccountParams{LoginID: loginID}
	return nil
}

// CancelLoginAccountStatus is the exact closed public cancellation outcome.
type CancelLoginAccountStatus string

const (
	CancelLoginAccountStatusCanceled CancelLoginAccountStatus = "canceled"
	CancelLoginAccountStatusNotFound CancelLoginAccountStatus = "notFound"
)

func (s CancelLoginAccountStatus) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(
		s,
		"cancel-login account status",
		CancelLoginAccountStatus.valid,
	)
}

func (s *CancelLoginAccountStatus) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(
		data,
		s,
		"cancel-login account status",
		CancelLoginAccountStatus.valid,
	)
}

func (s CancelLoginAccountStatus) valid() bool {
	return s == CancelLoginAccountStatusCanceled || s == CancelLoginAccountStatusNotFound
}

// CancelLoginAccountResponse is the exact public cancellation outcome. It
// remains standalone from the deferred account/login/cancel method.
type CancelLoginAccountResponse struct {
	Status CancelLoginAccountStatus `json:"status"`
}

func (r *CancelLoginAccountResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode cancel-login account response into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"cancel-login account response",
		"status",
	)
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[CancelLoginAccountStatus](
		payload,
		"cancel-login account response",
		"status",
	)
	if err != nil {
		return err
	}
	*r = CancelLoginAccountResponse{Status: status}
	return nil
}

var (
	_ json.Unmarshaler = (*CancelLoginAccountParams)(nil)
	_ json.Marshaler   = CancelLoginAccountStatus("")
	_ json.Unmarshaler = (*CancelLoginAccountStatus)(nil)
	_ json.Unmarshaler = (*CancelLoginAccountResponse)(nil)
)
