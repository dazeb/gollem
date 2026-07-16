package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AttestationGenerateParams is the exact empty public attestation request. It
// remains standalone until Slang owns an attestation provider and threat model.
type AttestationGenerateParams struct{}

func (p *AttestationGenerateParams) UnmarshalJSON(data []byte) error {
	if p == nil {
		return errors.New("decode attestation generation params into nil receiver")
	}

	// Serde ignores unknown fields for this empty struct. Decode through a map
	// to preserve that input compatibility while still requiring one object.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("decode attestation generation params: %w", err)
	}
	if object == nil {
		return errors.New("attestation generation params must be an object")
	}

	*p = AttestationGenerateParams{}
	return nil
}

// AttestationGenerateResponse is the exact public opaque-token response. It
// describes wire data only and does not generate, validate, or trust a token.
type AttestationGenerateResponse struct {
	Token string `json:"token" jsonschema:"description=Opaque client attestation token."`
}

func (r *AttestationGenerateResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode attestation generation response into nil receiver")
	}
	const objectName = "attestation generation response"
	payload, err := decodeRustSerdeObject(data, objectName, "token")
	if err != nil {
		return err
	}
	token, err := decodeRequiredThreadItemValue[string](payload, objectName, "token")
	if err != nil {
		return err
	}
	*r = AttestationGenerateResponse{Token: token}
	return nil
}

var (
	_ json.Unmarshaler = (*AttestationGenerateParams)(nil)
	_ json.Unmarshaler = (*AttestationGenerateResponse)(nil)
)
