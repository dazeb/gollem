package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AppMetadata is exact standalone descriptive metadata for experimental apps.
// App discovery, policy interpretation, and runtime behavior remain deferred.
type AppMetadata struct {
	Review                     *AppReview       `json:"review,omitempty"`
	Categories                 *[]string        `json:"categories,omitempty"`
	SubCategories              *[]string        `json:"subCategories,omitempty"`
	SEODescription             *string          `json:"seoDescription,omitempty"`
	Screenshots                *[]AppScreenshot `json:"screenshots,omitempty"`
	Developer                  *string          `json:"developer,omitempty"`
	Version                    *string          `json:"version,omitempty"`
	VersionID                  *string          `json:"versionId,omitempty"`
	VersionNotes               *string          `json:"versionNotes,omitempty"`
	FirstPartyType             *string          `json:"firstPartyType,omitempty"`
	FirstPartyRequiresInstall  *bool            `json:"firstPartyRequiresInstall,omitempty"`
	ShowInComposerWhenUnlinked *bool            `json:"showInComposerWhenUnlinked,omitempty"`
}

func (m AppMetadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"review":                     m.Review,
		"categories":                 m.Categories,
		"subCategories":              m.SubCategories,
		"seoDescription":             m.SEODescription,
		"screenshots":                m.Screenshots,
		"developer":                  m.Developer,
		"version":                    m.Version,
		"versionId":                  m.VersionID,
		"versionNotes":               m.VersionNotes,
		"firstPartyType":             m.FirstPartyType,
		"firstPartyRequiresInstall":  m.FirstPartyRequiresInstall,
		"showInComposerWhenUnlinked": m.ShowInComposerWhenUnlinked,
	})
}

func (m *AppMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode app metadata into nil receiver")
	}
	const objectName = "app metadata"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"review", "categories", "subCategories", "seoDescription", "screenshots",
		"developer", "version", "versionId", "versionNotes", "firstPartyType",
		"firstPartyRequiresInstall", "showInComposerWhenUnlinked",
	)
	if err != nil {
		return err
	}
	review, err := decodeOptionalNullableConfigValue[AppReview](payload, objectName, "review")
	if err != nil {
		return err
	}
	categories, err := decodeOptionalNullableAppMetadataArray[string](payload, objectName, "categories")
	if err != nil {
		return err
	}
	subCategories, err := decodeOptionalNullableAppMetadataArray[string](payload, objectName, "subCategories")
	if err != nil {
		return err
	}
	seoDescription, err := decodeOptionalNullableConfigValue[string](payload, objectName, "seoDescription")
	if err != nil {
		return err
	}
	screenshots, err := decodeOptionalNullableAppMetadataArray[AppScreenshot](payload, objectName, "screenshots")
	if err != nil {
		return err
	}
	developer, err := decodeOptionalNullableConfigValue[string](payload, objectName, "developer")
	if err != nil {
		return err
	}
	version, err := decodeOptionalNullableConfigValue[string](payload, objectName, "version")
	if err != nil {
		return err
	}
	versionID, err := decodeOptionalNullableConfigValue[string](payload, objectName, "versionId")
	if err != nil {
		return err
	}
	versionNotes, err := decodeOptionalNullableConfigValue[string](payload, objectName, "versionNotes")
	if err != nil {
		return err
	}
	firstPartyType, err := decodeOptionalNullableConfigValue[string](payload, objectName, "firstPartyType")
	if err != nil {
		return err
	}
	firstPartyRequiresInstall, err := decodeOptionalNullableConfigValue[bool](
		payload, objectName, "firstPartyRequiresInstall",
	)
	if err != nil {
		return err
	}
	showInComposerWhenUnlinked, err := decodeOptionalNullableConfigValue[bool](
		payload, objectName, "showInComposerWhenUnlinked",
	)
	if err != nil {
		return err
	}
	*m = AppMetadata{
		Review: review, Categories: categories, SubCategories: subCategories,
		SEODescription: seoDescription, Screenshots: screenshots, Developer: developer,
		Version: version, VersionID: versionID, VersionNotes: versionNotes,
		FirstPartyType: firstPartyType, FirstPartyRequiresInstall: firstPartyRequiresInstall,
		ShowInComposerWhenUnlinked: showInComposerWhenUnlinked,
	}
	return nil
}

func decodeOptionalNullableAppMetadataArray[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}

var (
	_ json.Marshaler   = AppMetadata{}
	_ json.Unmarshaler = (*AppMetadata)(nil)
)
