package protocol

import (
	"encoding/json"
	"errors"
)

// NetworkApprovalContext is the exact standalone public context for a network
// approval. It describes protocol data only and does not grant network access.
type NetworkApprovalContext struct {
	Host     string                  `json:"host"`
	Protocol NetworkApprovalProtocol `json:"protocol"`
}

func (c *NetworkApprovalContext) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode network approval context into nil receiver")
	}
	const objectName = "network approval context"
	payload, err := decodeRustSerdeObject(data, objectName, "host", "protocol")
	if err != nil {
		return err
	}
	host, err := decodeRequiredThreadItemValue[string](payload, objectName, "host")
	if err != nil {
		return err
	}
	protocol, err := decodeRequiredThreadItemValue[NetworkApprovalProtocol](payload, objectName, "protocol")
	if err != nil {
		return err
	}
	*c = NetworkApprovalContext{Host: host, Protocol: protocol}
	return nil
}

var _ json.Unmarshaler = (*NetworkApprovalContext)(nil)
