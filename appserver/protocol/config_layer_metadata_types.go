package protocol

import (
	"encoding/json"
	"errors"
)

// ConfigLayerMetadata identifies one versioned public configuration layer.
type ConfigLayerMetadata struct {
	Name    ConfigLayerSource `json:"name"`
	Version string            `json:"version"`
}

func (m *ConfigLayerMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode config layer metadata into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(data, "config layer metadata", "name", "version")
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[ConfigLayerSource](payload, "config layer metadata", "name")
	if err != nil {
		return err
	}
	version, err := decodeRequiredThreadItemValue[string](payload, "config layer metadata", "version")
	if err != nil {
		return err
	}
	*m = ConfigLayerMetadata{Name: name, Version: version}
	return nil
}

// ConfigLayer is one exact public configuration layer and its JSON value.
type ConfigLayer struct {
	Name           ConfigLayerSource `json:"name"`
	Version        string            `json:"version"`
	Config         JsonValue         `json:"config"`
	DisabledReason *string           `json:"disabledReason"`
}

func (l *ConfigLayer) UnmarshalJSON(data []byte) error {
	if l == nil {
		return errors.New("decode config layer into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"config layer",
		"name",
		"version",
		"config",
		"disabledReason",
	)
	if err != nil {
		return err
	}
	name, err := decodeRequiredThreadItemValue[ConfigLayerSource](payload, "config layer", "name")
	if err != nil {
		return err
	}
	version, err := decodeRequiredThreadItemValue[string](payload, "config layer", "version")
	if err != nil {
		return err
	}
	config, err := decodeRequiredThreadItemJSONValue(payload, "config layer", "config")
	if err != nil {
		return err
	}
	disabledReason, err := decodeOptionalNullableConfigRequirementValue[string](
		payload,
		"config layer",
		"disabledReason",
	)
	if err != nil {
		return err
	}
	*l = ConfigLayer{
		Name:           name,
		Version:        version,
		Config:         config,
		DisabledReason: disabledReason,
	}
	return nil
}

// OverriddenMetadata describes the layer and value that overrides a setting.
type OverriddenMetadata struct {
	Message         string              `json:"message"`
	OverridingLayer ConfigLayerMetadata `json:"overridingLayer"`
	EffectiveValue  JsonValue           `json:"effectiveValue"`
}

func (m *OverriddenMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return errors.New("decode overridden metadata into nil receiver")
	}
	payload, err := decodeExactThreadItemObject(
		data,
		"overridden metadata",
		"message",
		"overridingLayer",
		"effectiveValue",
	)
	if err != nil {
		return err
	}
	message, err := decodeRequiredThreadItemValue[string](payload, "overridden metadata", "message")
	if err != nil {
		return err
	}
	overridingLayer, err := decodeRequiredThreadItemValue[ConfigLayerMetadata](
		payload,
		"overridden metadata",
		"overridingLayer",
	)
	if err != nil {
		return err
	}
	effectiveValue, err := decodeRequiredThreadItemJSONValue(payload, "overridden metadata", "effectiveValue")
	if err != nil {
		return err
	}
	*m = OverriddenMetadata{
		Message:         message,
		OverridingLayer: overridingLayer,
		EffectiveValue:  effectiveValue,
	}
	return nil
}

var (
	_ json.Unmarshaler = (*ConfigLayerMetadata)(nil)
	_ json.Unmarshaler = (*ConfigLayer)(nil)
	_ json.Unmarshaler = (*OverriddenMetadata)(nil)
)
