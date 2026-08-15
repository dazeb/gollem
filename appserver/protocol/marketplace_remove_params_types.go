package protocol

import (
	"encoding/json"
	"errors"
)

// MarketplaceRemoveParams is the exact standalone source contract for
// marketplace/remove. It intentionally does not remove configured marketplaces.
type MarketplaceRemoveParams struct {
	MarketplaceName string `json:"marketplaceName"`
}

func (p *MarketplaceRemoveParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode marketplace remove params into nil receiver")
	}
	const objectName = "marketplace remove params"
	payload, err := decodeRustSerdeObject(data, objectName, "marketplaceName")
	if err != nil {
		return err
	}
	marketplaceName, err := decodeRequiredThreadItemValue[string](payload, objectName, "marketplaceName")
	if err != nil {
		return err
	}
	*p = MarketplaceRemoveParams{MarketplaceName: marketplaceName}
	return nil
}

func marketplaceRemoveParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"marketplaceName": Schema{"type": "string"},
		},
		"required": []string{"marketplaceName"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*MarketplaceRemoveParams)(nil)
