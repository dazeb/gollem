package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// ComputerUseRequirements is the exact public computer-use requirement.
// Nullable Rust options remain explicit in canonical output.
type ComputerUseRequirements struct {
	AllowLockedComputerUse *bool `json:"allowLockedComputerUse"`
}

func (r *ComputerUseRequirements) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode computer-use requirements into nil receiver")
	}
	const objectName = "computer-use requirements"
	payload, err := decodeExactThreadItemObject(data, objectName, "allowLockedComputerUse")
	if err != nil {
		return err
	}
	allowLockedComputerUse, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload,
		objectName,
		"allowLockedComputerUse",
	)
	if err != nil {
		return err
	}
	*r = ComputerUseRequirements{AllowLockedComputerUse: allowLockedComputerUse}
	return nil
}

// ResidencyRequirement is the exact closed public residency requirement.
type ResidencyRequirement string

const ResidencyRequirementUS ResidencyRequirement = "us"

func (r ResidencyRequirement) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(r, "residency requirement", ResidencyRequirement.valid)
}

func (r *ResidencyRequirement) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, r, "residency requirement", ResidencyRequirement.valid)
}

func (r ResidencyRequirement) valid() bool { return r == ResidencyRequirementUS }

// WebSearchMode is the exact closed public web-search requirement mode.
type WebSearchMode string

const (
	WebSearchModeDisabled WebSearchMode = "disabled"
	WebSearchModeCached   WebSearchMode = "cached"
	WebSearchModeIndexed  WebSearchMode = "indexed"
	WebSearchModeLive     WebSearchMode = "live"
)

func (m WebSearchMode) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "web-search mode", WebSearchMode.valid)
}

func (m *WebSearchMode) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "web-search mode", WebSearchMode.valid)
}

func (m WebSearchMode) valid() bool {
	switch m {
	case WebSearchModeDisabled, WebSearchModeCached, WebSearchModeIndexed, WebSearchModeLive:
		return true
	default:
		return false
	}
}

// WindowsSandboxSetupMode is the exact closed public Windows setup mode. It
// does not add a Windows sandbox implementation to Gollem.
type WindowsSandboxSetupMode string

const (
	WindowsSandboxSetupModeElevated   WindowsSandboxSetupMode = "elevated"
	WindowsSandboxSetupModeUnelevated WindowsSandboxSetupMode = "unelevated"
)

func (m WindowsSandboxSetupMode) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "Windows sandbox setup mode", WindowsSandboxSetupMode.valid)
}

func (m *WindowsSandboxSetupMode) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "Windows sandbox setup mode", WindowsSandboxSetupMode.valid)
}

func (m WindowsSandboxSetupMode) valid() bool {
	return m == WindowsSandboxSetupModeElevated || m == WindowsSandboxSetupModeUnelevated
}

// ConfigRequirements is the exact public configuration-requirements value.
// Gollem's broader live provider/workspace requirements remain separate.
type ConfigRequirements struct {
	AllowedApprovalPolicies              *[]AskForApproval          `json:"allowedApprovalPolicies"`
	AllowedSandboxModes                  *[]SandboxMode             `json:"allowedSandboxModes"`
	AllowedWindowsSandboxImplementations *[]WindowsSandboxSetupMode `json:"allowedWindowsSandboxImplementations"`
	AllowedPermissionProfiles            *map[string]bool           `json:"allowedPermissionProfiles"`
	DefaultPermissions                   *string                    `json:"defaultPermissions"`
	AllowedWebSearchModes                *[]WebSearchMode           `json:"allowedWebSearchModes"`
	AllowManagedHooksOnly                *bool                      `json:"allowManagedHooksOnly"`
	AllowAppshots                        *bool                      `json:"allowAppshots"`
	AllowRemoteControl                   *bool                      `json:"allowRemoteControl"`
	ComputerUse                          *ComputerUseRequirements   `json:"computerUse"`
	FeatureRequirements                  *map[string]bool           `json:"featureRequirements"`
	EnforceResidency                     *ResidencyRequirement      `json:"enforceResidency"`
	Models                               *ModelsRequirements        `json:"models"`
}

func (r *ConfigRequirements) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode config requirements into nil receiver")
	}
	const objectName = "config requirements"
	payload, err := decodeExactThreadItemObject(
		data,
		objectName,
		"allowedApprovalPolicies",
		"allowedSandboxModes",
		"allowedWindowsSandboxImplementations",
		"allowedPermissionProfiles",
		"defaultPermissions",
		"allowedWebSearchModes",
		"allowManagedHooksOnly",
		"allowAppshots",
		"allowRemoteControl",
		"computerUse",
		"featureRequirements",
		"enforceResidency",
		"models",
	)
	if err != nil {
		return err
	}
	allowedApprovalPolicies, err := decodeOptionalNullableConfigRequirementArray[AskForApproval](
		payload, objectName, "allowedApprovalPolicies",
	)
	if err != nil {
		return err
	}
	allowedSandboxModes, err := decodeOptionalNullableConfigRequirementArray[SandboxMode](
		payload, objectName, "allowedSandboxModes",
	)
	if err != nil {
		return err
	}
	allowedWindowsSandboxImplementations, err := decodeOptionalNullableConfigRequirementArray[WindowsSandboxSetupMode](
		payload, objectName, "allowedWindowsSandboxImplementations",
	)
	if err != nil {
		return err
	}
	allowedPermissionProfiles, err := decodeOptionalNullableConfigRequirementBoolMap(
		payload, objectName, "allowedPermissionProfiles",
	)
	if err != nil {
		return err
	}
	defaultPermissions, err := decodeOptionalNullableConfigRequirementValue[string](
		payload, objectName, "defaultPermissions",
	)
	if err != nil {
		return err
	}
	allowedWebSearchModes, err := decodeOptionalNullableConfigRequirementArray[WebSearchMode](
		payload, objectName, "allowedWebSearchModes",
	)
	if err != nil {
		return err
	}
	allowManagedHooksOnly, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "allowManagedHooksOnly",
	)
	if err != nil {
		return err
	}
	allowAppshots, err := decodeOptionalNullableConfigRequirementValue[bool](payload, objectName, "allowAppshots")
	if err != nil {
		return err
	}
	allowRemoteControl, err := decodeOptionalNullableConfigRequirementValue[bool](
		payload, objectName, "allowRemoteControl",
	)
	if err != nil {
		return err
	}
	computerUse, err := decodeOptionalNullableConfigRequirementValue[ComputerUseRequirements](
		payload, objectName, "computerUse",
	)
	if err != nil {
		return err
	}
	featureRequirements, err := decodeOptionalNullableConfigRequirementBoolMap(
		payload, objectName, "featureRequirements",
	)
	if err != nil {
		return err
	}
	enforceResidency, err := decodeOptionalNullableConfigRequirementValue[ResidencyRequirement](
		payload, objectName, "enforceResidency",
	)
	if err != nil {
		return err
	}
	models, err := decodeOptionalNullableConfigRequirementValue[ModelsRequirements](payload, objectName, "models")
	if err != nil {
		return err
	}
	*r = ConfigRequirements{
		AllowedApprovalPolicies:              allowedApprovalPolicies,
		AllowedSandboxModes:                  allowedSandboxModes,
		AllowedWindowsSandboxImplementations: allowedWindowsSandboxImplementations,
		AllowedPermissionProfiles:            allowedPermissionProfiles,
		DefaultPermissions:                   defaultPermissions,
		AllowedWebSearchModes:                allowedWebSearchModes,
		AllowManagedHooksOnly:                allowManagedHooksOnly,
		AllowAppshots:                        allowAppshots,
		AllowRemoteControl:                   allowRemoteControl,
		ComputerUse:                          computerUse,
		FeatureRequirements:                  featureRequirements,
		EnforceResidency:                     enforceResidency,
		Models:                               models,
	}
	return nil
}

// ConfigRequirementsReadResponse is the exact public read response. It stays
// standalone until the broader live handler has an explicit projection.
type ConfigRequirementsReadResponse struct {
	Requirements *ConfigRequirements `json:"requirements"`
}

func (r *ConfigRequirementsReadResponse) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errors.New("decode config-requirements read response into nil receiver")
	}
	const objectName = "config-requirements read response"
	payload, err := decodeExactThreadItemObject(data, objectName, "requirements")
	if err != nil {
		return err
	}
	requirements, err := decodeOptionalNullableConfigRequirementValue[ConfigRequirements](
		payload, objectName, "requirements",
	)
	if err != nil {
		return err
	}
	*r = ConfigRequirementsReadResponse{Requirements: requirements}
	return nil
}

func decodeOptionalNullableConfigRequirementValue[T any](
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

func decodeOptionalNullableConfigRequirementArray[T any](
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*[]T, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]T, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return &values, nil
}

func decodeOptionalNullableConfigRequirementBoolMap(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) (*map[string]bool, error) {
	raw, ok := payload[fieldName]
	if !ok || isJSONNull(raw) {
		return nil, nil
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make(map[string]bool, len(entries))
	for name, encoded := range entries {
		if isJSONNull(encoded) {
			return nil, fmt.Errorf("decode %s %s[%q]: value cannot be null", objectName, fieldName, name)
		}
		var value bool
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("decode %s %s[%q]: %w", objectName, fieldName, name, err)
		}
		values[name] = value
	}
	return &values, nil
}

var (
	_ json.Unmarshaler = (*ComputerUseRequirements)(nil)
	_ json.Marshaler   = ResidencyRequirement("")
	_ json.Unmarshaler = (*ResidencyRequirement)(nil)
	_ json.Marshaler   = WebSearchMode("")
	_ json.Unmarshaler = (*WebSearchMode)(nil)
	_ json.Marshaler   = WindowsSandboxSetupMode("")
	_ json.Unmarshaler = (*WindowsSandboxSetupMode)(nil)
	_ json.Unmarshaler = (*ConfigRequirements)(nil)
	_ json.Unmarshaler = (*ConfigRequirementsReadResponse)(nil)
)
