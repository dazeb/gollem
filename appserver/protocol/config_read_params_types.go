package protocol

import (
	"encoding/json"
	"errors"
)

const configReadCWDDescription = "Optional working directory to resolve project config layers. If specified, return the effective config as seen from that directory (i.e., including any project layers between `cwd` and the project/repo root)."

// ConfigReadParams is the exact standalone public config-read request.
// Gollem's live key-filtered request remains a separate contract.
type ConfigReadParams struct {
	IncludeLayers bool    `json:"includeLayers,omitempty"`
	CWD           *string `json:"cwd,omitempty"`
}

func (p ConfigReadParams) MarshalJSON() ([]byte, error) {
	type wire struct {
		IncludeLayers bool    `json:"includeLayers,omitempty"`
		CWD           *string `json:"cwd"`
	}
	return json.Marshal(wire(p))
}

func (p *ConfigReadParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode config-read params into nil receiver")
	}
	const objectName = "config-read params"
	payload, err := decodeExactThreadItemObject(data, objectName, "includeLayers", "cwd")
	if err != nil {
		return err
	}
	includeLayers, err := decodeOptionalConfigBool(payload, objectName, "includeLayers")
	if err != nil {
		return err
	}
	cwd, err := decodeOptionalNullableConfigValue[string](payload, objectName, "cwd")
	if err != nil {
		return err
	}
	*p = ConfigReadParams{IncludeLayers: includeLayers, CWD: cwd}
	return nil
}

var (
	_ json.Marshaler   = ConfigReadParams{}
	_ json.Unmarshaler = (*ConfigReadParams)(nil)
)
