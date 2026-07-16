package protocol

import (
	"encoding/json"
	"errors"
)

// AppTemplateSummary is exact standalone descriptive app-template metadata.
// Template discovery, materialization, and plugin runtime behavior remain absent.
type AppTemplateSummary struct {
	TemplateID           string                        `json:"templateId"`
	Name                 string                        `json:"name"`
	Description          *string                       `json:"description,omitempty"`
	Category             *string                       `json:"category,omitempty"`
	CanonicalConnectorID *string                       `json:"canonicalConnectorId,omitempty"`
	LogoURL              *string                       `json:"logoUrl,omitempty"`
	LogoURLDark          *string                       `json:"logoUrlDark,omitempty"`
	MaterializedAppIDs   []string                      `json:"materializedAppIds"`
	Reason               *AppTemplateUnavailableReason `json:"reason,omitempty"`
}

func (s AppTemplateSummary) MarshalJSON() ([]byte, error) {
	if s.MaterializedAppIDs == nil {
		return nil, errors.New("marshal app template summary with nil materializedAppIds")
	}
	return json.Marshal(map[string]any{
		"templateId":           s.TemplateID,
		"name":                 s.Name,
		"description":          s.Description,
		"category":             s.Category,
		"canonicalConnectorId": s.CanonicalConnectorID,
		"logoUrl":              s.LogoURL,
		"logoUrlDark":          s.LogoURLDark,
		"materializedAppIds":   s.MaterializedAppIDs,
		"reason":               s.Reason,
	})
}

func (s *AppTemplateSummary) UnmarshalJSON(data []byte) error {
	if s == nil {
		return errors.New("decode app template summary into nil receiver")
	}
	const objectName = "app template summary"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"templateId", "name", "description", "category", "canonicalConnectorId",
		"logoUrl", "logoUrlDark", "materializedAppIds", "reason",
	)
	if err != nil {
		return err
	}
	templateID, err := decodeRequiredThreadItemValue[string](payload, objectName, "templateId")
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
	category, err := decodeOptionalNullableConfigValue[string](payload, objectName, "category")
	if err != nil {
		return err
	}
	canonicalConnectorID, err := decodeOptionalNullableConfigValue[string](
		payload, objectName, "canonicalConnectorId",
	)
	if err != nil {
		return err
	}
	logoURL, err := decodeOptionalNullableConfigValue[string](payload, objectName, "logoUrl")
	if err != nil {
		return err
	}
	logoURLDark, err := decodeOptionalNullableConfigValue[string](payload, objectName, "logoUrlDark")
	if err != nil {
		return err
	}
	materializedAppIDs, err := decodeRequiredThreadItemArray[string](payload, objectName, "materializedAppIds")
	if err != nil {
		return err
	}
	reason, err := decodeOptionalNullableConfigValue[AppTemplateUnavailableReason](payload, objectName, "reason")
	if err != nil {
		return err
	}
	*s = AppTemplateSummary{
		TemplateID: templateID, Name: name, Description: description, Category: category,
		CanonicalConnectorID: canonicalConnectorID, LogoURL: logoURL, LogoURLDark: logoURLDark,
		MaterializedAppIDs: materializedAppIDs, Reason: reason,
	}
	return nil
}

var (
	_ json.Marshaler   = AppTemplateSummary{}
	_ json.Unmarshaler = (*AppTemplateSummary)(nil)
)
