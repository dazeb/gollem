package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ConfigReadResponse is the exact standalone public effective-config response.
// It is kept separate from Gollem's memory-backed config/read result.
type ConfigReadResponse struct {
	Config  Config                         `json:"config"`
	Origins map[string]ConfigLayerMetadata `json:"origins"`
	Layers  []ConfigLayer                  `json:"layers"`
}

func (r ConfigReadResponse) MarshalJSON() ([]byte, error) {
	if r.Origins == nil {
		return nil, errors.New("ConfigReadResponse origins must be a non-null object")
	}
	type wireConfigReadResponse struct {
		Config  Config                         `json:"config"`
		Origins map[string]ConfigLayerMetadata `json:"origins"`
		Layers  []ConfigLayer                  `json:"layers"`
	}
	return json.Marshal(wireConfigReadResponse(r))
}

func (r *ConfigReadResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode ConfigReadResponse into nil receiver")
	}
	payload, err := decodeConfigReadResponseObject(data)
	if err != nil {
		return err
	}
	config, err := decodeRequiredThreadItemValue[Config](payload, "ConfigReadResponse", "config")
	if err != nil {
		return err
	}
	origins, err := decodeConfigReadResponseOrigins(payload)
	if err != nil {
		return err
	}
	layers, err := decodeConfigReadResponseLayers(payload)
	if err != nil {
		return err
	}
	*r = ConfigReadResponse{Config: config, Origins: origins, Layers: layers}
	return nil
}

func decodeConfigReadResponseObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode ConfigReadResponse: %w", err)
	}
	if opening != json.Delim('{') {
		return nil, errors.New("ConfigReadResponse must be an object")
	}
	payload := make(map[string]json.RawMessage, 3)
	seen := make(map[string]bool, 3)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode ConfigReadResponse field name: %w", err)
		}
		name := token.(string)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode ConfigReadResponse field %q: %w", name, err)
		}
		if !isConfigReadResponseKnownField(name) {
			continue
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate ConfigReadResponse field %q", name)
		}
		seen[name] = true
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("decode ConfigReadResponse: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("ConfigReadResponse must contain one JSON value")
		}
		return nil, fmt.Errorf("decode ConfigReadResponse trailing value: %w", err)
	}
	return payload, nil
}

func decodeConfigReadResponseOrigins(
	payload map[string]json.RawMessage,
) (map[string]ConfigLayerMetadata, error) {
	raw, ok := payload["origins"]
	if !ok {
		return nil, errors.New("ConfigReadResponse requires origins")
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("ConfigReadResponse origins must be a non-null object")
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode ConfigReadResponse origins: %w", err)
	}
	origins := make(map[string]ConfigLayerMetadata, len(values))
	for name, encoded := range values {
		if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return nil, fmt.Errorf("ConfigReadResponse origin %q must be non-null", name)
		}
		var metadata ConfigLayerMetadata
		if err := json.Unmarshal(encoded, &metadata); err != nil {
			return nil, fmt.Errorf("decode ConfigReadResponse origin %q: %w", name, err)
		}
		origins[name] = metadata
	}
	return origins, nil
}

func decodeConfigReadResponseLayers(payload map[string]json.RawMessage) ([]ConfigLayer, error) {
	raw, ok := payload["layers"]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode ConfigReadResponse layers: %w", err)
	}
	layers := make([]ConfigLayer, len(values))
	for index, encoded := range values {
		if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return nil, fmt.Errorf("ConfigReadResponse layer %d must be non-null", index)
		}
		if err := json.Unmarshal(encoded, &layers[index]); err != nil {
			return nil, fmt.Errorf("decode ConfigReadResponse layer %d: %w", index, err)
		}
	}
	return layers, nil
}

func isConfigReadResponseKnownField(name string) bool {
	switch name {
	case "config", "origins", "layers":
		return true
	default:
		return false
	}
}

var (
	_ json.Marshaler   = ConfigReadResponse{}
	_ json.Unmarshaler = (*ConfigReadResponse)(nil)
)
