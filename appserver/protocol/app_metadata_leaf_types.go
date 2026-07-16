package protocol

import (
	"encoding/json"
	"errors"
)

// AppBranding is exact standalone public branding data for experimental apps.
// App discovery and runtime behavior remain deferred and unbound.
type AppBranding struct {
	Category          *string `json:"category,omitempty"`
	Developer         *string `json:"developer,omitempty"`
	Website           *string `json:"website,omitempty"`
	PrivacyPolicy     *string `json:"privacyPolicy,omitempty"`
	TermsOfService    *string `json:"termsOfService,omitempty"`
	IsDiscoverableApp bool    `json:"isDiscoverableApp"`
}

func (b AppBranding) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"category":          b.Category,
		"developer":         b.Developer,
		"website":           b.Website,
		"privacyPolicy":     b.PrivacyPolicy,
		"termsOfService":    b.TermsOfService,
		"isDiscoverableApp": b.IsDiscoverableApp,
	})
}

func (b *AppBranding) UnmarshalJSON(data []byte) error {
	if b == nil {
		return errors.New("decode app branding into nil receiver")
	}
	const objectName = "app branding"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"category", "developer", "website", "privacyPolicy", "termsOfService", "isDiscoverableApp",
	)
	if err != nil {
		return err
	}
	category, err := decodeOptionalNullableConfigValue[string](payload, objectName, "category")
	if err != nil {
		return err
	}
	developer, err := decodeOptionalNullableConfigValue[string](payload, objectName, "developer")
	if err != nil {
		return err
	}
	website, err := decodeOptionalNullableConfigValue[string](payload, objectName, "website")
	if err != nil {
		return err
	}
	privacyPolicy, err := decodeOptionalNullableConfigValue[string](payload, objectName, "privacyPolicy")
	if err != nil {
		return err
	}
	termsOfService, err := decodeOptionalNullableConfigValue[string](payload, objectName, "termsOfService")
	if err != nil {
		return err
	}
	isDiscoverableApp, err := decodeRequiredThreadItemValue[bool](payload, objectName, "isDiscoverableApp")
	if err != nil {
		return err
	}
	*b = AppBranding{
		Category: category, Developer: developer, Website: website,
		PrivacyPolicy: privacyPolicy, TermsOfService: termsOfService,
		IsDiscoverableApp: isDiscoverableApp,
	}
	return nil
}

// AppReview is an exact opaque app-review status value.
type AppReview struct {
	Status string `json:"status"`
}

func (r AppReview) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Status string `json:"status"`
	}{Status: r.Status})
}

func (r *AppReview) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode app review into nil receiver")
	}
	const objectName = "app review"
	payload, err := decodeRustSerdeObject(data, objectName, "status")
	if err != nil {
		return err
	}
	status, err := decodeRequiredThreadItemValue[string](payload, objectName, "status")
	if err != nil {
		return err
	}
	*r = AppReview{Status: status}
	return nil
}

// AppScreenshot is exact standalone public screenshot metadata. Gollem does
// not fetch or interpret the URL, file identifier, or user prompt.
type AppScreenshot struct {
	URL        *string `json:"url,omitempty"`
	FileID     *string `json:"fileId,omitempty"`
	UserPrompt string  `json:"userPrompt"`
}

func (s AppScreenshot) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"url":        s.URL,
		"fileId":     s.FileID,
		"userPrompt": s.UserPrompt,
	})
}

func (s *AppScreenshot) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode app screenshot into nil receiver")
	}
	const objectName = "app screenshot"
	payload, err := decodeRustSerdeObject(data, objectName, "url", "fileId", "userPrompt")
	if err != nil {
		return err
	}
	url, err := decodeOptionalNullableConfigValue[string](payload, objectName, "url")
	if err != nil {
		return err
	}
	fileID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "fileId")
	if err != nil {
		return err
	}
	userPrompt, err := decodeRequiredThreadItemValue[string](payload, objectName, "userPrompt")
	if err != nil {
		return err
	}
	*s = AppScreenshot{URL: url, FileID: fileID, UserPrompt: userPrompt}
	return nil
}

var (
	_ json.Marshaler   = AppBranding{}
	_ json.Unmarshaler = (*AppBranding)(nil)
	_ json.Marshaler   = AppReview{}
	_ json.Unmarshaler = (*AppReview)(nil)
	_ json.Marshaler   = AppScreenshot{}
	_ json.Unmarshaler = (*AppScreenshot)(nil)
)
