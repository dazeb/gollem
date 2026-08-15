package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MarketplaceAddParams is the exact standalone source contract for
// marketplace/add. It intentionally does not add configured marketplaces.
type MarketplaceAddParams struct {
	Source      string    `json:"source"`
	RefName     *string   `json:"refName"`
	SparsePaths *[]string `json:"sparsePaths"`
}

func (p *MarketplaceAddParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode marketplace add params into nil receiver")
	}
	const objectName = "marketplace add params"
	payload, err := decodeRustSerdeObject(data, objectName, "source", "refName", "sparsePaths")
	if err != nil {
		return err
	}
	source, err := decodeRequiredThreadItemValue[string](payload, objectName, "source")
	if err != nil {
		return err
	}
	refName, err := decodeOptionalMarketplaceAddValue[string](payload, objectName, "refName")
	if err != nil {
		return err
	}
	sparsePaths, err := decodeOptionalMarketplaceAddStringList(payload, objectName, "sparsePaths")
	if err != nil {
		return err
	}
	*p = MarketplaceAddParams{Source: source, RefName: refName, SparsePaths: sparsePaths}
	return nil
}

func decodeOptionalMarketplaceAddValue[T any](
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

func decodeOptionalMarketplaceAddStringList(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]string, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]string, len(entries))
	for index, entry := range entries {
		if isJSONNull(entry) {
			return nil, fmt.Errorf("decode %s %s[%d]: must be a string", objectName, fieldName, index)
		}
		if err := json.Unmarshal(entry, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}

func marketplaceAddParamSchema() Schema {
	return Schema{
		"properties": Schema{
			"refName": Schema{"type": []any{"string", "null"}},
			"source":  Schema{"type": "string"},
			"sparsePaths": Schema{
				"items": Schema{"type": "string"},
				"type":  []any{"array", "null"},
			},
		},
		"required": []string{"source"},
		"type":     "object",
	}
}

var _ json.Unmarshaler = (*MarketplaceAddParams)(nil)
