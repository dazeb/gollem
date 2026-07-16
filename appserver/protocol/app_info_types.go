package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AppInfo is exact standalone metadata for an experimental app.
// App discovery, policy interpretation, and runtime behavior remain deferred.
type AppInfo struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Description         *string            `json:"description,omitempty"`
	LogoURL             *string            `json:"logoUrl,omitempty"`
	LogoURLDark         *string            `json:"logoUrlDark,omitempty"`
	IconAssets          *map[string]string `json:"iconAssets,omitempty"`
	IconDarkAssets      *map[string]string `json:"iconDarkAssets,omitempty"`
	DistributionChannel *string            `json:"distributionChannel,omitempty"`
	Branding            *AppBranding       `json:"branding,omitempty"`
	AppMetadata         *AppMetadata       `json:"appMetadata,omitempty"`
	Labels              *map[string]string `json:"labels,omitempty"`
	InstallURL          *string            `json:"installUrl,omitempty"`
	IsAccessible        bool               `json:"isAccessible"`
	IsEnabled           bool               `json:"isEnabled"`
	PluginDisplayNames  []string           `json:"pluginDisplayNames"`
}

func (i AppInfo) MarshalJSON() ([]byte, error) {
	pluginDisplayNames := i.PluginDisplayNames
	if pluginDisplayNames == nil {
		pluginDisplayNames = []string{}
	}
	return json.Marshal(map[string]any{
		"id":                  i.ID,
		"name":                i.Name,
		"description":         i.Description,
		"logoUrl":             i.LogoURL,
		"logoUrlDark":         i.LogoURLDark,
		"iconAssets":          i.IconAssets,
		"iconDarkAssets":      i.IconDarkAssets,
		"distributionChannel": i.DistributionChannel,
		"branding":            i.Branding,
		"appMetadata":         i.AppMetadata,
		"labels":              i.Labels,
		"installUrl":          i.InstallURL,
		"isAccessible":        i.IsAccessible,
		"isEnabled":           i.IsEnabled,
		"pluginDisplayNames":  pluginDisplayNames,
	})
}

func (i *AppInfo) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode app info into nil receiver")
	}
	const objectName = "app info"
	payload, err := decodeRustSerdeObject(
		data, objectName,
		"id", "name", "description", "logoUrl", "logoUrlDark", "iconAssets",
		"iconDarkAssets", "distributionChannel", "branding", "appMetadata", "labels",
		"installUrl", "isAccessible", "isEnabled", "pluginDisplayNames",
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
	logoURL, err := decodeOptionalNullableConfigValue[string](payload, objectName, "logoUrl")
	if err != nil {
		return err
	}
	logoURLDark, err := decodeOptionalNullableConfigValue[string](payload, objectName, "logoUrlDark")
	if err != nil {
		return err
	}
	iconAssets, err := decodeOptionalNullableAppInfoStringMap(payload, objectName, "iconAssets")
	if err != nil {
		return err
	}
	iconDarkAssets, err := decodeOptionalNullableAppInfoStringMap(payload, objectName, "iconDarkAssets")
	if err != nil {
		return err
	}
	distributionChannel, err := decodeOptionalNullableConfigValue[string](
		payload, objectName, "distributionChannel",
	)
	if err != nil {
		return err
	}
	branding, err := decodeOptionalNullableConfigValue[AppBranding](payload, objectName, "branding")
	if err != nil {
		return err
	}
	appMetadata, err := decodeOptionalNullableConfigValue[AppMetadata](payload, objectName, "appMetadata")
	if err != nil {
		return err
	}
	labels, err := decodeOptionalNullableAppInfoStringMap(payload, objectName, "labels")
	if err != nil {
		return err
	}
	installURL, err := decodeOptionalNullableConfigValue[string](payload, objectName, "installUrl")
	if err != nil {
		return err
	}
	isAccessible, err := decodeOptionalConfigBool(payload, objectName, "isAccessible")
	if err != nil {
		return err
	}
	isEnabled, err := decodeDefaultEnabledAppInfoBool(payload, objectName, "isEnabled")
	if err != nil {
		return err
	}
	pluginDisplayNames, err := decodeDefaultedAppInfoStringArray(
		payload, objectName, "pluginDisplayNames",
	)
	if err != nil {
		return err
	}
	*i = AppInfo{
		ID: id, Name: name, Description: description, LogoURL: logoURL, LogoURLDark: logoURLDark,
		IconAssets: iconAssets, IconDarkAssets: iconDarkAssets,
		DistributionChannel: distributionChannel, Branding: branding, AppMetadata: appMetadata,
		Labels: labels, InstallURL: installURL, IsAccessible: isAccessible, IsEnabled: isEnabled,
		PluginDisplayNames: pluginDisplayNames,
	}
	return nil
}

func decodeOptionalNullableAppInfoStringMap(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*map[string]string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make(map[string]string, len(fields))
	for key, value := range fields {
		if isJSONNull(value) {
			return nil, fmt.Errorf("decode %s %s[%q]: value cannot be null", objectName, fieldName, key)
		}
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, fmt.Errorf("decode %s %s[%q]: %w", objectName, fieldName, key, err)
		}
		values[key] = decoded
	}
	return &values, nil
}

func decodeDefaultEnabledAppInfoBool(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (bool, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return true, nil
	}
	if isJSONNull(raw) {
		return false, fmt.Errorf("decode %s %s: value cannot be null", objectName, fieldName)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	return value, nil
}

func decodeDefaultedAppInfoStringArray(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) ([]string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return []string{}, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("decode %s %s: value cannot be null", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]string, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return values, nil
}

var (
	_ json.Marshaler   = AppInfo{}
	_ json.Unmarshaler = (*AppInfo)(nil)
)
