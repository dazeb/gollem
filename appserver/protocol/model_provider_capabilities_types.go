package protocol

import (
	"encoding/json"
	"errors"
)

// ModelProviderCapabilitiesReadParams is the exact empty public capability
// request. It stays separate from Gollem's provider-selector live extension.
type ModelProviderCapabilitiesReadParams struct{}

func (p *ModelProviderCapabilitiesReadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode model-provider capabilities params into nil receiver")
	}
	if _, err := decodeExactThreadItemObject(data, "model-provider capabilities params"); err != nil {
		return err
	}
	*p = ModelProviderCapabilitiesReadParams{}
	return nil
}

// ModelProviderCapabilitiesReadResponse is the exact public capability
// projection. Gollem's broader live response remains unbound.
type ModelProviderCapabilitiesReadResponse struct {
	NamespaceTools  bool `json:"namespaceTools"`
	ImageGeneration bool `json:"imageGeneration"`
	WebSearch       bool `json:"webSearch"`
}

func (r *ModelProviderCapabilitiesReadResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode model-provider capabilities response into nil receiver")
	}
	const objectName = "model-provider capabilities response"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"namespaceTools",
		"imageGeneration",
		"webSearch",
	)
	if err != nil {
		return err
	}
	namespaceTools, err := decodeRequiredThreadItemValue[bool](payload, objectName, "namespaceTools")
	if err != nil {
		return err
	}
	imageGeneration, err := decodeRequiredThreadItemValue[bool](payload, objectName, "imageGeneration")
	if err != nil {
		return err
	}
	webSearch, err := decodeRequiredThreadItemValue[bool](payload, objectName, "webSearch")
	if err != nil {
		return err
	}
	*r = ModelProviderCapabilitiesReadResponse{
		NamespaceTools:  namespaceTools,
		ImageGeneration: imageGeneration,
		WebSearch:       webSearch,
	}
	return nil
}

var (
	_ json.Unmarshaler = (*ModelProviderCapabilitiesReadParams)(nil)
	_ json.Unmarshaler = (*ModelProviderCapabilitiesReadResponse)(nil)
)
