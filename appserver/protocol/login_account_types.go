package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// LoginAccountParams is the exact public login-start request union. It remains
// standalone until Gollem implements the account/login/start method itself.
type LoginAccountParams struct{ raw json.RawMessage }

// LoginAccountResponse is the exact public login-start response union. It
// remains standalone from Gollem's account and credential runtime.
type LoginAccountResponse struct{ raw json.RawMessage }

func (p LoginAccountParams) MarshalJSON() ([]byte, error) {
	if len(p.raw) == 0 {
		return nil, errors.New("login account params are empty")
	}
	return validateLoginAccountParamsJSON(p.raw)
}

func (p *LoginAccountParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode login account params into nil receiver")
	}
	canonical, err := validateLoginAccountParamsJSON(data)
	if err != nil {
		return err
	}
	p.raw = canonical
	return nil
}

// Type returns the request union's public discriminant, or an empty string for
// an uninitialized or invalid internal value.
func (p LoginAccountParams) Type() string {
	return loginAccountDiscriminant(p.raw)
}

func (r LoginAccountResponse) MarshalJSON() ([]byte, error) {
	if len(r.raw) == 0 {
		return nil, errors.New("login account response is empty")
	}
	return validateLoginAccountResponseJSON(r.raw)
}

func (r *LoginAccountResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode login account response into nil receiver")
	}
	canonical, err := validateLoginAccountResponseJSON(data)
	if err != nil {
		return err
	}
	r.raw = canonical
	return nil
}

// Type returns the response union's public discriminant, or an empty string
// for an uninitialized or invalid internal value.
func (r LoginAccountResponse) Type() string {
	return loginAccountDiscriminant(r.raw)
}

func validateLoginAccountParamsJSON(data []byte) ([]byte, error) {
	payload, err := decodeLoginAccountObject(data, "login account params")
	if err != nil {
		return nil, err
	}
	typeName, err := decodeRequiredThreadItemValue[string](payload, "login account params", "type")
	if err != nil {
		return nil, err
	}

	switch typeName {
	case "apiKey":
		if err := requireLoginAccountVariantFields(payload, typeName, "type", "apiKey"); err != nil {
			return nil, err
		}
		apiKey, err := decodeRequiredThreadItemValue[string](payload, "apiKey login account params", "apiKey")
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			//nolint:gosec // APIKey is the required public wire field name.
			APIKey string `json:"apiKey"`
		}{Type: typeName, APIKey: apiKey})
	case "chatgpt":
		if err := requireLoginAccountVariantFields(
			payload,
			typeName,
			"type",
			"codexStreamlinedLogin",
			"useHostedLoginSuccessPage",
			"appBrand",
		); err != nil {
			return nil, err
		}
		streamlined, err := decodeOptionalLoginAccountBool(
			payload, "chatgpt login account params", "codexStreamlinedLogin",
		)
		if err != nil {
			return nil, err
		}
		hostedSuccessPage, err := decodeOptionalLoginAccountBool(
			payload, "chatgpt login account params", "useHostedLoginSuccessPage",
		)
		if err != nil {
			return nil, err
		}
		appBrand, err := decodeNullableLoginAccountValue[LoginAppBrand](
			payload, "chatgpt login account params", "appBrand",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type                      string         `json:"type"`
			CodexStreamlinedLogin     bool           `json:"codexStreamlinedLogin,omitempty"`
			UseHostedLoginSuccessPage bool           `json:"useHostedLoginSuccessPage,omitempty"`
			AppBrand                  *LoginAppBrand `json:"appBrand"`
		}{
			Type:                      typeName,
			CodexStreamlinedLogin:     streamlined,
			UseHostedLoginSuccessPage: hostedSuccessPage,
			AppBrand:                  appBrand,
		})
	case "chatgptDeviceCode":
		if err := requireLoginAccountVariantFields(payload, typeName, "type"); err != nil {
			return nil, err
		}
		return marshalLoginAccountType(typeName)
	case "chatgptAuthTokens":
		if err := requireLoginAccountVariantFields(
			payload,
			typeName,
			"type",
			"accessToken",
			"chatgptAccountId",
			"chatgptPlanType",
		); err != nil {
			return nil, err
		}
		accessToken, err := decodeRequiredThreadItemValue[string](
			payload, "chatgptAuthTokens login account params", "accessToken",
		)
		if err != nil {
			return nil, err
		}
		accountID, err := decodeRequiredThreadItemValue[string](
			payload, "chatgptAuthTokens login account params", "chatgptAccountId",
		)
		if err != nil {
			return nil, err
		}
		planType, err := decodeNullableLoginAccountValue[string](
			payload, "chatgptAuthTokens login account params", "chatgptPlanType",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			//nolint:gosec // AccessToken is the required public wire field name.
			AccessToken      string  `json:"accessToken"`
			ChatGPTAccountID string  `json:"chatgptAccountId"`
			ChatGPTPlanType  *string `json:"chatgptPlanType"`
		}{
			Type:             typeName,
			AccessToken:      accessToken,
			ChatGPTAccountID: accountID,
			ChatGPTPlanType:  planType,
		})
	case "amazonBedrock":
		if err := requireLoginAccountVariantFields(payload, typeName, "type", "apiKey", "region"); err != nil {
			return nil, err
		}
		apiKey, err := decodeRequiredThreadItemValue[string](
			payload, "amazonBedrock login account params", "apiKey",
		)
		if err != nil {
			return nil, err
		}
		region, err := decodeRequiredThreadItemValue[string](
			payload, "amazonBedrock login account params", "region",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type string `json:"type"`
			//nolint:gosec // APIKey is the required public wire field name.
			APIKey string `json:"apiKey"`
			Region string `json:"region"`
		}{Type: typeName, APIKey: apiKey, Region: region})
	default:
		return nil, fmt.Errorf("unsupported login account params type %q", typeName)
	}
}

func validateLoginAccountResponseJSON(data []byte) ([]byte, error) {
	payload, err := decodeLoginAccountObject(data, "login account response")
	if err != nil {
		return nil, err
	}
	typeName, err := decodeRequiredThreadItemValue[string](payload, "login account response", "type")
	if err != nil {
		return nil, err
	}

	switch typeName {
	case "apiKey", "chatgptAuthTokens", "amazonBedrock":
		if err := requireLoginAccountVariantFields(payload, typeName, "type"); err != nil {
			return nil, err
		}
		return marshalLoginAccountType(typeName)
	case "chatgpt":
		if err := requireLoginAccountVariantFields(payload, typeName, "type", "loginId", "authUrl"); err != nil {
			return nil, err
		}
		loginID, err := decodeRequiredThreadItemValue[string](
			payload, "chatgpt login account response", "loginId",
		)
		if err != nil {
			return nil, err
		}
		authURL, err := decodeRequiredThreadItemValue[string](
			payload, "chatgpt login account response", "authUrl",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type    string `json:"type"`
			LoginID string `json:"loginId"`
			AuthURL string `json:"authUrl"`
		}{Type: typeName, LoginID: loginID, AuthURL: authURL})
	case "chatgptDeviceCode":
		if err := requireLoginAccountVariantFields(
			payload,
			typeName,
			"type",
			"loginId",
			"verificationUrl",
			"userCode",
		); err != nil {
			return nil, err
		}
		loginID, err := decodeRequiredThreadItemValue[string](
			payload, "chatgptDeviceCode login account response", "loginId",
		)
		if err != nil {
			return nil, err
		}
		verificationURL, err := decodeRequiredThreadItemValue[string](
			payload, "chatgptDeviceCode login account response", "verificationUrl",
		)
		if err != nil {
			return nil, err
		}
		userCode, err := decodeRequiredThreadItemValue[string](
			payload, "chatgptDeviceCode login account response", "userCode",
		)
		if err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			Type            string `json:"type"`
			LoginID         string `json:"loginId"`
			VerificationURL string `json:"verificationUrl"`
			UserCode        string `json:"userCode"`
		}{
			Type:            typeName,
			LoginID:         loginID,
			VerificationURL: verificationURL,
			UserCode:        userCode,
		})
	default:
		return nil, fmt.Errorf("unsupported login account response type %q", typeName)
	}
}

func decodeLoginAccountObject(data []byte, objectName string) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}

	payload := make(map[string]json.RawMessage, 9)
	seen := make(map[string]bool, 9)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode %s field name: %w", objectName, err)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode %s field %q: %w", objectName, name, err)
		}
		if !isLoginAccountKnownField(name) {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate %s field %q", objectName, name)
		}
		seen[name] = true
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%s must contain one JSON value", objectName)
		}
		return nil, fmt.Errorf("decode %s trailing value: %w", objectName, err)
	}
	return payload, nil
}

func isLoginAccountKnownField(name string) bool {
	switch name {
	case "type",
		"apiKey",
		"codexStreamlinedLogin",
		"useHostedLoginSuccessPage",
		"appBrand",
		"accessToken",
		"chatgptAccountId",
		"chatgptPlanType",
		"region",
		"loginId",
		"authUrl",
		"verificationUrl",
		"userCode":
		return true
	default:
		return false
	}
}

func requireLoginAccountVariantFields(
	payload map[string]json.RawMessage,
	typeName string,
	allowed ...string,
) error {
	for name := range payload {
		accepted := false
		for _, candidate := range allowed {
			if name == candidate {
				accepted = true
				break
			}
		}
		if !accepted {
			return fmt.Errorf("login account %s variant does not accept field %q", typeName, name)
		}
	}
	return nil
}

func decodeOptionalLoginAccountBool(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (bool, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return false, nil
	}
	if isJSONNull(raw) {
		return false, fmt.Errorf("%s %s must be a boolean", objectName, fieldName)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func decodeNullableLoginAccountValue[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return &value, nil
}

func marshalLoginAccountType(typeName string) ([]byte, error) {
	return json.Marshal(struct {
		Type string `json:"type"`
	}{Type: typeName})
}

func loginAccountDiscriminant(data json.RawMessage) string {
	var value struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	return value.Type
}

var (
	_ json.Marshaler   = LoginAccountParams{}
	_ json.Unmarshaler = (*LoginAccountParams)(nil)
	_ json.Marshaler   = LoginAccountResponse{}
	_ json.Unmarshaler = (*LoginAccountResponse)(nil)
)
