package protocol

import (
	"encoding/json"
	"errors"
)

// AppSummary is exact standalone descriptive app metadata for plugin results.
// Category derivation, app discovery, and plugin runtime behavior remain absent.
type AppSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	InstallURL  *string `json:"installUrl,omitempty"`
	Category    *string `json:"category,omitempty"`
}

func (s AppSummary) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"id":          s.ID,
		"name":        s.Name,
		"description": s.Description,
		"installUrl":  s.InstallURL,
		"category":    s.Category,
	})
}

func (s *AppSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode app summary into nil receiver")
	}
	const objectName = "app summary"
	payload, err := decodeRustSerdeObject(
		data, objectName, "id", "name", "description", "installUrl", "category",
	)
	if err != nil {
		return err
	}
	id, err := decodeRequiredThreadItemValue[string](payload, objectName, "id")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[string](payload, objectName, "name")
	if err != nil {
		return err
	}
	description, err := decodeOptionalNullableConfigValue[string](payload, objectName, "description")
	if err != nil {
		return err
	}
	installURL, err := decodeOptionalNullableConfigValue[string](payload, objectName, "installUrl")
	if err != nil {
		return err
	}
	category, err := decodeOptionalNullableConfigValue[string](payload, objectName, "category")
	if err != nil {
		return err
	}
	*s = AppSummary{
		ID: id, Name: name, Description: description, InstallURL: installURL, Category: category,
	}
	return nil
}

var (
	_ json.Marshaler   = AppSummary{}
	_ json.Unmarshaler = (*AppSummary)(nil)
)
