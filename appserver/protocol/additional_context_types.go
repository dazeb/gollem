package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// AdditionalContextKind is the exact closed public context trust category.
type AdditionalContextKind string

const (
	AdditionalContextKindUntrusted   AdditionalContextKind = "untrusted"
	AdditionalContextKindApplication AdditionalContextKind = "application"
)

func (k AdditionalContextKind) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(k, "additional context kind", AdditionalContextKind.valid)
}

func (k *AdditionalContextKind) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, k, "additional context kind", AdditionalContextKind.valid)
}

func (k AdditionalContextKind) valid() bool {
	return k == AdditionalContextKindUntrusted || k == AdditionalContextKindApplication
}

// AdditionalContextEntry is an exact standalone public context fragment. Its
// decoder ignores unknown fields for Rust-compatible input evolution while
// canonical output contains only the two public fields.
type AdditionalContextEntry struct {
	Value string                `json:"value"`
	Kind  AdditionalContextKind `json:"kind"`
}

func (e *AdditionalContextEntry) UnmarshalJSON(data []byte) error {
	if e == nil {
		return errors.New("decode additional context entry into nil receiver")
	}
	payload, err := decodeAdditionalContextEntryObject(data)
	if err != nil {
		return err
	}
	value, err := decodeRequiredThreadItemValue[string](payload, "additional context entry", "value")
	if err != nil {
		return err
	}
	kind, err := decodeRequiredThreadItemValue[AdditionalContextKind](payload, "additional context entry", "kind")
	if err != nil {
		return err
	}
	*e = AdditionalContextEntry{Value: value, Kind: kind}
	return nil
}

func decodeAdditionalContextEntryObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode additional context entry: %w", err)
	}
	if opening != json.Delim('{') {
		return nil, errors.New("additional context entry must be an object")
	}
	payload := make(map[string]json.RawMessage, 2)
	seen := make(map[string]bool, 2)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode additional context entry field name: %w", err)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode additional context entry field %q: %w", name, err)
		}
		if name != "value" && name != "kind" {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate additional context entry field %q", name)
		}
		seen[name] = true
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode additional context entry: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("additional context entry must contain one JSON value")
		}
		return nil, fmt.Errorf("decode additional context entry trailing value: %w", err)
	}
	return payload, nil
}

var (
	_ json.Marshaler   = AdditionalContextKind("")
	_ json.Unmarshaler = (*AdditionalContextKind)(nil)
	_ json.Unmarshaler = (*AdditionalContextEntry)(nil)
)
