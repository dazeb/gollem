package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Account is exact standalone public account identity data. It does not expose
// credentials or imply that Gollem owns authentication state.
type Account struct{ raw json.RawMessage }

func (a Account) MarshalJSON() ([]byte, error) {
	if len(a.raw) == 0 {
		return nil, errors.New("account is empty")
	}
	return validateAccountJSON(a.raw)
}

func (a *Account) UnmarshalJSON(data []byte) error {
	if a == nil {
		return errors.New("decode account into nil receiver")
	}
	canonical, err := validateAccountJSON(data)
	if err != nil {
		return err
	}
	a.raw = canonical
	return nil
}

// Type returns the public account discriminant, or an empty string for an
// uninitialized or invalid internal value.
func (a Account) Type() string { return loginAccountDiscriminant(a.raw) }

func validateAccountJSON(data []byte) ([]byte, error) {
	discriminant, err := decodeRustSerdeObject(data, "account", "type")
	if err != nil {
		return nil, err
	}
	typeName, err := decodeRequiredThreadItemValue[string](discriminant, "account", "type")
	if err != nil {
		return nil, err
	}
	switch typeName {
	case "apiKey":
		return marshalLoginAccountType(typeName)
	case "chatgpt":
		payload, err := decodeRustSerdeObject(data, "chatgpt account", "type", "email", "planType")
		if err != nil {
			return nil, err
		}
		email, err := decodeOptionalNullableConfigValue[string](payload, "chatgpt account", "email")
		if err != nil {
			return nil, err
		}
		planType, err := decodeRequiredThreadItemValue[PlanType](payload, "chatgpt account", "planType")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type     string   `json:"type"`
			Email    *string  `json:"email"`
			PlanType PlanType `json:"planType"`
		}{Type: typeName, Email: email, PlanType: planType})
	case "amazonBedrock":
		payload, err := decodeRustSerdeObject(data, "amazonBedrock account", "type", "credentialSource")
		if err != nil {
			return nil, err
		}
		credentialSource := AmazonBedrockCredentialSourceAWSManaged
		if raw, ok := payload["credentialSource"]; ok {
			if isJSONNull(raw) {
				return nil, errors.New("amazonBedrock account credentialSource cannot be null")
			}
			if err := json.Unmarshal(raw, &credentialSource); err != nil {
				return nil, fmt.Errorf("decode amazonBedrock account credentialSource: %w", err)
			}
		}
		return json.Marshal(struct {
			Type             string                        `json:"type"`
			CredentialSource AmazonBedrockCredentialSource `json:"credentialSource"`
		}{Type: typeName, CredentialSource: credentialSource})
	default:
		return nil, fmt.Errorf("unsupported account type %q", typeName)
	}
}

// GetAccountParams is the exact standalone account-read request.
type GetAccountParams struct {
	RefreshToken bool `json:"refreshToken,omitempty"`
}

func (p *GetAccountParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode get-account params into nil receiver")
	}
	payload, err := decodeRustSerdeObject(data, "get-account params", "refreshToken")
	if err != nil {
		return err
	}
	refreshToken := false
	if raw, ok := payload["refreshToken"]; ok {
		if isJSONNull(raw) {
			return errors.New("get-account params refreshToken cannot be null")
		}
		if err := json.Unmarshal(raw, &refreshToken); err != nil {
			return fmt.Errorf("decode get-account params refreshToken: %w", err)
		}
	}
	*p = GetAccountParams{RefreshToken: refreshToken}
	return nil
}

// GetAccountResponse is exact standalone account-read data.
type GetAccountResponse struct {
	Account            *Account `json:"account,omitempty"`
	RequiresOpenAIAuth bool     `json:"requiresOpenaiAuth"`
}

func (r GetAccountResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Account            *Account `json:"account"`
		RequiresOpenAIAuth bool     `json:"requiresOpenaiAuth"`
	}{Account: r.Account, RequiresOpenAIAuth: r.RequiresOpenAIAuth})
}

func (r *GetAccountResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode get-account response into nil receiver")
	}
	const objectName = "get-account response"
	payload, err := decodeRustSerdeObject(data, objectName, "account", "requiresOpenaiAuth")
	if err != nil {
		return err
	}
	account, err := decodeOptionalNullableConfigValue[Account](payload, objectName, "account")
	if err != nil {
		return err
	}
	requiresOpenAIAuth, err := decodeRequiredThreadItemValue[bool](payload, objectName, "requiresOpenaiAuth")
	if err != nil {
		return err
	}
	*r = GetAccountResponse{Account: account, RequiresOpenAIAuth: requiresOpenAIAuth}
	return nil
}

// AccountUpdatedNotification is exact standalone account metadata.
type AccountUpdatedNotification struct {
	AuthMode *AuthMode `json:"authMode,omitempty"`
	PlanType *PlanType `json:"planType,omitempty"`
}

func (n AccountUpdatedNotification) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		AuthMode *AuthMode `json:"authMode"`
		PlanType *PlanType `json:"planType"`
	}{AuthMode: n.AuthMode, PlanType: n.PlanType})
}

func (n *AccountUpdatedNotification) UnmarshalJSON(data []byte) error {
	if n == nil {
		return errors.New("decode account-updated notification into nil receiver")
	}
	const objectName = "account-updated notification"
	payload, err := decodeRustSerdeObject(data, objectName, "authMode", "planType")
	if err != nil {
		return err
	}
	authMode, err := decodeOptionalNullableConfigValue[AuthMode](payload, objectName, "authMode")
	if err != nil {
		return err
	}
	planType, err := decodeOptionalNullableConfigValue[PlanType](payload, objectName, "planType")
	if err != nil {
		return err
	}
	*n = AccountUpdatedNotification{AuthMode: authMode, PlanType: planType}
	return nil
}

// GetAccountTokenUsageResponse is exact standalone usage-read data.
type GetAccountTokenUsageResponse struct {
	Summary           AccountTokenUsageSummary       `json:"summary"`
	DailyUsageBuckets []AccountTokenUsageDailyBucket `json:"dailyUsageBuckets,omitempty"`
}

func (r GetAccountTokenUsageResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Summary           AccountTokenUsageSummary       `json:"summary"`
		DailyUsageBuckets []AccountTokenUsageDailyBucket `json:"dailyUsageBuckets"`
	}{Summary: r.Summary, DailyUsageBuckets: r.DailyUsageBuckets})
}

func (r *GetAccountTokenUsageResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode account token-usage response into nil receiver")
	}
	const objectName = "account token-usage response"
	payload, err := decodeRustSerdeObject(data, objectName, "summary", "dailyUsageBuckets")
	if err != nil {
		return err
	}
	summary, err := decodeRequiredThreadItemValue[AccountTokenUsageSummary](payload, objectName, "summary")
	if err != nil {
		return err
	}
	dailyUsageBuckets, err := decodeOptionalNullableRateLimitValue[[]AccountTokenUsageDailyBucket](payload, objectName, "dailyUsageBuckets")
	if err != nil {
		return err
	}
	*r = GetAccountTokenUsageResponse{Summary: summary, DailyUsageBuckets: dailyUsageBuckets}
	return nil
}

// SendAddCreditsNudgeEmailParams is exact standalone nudge request data.
type SendAddCreditsNudgeEmailParams struct {
	CreditType AddCreditsNudgeCreditType `json:"creditType"`
}

func (p *SendAddCreditsNudgeEmailParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode send-add-credits-nudge-email params into nil receiver")
	}
	const objectName = "send-add-credits-nudge-email params"
	payload, err := decodeRustSerdeObject(data, objectName, "creditType")
	if err != nil {
		return err
	}
	creditType, err := decodeRequiredThreadItemValue[AddCreditsNudgeCreditType](payload, objectName, "creditType")
	if err != nil {
		return err
	}
	*p = SendAddCreditsNudgeEmailParams{CreditType: creditType}
	return nil
}

// SendAddCreditsNudgeEmailResponse is exact standalone nudge outcome data.
type SendAddCreditsNudgeEmailResponse struct {
	Status AddCreditsNudgeEmailStatus `json:"status"`
}

func (r *SendAddCreditsNudgeEmailResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode send-add-credits-nudge-email response into nil receiver")
	}
	const objectName = "send-add-credits-nudge-email response"
	payload, err := decodeRustSerdeObject(data, objectName, "status")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[AddCreditsNudgeEmailStatus](payload, objectName, "status")
	if err != nil {
		return err
	}
	*r = SendAddCreditsNudgeEmailResponse{Status: status}
	return nil
}

func accountEnvelopeSchemas() map[string]Schema {
	nullableRef := func(name string) Schema {
		return Schema{"anyOf": []any{Schema{"$ref": "#/$defs/" + name}, Schema{"type": "null"}}}
	}
	object := func(properties Schema, required ...string) Schema {
		out := Schema{"type": "object", "properties": properties}
		if len(required) != 0 {
			out["required"] = required
		}
		return out
	}
	return map[string]Schema{
		"Account": {"oneOf": []any{
			Schema{"properties": Schema{"type": Schema{"enum": []any{"apiKey"}, "title": "ApiKeyAccountType", "type": "string"}}, "required": []string{"type"}, "title": "ApiKeyAccount", "type": "object"},
			Schema{
				"properties": Schema{
					"email": Schema{"type": []any{"string", "null"}}, "planType": Schema{"$ref": "#/$defs/PlanType"},
					"type": Schema{"enum": []any{"chatgpt"}, "title": "ChatgptAccountType", "type": "string"},
				},
				"required": []string{"email", "planType", "type"}, "title": "ChatgptAccount", "type": "object",
			},
			Schema{
				"properties": Schema{
					"credentialSource": Schema{"allOf": []any{Schema{"$ref": "#/$defs/AmazonBedrockCredentialSource"}}, "default": "awsManaged"},
					"type":             Schema{"enum": []any{"amazonBedrock"}, "title": "AmazonBedrockAccountType", "type": "string"},
				},
				"required": []string{"type"}, "title": "AmazonBedrockAccount", "type": "object",
			},
		}},
		"AccountUpdatedNotification": object(Schema{"authMode": nullableRef("AuthMode"), "planType": nullableRef("PlanType")}),
		"GetAccountParams": object(Schema{"refreshToken": Schema{
			"description": "When `true`, requests a proactive token refresh before returning.\n\n" +
				"In managed auth mode this triggers the normal refresh-token flow. In external auth mode this flag is ignored. Clients should refresh tokens themselves and call `account/login/start` with `chatgptAuthTokens`.",
			"type": "boolean",
		}}),
		"GetAccountResponse": object(Schema{"account": nullableRef("Account"), "requiresOpenaiAuth": Schema{"type": "boolean"}}, "requiresOpenaiAuth"),
		"GetAccountTokenUsageResponse": object(Schema{
			"dailyUsageBuckets": Schema{"items": Schema{"$ref": "#/$defs/AccountTokenUsageDailyBucket"}, "type": []any{"array", "null"}},
			"summary":           Schema{"$ref": "#/$defs/AccountTokenUsageSummary"},
		}, "summary"),
		"SendAddCreditsNudgeEmailParams":   object(Schema{"creditType": Schema{"$ref": "#/$defs/AddCreditsNudgeCreditType"}}, "creditType"),
		"SendAddCreditsNudgeEmailResponse": object(Schema{"status": Schema{"$ref": "#/$defs/AddCreditsNudgeEmailStatus"}}, "status"),
	}
}

var (
	_ json.Marshaler   = Account{}
	_ json.Unmarshaler = (*Account)(nil)
	_ json.Unmarshaler = (*GetAccountParams)(nil)
	_ json.Marshaler   = GetAccountResponse{}
	_ json.Unmarshaler = (*GetAccountResponse)(nil)
	_ json.Marshaler   = AccountUpdatedNotification{}
	_ json.Unmarshaler = (*AccountUpdatedNotification)(nil)
	_ json.Marshaler   = GetAccountTokenUsageResponse{}
	_ json.Unmarshaler = (*GetAccountTokenUsageResponse)(nil)
	_ json.Unmarshaler = (*SendAddCreditsNudgeEmailParams)(nil)
	_ json.Unmarshaler = (*SendAddCreditsNudgeEmailResponse)(nil)
)
