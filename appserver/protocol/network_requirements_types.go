package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// NetworkDomainPermission is the exact closed public domain permission.
type NetworkDomainPermission string

const (
	NetworkDomainPermissionAllow NetworkDomainPermission = "allow"
	NetworkDomainPermissionDeny  NetworkDomainPermission = "deny"
)

func (p NetworkDomainPermission) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(p, "network domain permission", NetworkDomainPermission.valid)
}

func (p *NetworkDomainPermission) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, p, "network domain permission", NetworkDomainPermission.valid)
}

func (p NetworkDomainPermission) valid() bool {
	return p == NetworkDomainPermissionAllow || p == NetworkDomainPermissionDeny
}

// NetworkUnixSocketPermission is the exact closed public Unix-socket permission.
type NetworkUnixSocketPermission string

const (
	NetworkUnixSocketPermissionAllow NetworkUnixSocketPermission = "allow"
	NetworkUnixSocketPermissionDeny  NetworkUnixSocketPermission = "deny"
)

func (p NetworkUnixSocketPermission) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(p, "network Unix-socket permission", NetworkUnixSocketPermission.valid)
}

func (p *NetworkUnixSocketPermission) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(
		data,
		p,
		"network Unix-socket permission",
		NetworkUnixSocketPermission.valid,
	)
}

func (p NetworkUnixSocketPermission) valid() bool {
	return p == NetworkUnixSocketPermissionAllow || p == NetworkUnixSocketPermissionDeny
}

// NetworkRequirements is the exact standalone public network-requirements
// value. It does not configure a proxy or imply runtime network enforcement.
type NetworkRequirements struct {
	Enabled                          *bool                                   `json:"enabled"`
	HTTPPort                         *uint16                                 `json:"httpPort"`
	SOCKSPort                        *uint16                                 `json:"socksPort"`
	AllowUpstreamProxy               *bool                                   `json:"allowUpstreamProxy"`
	DangerouslyAllowNonLoopbackProxy *bool                                   `json:"dangerouslyAllowNonLoopbackProxy"`
	DangerouslyAllowAllUnixSockets   *bool                                   `json:"dangerouslyAllowAllUnixSockets"`
	Domains                          *map[string]NetworkDomainPermission     `json:"domains"`
	ManagedAllowedDomainsOnly        *bool                                   `json:"managedAllowedDomainsOnly"`
	AllowedDomains                   *[]string                               `json:"allowedDomains"`
	DeniedDomains                    *[]string                               `json:"deniedDomains"`
	UnixSockets                      *map[string]NetworkUnixSocketPermission `json:"unixSockets"`
	AllowUnixSockets                 *[]string                               `json:"allowUnixSockets"`
	AllowLocalBinding                *bool                                   `json:"allowLocalBinding"`
}

func (r *NetworkRequirements) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode network requirements into nil receiver")
	}
	const objectName = "network requirements"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"enabled",
		"httpPort",
		"socksPort",
		"allowUpstreamProxy",
		"dangerouslyAllowNonLoopbackProxy",
		"dangerouslyAllowAllUnixSockets",
		"domains",
		"managedAllowedDomainsOnly",
		"allowedDomains",
		"deniedDomains",
		"unixSockets",
		"allowUnixSockets",
		"allowLocalBinding",
	)
	if err != nil {
		return err
	}
	enabled, err := decodeOptionalNullableConfigRequirementValue[bool](payload, objectName, "enabled")
	if err != nil {
		return err
	}
	httpPort, err := decodeOptionalNullableConfigRequirementValue[uint16](payload, objectName, "httpPort")
	if err != nil {
		return err
	}
	socksPort, err := decodeOptionalNullableConfigRequirementValue[uint16](payload, objectName, "socksPort")
	if err != nil {
		return err
	}
	allowUpstreamProxy, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "allowUpstreamProxy",
	)
	if err != nil {
		return err
	}
	dangerouslyAllowNonLoopbackProxy, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "dangerouslyAllowNonLoopbackProxy",
	)
	if err != nil {
		return err
	}
	dangerouslyAllowAllUnixSockets, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "dangerouslyAllowAllUnixSockets",
	)
	if err != nil {
		return err
	}
	domains, err := decodeOptionalNullableNetworkRequirementMap[NetworkDomainPermission](
		payload, objectName, "domains",
	)
	if err != nil {
		return err
	}
	managedAllowedDomainsOnly, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "managedAllowedDomainsOnly",
	)
	if err != nil {
		return err
	}
	allowedDomains, err := decodeOptionalNullableConfigRequirementArray[string](
		payload, objectName, "allowedDomains",
	)
	if err != nil {
		return err
	}
	deniedDomains, err := decodeOptionalNullableConfigRequirementArray[string](
		payload, objectName, "deniedDomains",
	)
	if err != nil {
		return err
	}
	unixSockets, err := decodeOptionalNullableNetworkRequirementMap[NetworkUnixSocketPermission](
		payload, objectName, "unixSockets",
	)
	if err != nil {
		return err
	}
	allowUnixSockets, err := decodeOptionalNullableConfigRequirementArray[string](
		payload, objectName, "allowUnixSockets",
	)
	if err != nil {
		return err
	}
	allowLocalBinding, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "allowLocalBinding",
	)
	if err != nil {
		return err
	}
	*r = NetworkRequirements{
		Enabled:                          enabled,
		HTTPPort:                         httpPort,
		SOCKSPort:                        socksPort,
		AllowUpstreamProxy:               allowUpstreamProxy,
		DangerouslyAllowNonLoopbackProxy: dangerouslyAllowNonLoopbackProxy,
		DangerouslyAllowAllUnixSockets:   dangerouslyAllowAllUnixSockets,
		Domains:                          domains,
		ManagedAllowedDomainsOnly:        managedAllowedDomainsOnly,
		AllowedDomains:                   allowedDomains,
		DeniedDomains:                    deniedDomains,
		UnixSockets:                      unixSockets,
		AllowUnixSockets:                 allowUnixSockets,
		AllowLocalBinding:                allowLocalBinding,
	}
	return nil
}

func decodeOptionalNullableNetworkRequirementMap[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*map[string]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make(map[string]T, len(entries))
	for name, encoded := range entries {
		if isJSONNull(encoded) {
			return nil, fmt.Errorf("decode %s %s[%q]: value cannot be null", objectName, fieldName, name)
		}
		var value T
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("decode %s %s[%q]: %w", objectName, fieldName, name, err)
		}
		values[name] = value
	}
	return &values, nil
}

var (
	_ json.Marshaler   = NetworkDomainPermission("")
	_ json.Unmarshaler = (*NetworkDomainPermission)(nil)
	_ json.Marshaler   = NetworkUnixSocketPermission("")
	_ json.Unmarshaler = (*NetworkUnixSocketPermission)(nil)
	_ json.Unmarshaler = (*NetworkRequirements)(nil)
)
