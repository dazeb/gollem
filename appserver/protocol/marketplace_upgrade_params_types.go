package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MarketplaceUpgradeParams is the exact standalone source contract for
// marketplace/upgrade. It intentionally does not upgrade configured marketplaces.
type MarketplaceUpgradeParams struct {
	MarketplaceName *string `json:"marketplaceName"`
}

func (p *MarketplaceUpgradeParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode marketplace upgrade params into nil receiver")
	}
	const objectName = "marketplace upgrade params"
	payload, err := decodeRustSerdeObject(data, objectName, "marketplaceName")
	if err != nil {
		return err
	}
	marketplaceName, err := decodeOptionalMarketplaceUpgradeValue[string](payload, objectName, "marketplaceName")
	if err != nil {
		return err
	}
	*p = MarketplaceUpgradeParams{MarketplaceName: marketplaceName}
	return nil
}

func decodeOptionalMarketplaceUpgradeValue[T any](
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

func marketplaceUpgradeParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"marketplaceName": Schema{"type": []any{"string", "null"}},
		},
		"type": "object",
	}
}

var _ json.Unmarshaler = (*MarketplaceUpgradeParams)(nil)
