package protocol

import (
	"encoding/json"
	"errors"
)

// ChatgptAuthTokensRefreshReason is the exact closed public reason for a
// client-managed ChatGPT token refresh. It carries no authentication authority.
type ChatgptAuthTokensRefreshReason string

const (
	ChatgptAuthTokensRefreshReasonUnauthorized ChatgptAuthTokensRefreshReason = "unauthorized"
)

func (r ChatgptAuthTokensRefreshReason) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "ChatGPT auth-tokens refresh reason", ChatgptAuthTokensRefreshReason.valid)
}

func (r *ChatgptAuthTokensRefreshReason) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "ChatGPT auth-tokens refresh reason", ChatgptAuthTokensRefreshReason.valid)
}

func (r ChatgptAuthTokensRefreshReason) valid() bool {
	return r == ChatgptAuthTokensRefreshReasonUnauthorized
}

// ChatgptAuthTokensRefreshParams is exact standalone public refresh-request
// data. The corresponding sensitive server request remains deferred and unbound.
type ChatgptAuthTokensRefreshParams struct {
	Reason            ChatgptAuthTokensRefreshReason `json:"reason"`
	PreviousAccountID *string                        `json:"previousAccountId,omitempty"`
}

func (p ChatgptAuthTokensRefreshParams) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"reason":            p.Reason,
		"previousAccountId": p.PreviousAccountID,
	})
}

func (p *ChatgptAuthTokensRefreshParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode ChatGPT auth-tokens refresh params into nil receiver")
	}
	const objectName = "ChatGPT auth-tokens refresh params"
	payload, err := decodeRustSerdeObject(data, objectName, "reason", "previousAccountId")
	if err != nil {
		return err
	}
	reason, err := decodeRequiredThreadItemValue[ChatgptAuthTokensRefreshReason](
		payload, objectName, "reason",
	)
	if err != nil {
		return err
	}
	previousAccountID, err := decodeOptionalNullableConfigValue[string](
		payload, objectName, "previousAccountId",
	)
	if err != nil {
		return err
	}
	*p = ChatgptAuthTokensRefreshParams{
		Reason:            reason,
		PreviousAccountID: previousAccountID,
	}
	return nil
}

// ChatgptAuthTokensRefreshResponse is exact standalone public refresh-response
// data. Gollem does not receive, store, log, validate, or return these tokens.
type ChatgptAuthTokensRefreshResponse struct {
	AccessToken      string  `json:"accessToken"` //nolint:gosec // Exact standalone protocol field; no runtime consumes it.
	ChatgptAccountID string  `json:"chatgptAccountId"`
	ChatgptPlanType  *string `json:"chatgptPlanType,omitempty"`
}

func (r ChatgptAuthTokensRefreshResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"accessToken":      r.AccessToken,
		"chatgptAccountId": r.ChatgptAccountID,
		"chatgptPlanType":  r.ChatgptPlanType,
	})
}

func (r *ChatgptAuthTokensRefreshResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode ChatGPT auth-tokens refresh response into nil receiver")
	}
	const objectName = "ChatGPT auth-tokens refresh response"
	payload, err := decodeRustSerdeObject(
		data, objectName, "accessToken", "chatgptAccountId", "chatgptPlanType",
	)
	if err != nil {
		return err
	}
	accessToken, err := decodeRequiredThreadItemValue[string](payload, objectName, "accessToken")
	if err != nil {
		return err
	}
	accountID, err := decodeRequiredThreadItemValue[string](payload, objectName, "chatgptAccountId")
	if err != nil {
		return err
	}
	planType, err := decodeOptionalNullableConfigValue[string](payload, objectName, "chatgptPlanType")
	if err != nil {
		return err
	}
	*r = ChatgptAuthTokensRefreshResponse{
		AccessToken:      accessToken,
		ChatgptAccountID: accountID,
		ChatgptPlanType:  planType,
	}
	return nil
}

var (
	_ json.Marshaler   = ChatgptAuthTokensRefreshReason("")
	_ json.Unmarshaler = (*ChatgptAuthTokensRefreshReason)(nil)
	_ json.Marshaler   = ChatgptAuthTokensRefreshParams{}
	_ json.Unmarshaler = (*ChatgptAuthTokensRefreshParams)(nil)
	_ json.Marshaler   = ChatgptAuthTokensRefreshResponse{}
	_ json.Unmarshaler = (*ChatgptAuthTokensRefreshResponse)(nil)
)
