package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// AutoCompactTokenLimitScope selects the context charged against the automatic
// compaction token limit.
type AutoCompactTokenLimitScope string

const (
	AutoCompactTokenLimitScopeTotal           AutoCompactTokenLimitScope = "total"
	AutoCompactTokenLimitScopeBodyAfterPrefix AutoCompactTokenLimitScope = "body_after_prefix"
)

func (s AutoCompactTokenLimitScope) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "automatic compaction token-limit scope", AutoCompactTokenLimitScope.valid)
}

func (s *AutoCompactTokenLimitScope) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "automatic compaction token-limit scope", AutoCompactTokenLimitScope.valid)
}

func (s AutoCompactTokenLimitScope) valid() bool {
	return s == AutoCompactTokenLimitScopeTotal || s == AutoCompactTokenLimitScopeBodyAfterPrefix
}

// ForcedLoginMethod is the exact closed public login method.
type ForcedLoginMethod string

const (
	ForcedLoginMethodChatGPT ForcedLoginMethod = "chatgpt"
	ForcedLoginMethodAPI     ForcedLoginMethod = "api"
)

func (m ForcedLoginMethod) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(m, "forced login method", ForcedLoginMethod.valid)
}

func (m *ForcedLoginMethod) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, m, "forced login method", ForcedLoginMethod.valid)
}

func (m ForcedLoginMethod) valid() bool {
	return m == ForcedLoginMethodChatGPT || m == ForcedLoginMethodAPI
}

// Verbosity is the exact closed public model verbosity.
type Verbosity string

const (
	VerbosityLow    Verbosity = "low"
	VerbosityMedium Verbosity = "medium"
	VerbosityHigh   Verbosity = "high"
)

func (v Verbosity) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(v, "model verbosity", Verbosity.valid)
}

func (v *Verbosity) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, v, "model verbosity", Verbosity.valid)
}

func (v Verbosity) valid() bool {
	switch v {
	case VerbosityLow, VerbosityMedium, VerbosityHigh:
		return true
	default:
		return false
	}
}

// WebSearchContextSize is the exact closed public web-search context size.
type WebSearchContextSize string

const (
	WebSearchContextSizeLow    WebSearchContextSize = "low"
	WebSearchContextSizeMedium WebSearchContextSize = "medium"
	WebSearchContextSizeHigh   WebSearchContextSize = "high"
)

func (s WebSearchContextSize) MarshalJSON() ([]byte, error) {
	return marshalThreadTurnLeafEnum(s, "web-search context size", WebSearchContextSize.valid)
}

func (s *WebSearchContextSize) UnmarshalJSON(data []byte) error {
	return unmarshalThreadTurnLeafEnum(data, s, "web-search context size", WebSearchContextSize.valid)
}

func (s WebSearchContextSize) valid() bool {
	switch s {
	case WebSearchContextSizeLow, WebSearchContextSizeMedium, WebSearchContextSizeHigh:
		return true
	default:
		return false
	}
}

// ForcedChatgptWorkspaceIds preserves whether the public compatibility value
// used its single-workspace or multiple-workspace wire form.
type ForcedChatgptWorkspaceIds struct {
	raw json.RawMessage
}

func (i ForcedChatgptWorkspaceIds) MarshalJSON() ([]byte, error) {
	if len(i.raw) == 0 {
		return nil, errors.New("forced ChatGPT workspace IDs have no value")
	}
	return validateForcedChatgptWorkspaceIDsJSON(i.raw)
}

func (i *ForcedChatgptWorkspaceIds) UnmarshalJSON(data []byte) error {
	if i == nil {
		return errors.New("decode forced ChatGPT workspace IDs into nil receiver")
	}
	canonical, err := validateForcedChatgptWorkspaceIDsJSON(data)
	if err != nil {
		return err
	}
	i.raw = canonical
	return nil
}

func validateForcedChatgptWorkspaceIDsJSON(data []byte) (json.RawMessage, error) {
	if isJSONNull(data) {
		return nil, errors.New("forced ChatGPT workspace IDs cannot be null")
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		return json.Marshal(single)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(data, &elements); err != nil {
		return nil, fmt.Errorf("decode forced ChatGPT workspace IDs: %w", err)
	}
	values := make([]string, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode forced ChatGPT workspace IDs[%d]: value cannot be null", index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode forced ChatGPT workspace IDs[%d]: %w", index, err)
		}
	}
	return json.Marshal(values)
}

// AnalyticsConfig preserves public flattened analytics fields as exact JSON
// values while keeping enabled as the one known nullable field.
type AnalyticsConfig struct {
	Enabled    *bool                `json:"enabled"`
	Additional map[string]JsonValue `json:"-"`
}

func (c AnalyticsConfig) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(c.Additional)+1)
	for name, value := range c.Additional {
		if name == "enabled" {
			return nil, errors.New("analytics additional fields cannot redefine enabled")
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode analytics field %q: %w", name, err)
		}
		fields[name] = encoded
	}
	enabled := json.RawMessage("null")
	if c.Enabled != nil {
		if *c.Enabled {
			enabled = json.RawMessage("true")
		} else {
			enabled = json.RawMessage("false")
		}
	}
	fields["enabled"] = enabled
	return json.Marshal(fields)
}

func (c *AnalyticsConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode analytics config into nil receiver")
	}
	const objectName = "analytics config"
	payload, err := decodeOpenConfigPrerequisiteObject(data, objectName)
	if err != nil {
		return err
	}
	enabled, err := decodeOptionalNullableConfigRequirementValue[bool](payload, objectName, "enabled")
	if err != nil {
		return err
	}
	additional := make(map[string]JsonValue, len(payload))
	for name, raw := range payload {
		if name == "enabled" {
			continue
		}
		additional[name] = JsonValue{raw: append(json.RawMessage(nil), raw...)}
	}
	*c = AnalyticsConfig{Enabled: enabled, Additional: additional}
	return nil
}

// SandboxWorkspaceWrite is the exact public writable-workspace sandbox
// configuration. Unknown fields are accepted for Rust-compatible evolution.
type SandboxWorkspaceWrite struct {
	WritableRoots       []string `json:"writable_roots"`
	NetworkAccess       bool     `json:"network_access"`
	ExcludeTmpdirEnvVar bool     `json:"exclude_tmpdir_env_var"`
	ExcludeSlashTmp     bool     `json:"exclude_slash_tmp"`
}

func (w SandboxWorkspaceWrite) MarshalJSON() ([]byte, error) {
	writableRoots := w.WritableRoots
	if writableRoots == nil {
		writableRoots = []string{}
	}
	return json.Marshal(struct {
		WritableRoots       []string `json:"writable_roots"`
		NetworkAccess       bool     `json:"network_access"`
		ExcludeTmpdirEnvVar bool     `json:"exclude_tmpdir_env_var"`
		ExcludeSlashTmp     bool     `json:"exclude_slash_tmp"`
	}{
		WritableRoots:       writableRoots,
		NetworkAccess:       w.NetworkAccess,
		ExcludeTmpdirEnvVar: w.ExcludeTmpdirEnvVar,
		ExcludeSlashTmp:     w.ExcludeSlashTmp,
	})
}

func (w *SandboxWorkspaceWrite) UnmarshalJSON(data []byte) error {
	if w == nil {
		return errors.New("decode sandbox workspace-write config into nil receiver")
	}
	const objectName = "sandbox workspace-write config"
	payload, err := decodeOpenConfigPrerequisiteObject(data, objectName)
	if err != nil {
		return err
	}
	writableRoots, err := decodeDefaultedConfigPrerequisiteStringArray(payload, objectName, "writable_roots")
	if err != nil {
		return err
	}
	networkAccess, err := decodeOptionalConfigBool(payload, objectName, "network_access")
	if err != nil {
		return err
	}
	excludeTmpdirEnvVar, err := decodeOptionalConfigBool(payload, objectName, "exclude_tmpdir_env_var")
	if err != nil {
		return err
	}
	excludeSlashTmp, err := decodeOptionalConfigBool(payload, objectName, "exclude_slash_tmp")
	if err != nil {
		return err
	}
	*w = SandboxWorkspaceWrite{
		WritableRoots:       writableRoots,
		NetworkAccess:       networkAccess,
		ExcludeTmpdirEnvVar: excludeTmpdirEnvVar,
		ExcludeSlashTmp:     excludeSlashTmp,
	}
	return nil
}

// WebSearchLocation is the exact closed public web-search location.
type WebSearchLocation struct {
	Country  *string `json:"country"`
	Region   *string `json:"region"`
	City     *string `json:"city"`
	Timezone *string `json:"timezone"`
}

func (l *WebSearchLocation) UnmarshalJSON(data []byte) error {
	if l == nil {
		return errors.New("decode web-search location into nil receiver")
	}
	const objectName = "web-search location"
	payload, err := decodeExactThreadItemObject(data, objectName, "country", "region", "city", "timezone")
	if err != nil {
		return err
	}
	country, err := decodeOptionalNullableConfigRequirementValue[string](payload, objectName, "country")
	if err != nil {
		return err
	}
	region, err := decodeOptionalNullableConfigRequirementValue[string](payload, objectName, "region")
	if err != nil {
		return err
	}
	city, err := decodeOptionalNullableConfigRequirementValue[string](payload, objectName, "city")
	if err != nil {
		return err
	}
	timezone, err := decodeOptionalNullableConfigRequirementValue[string](payload, objectName, "timezone")
	if err != nil {
		return err
	}
	*l = WebSearchLocation{Country: country, Region: region, City: city, Timezone: timezone}
	return nil
}

// WebSearchToolConfig is the exact closed public web-search tool config.
type WebSearchToolConfig struct {
	ContextSize    *WebSearchContextSize `json:"context_size"`
	AllowedDomains []string              `json:"allowed_domains"`
	Location       *WebSearchLocation    `json:"location"`
}

func (c *WebSearchToolConfig) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode web-search tool config into nil receiver")
	}
	const objectName = "web-search tool config"
	payload, err := decodeExactThreadItemObject(data, objectName, "context_size", "allowed_domains", "location")
	if err != nil {
		return err
	}
	contextSize, err := decodeOptionalNullableConfigRequirementValue[WebSearchContextSize](
		payload, objectName, "context_size",
	)
	if err != nil {
		return err
	}
	allowedDomainsValue, err := decodeOptionalNullableConfigRequirementArray[string](
		payload, objectName, "allowed_domains",
	)
	if err != nil {
		return err
	}
	var allowedDomains []string
	if allowedDomainsValue != nil {
		allowedDomains = *allowedDomainsValue
	}
	location, err := decodeOptionalNullableConfigRequirementValue[WebSearchLocation](payload, objectName, "location")
	if err != nil {
		return err
	}
	*c = WebSearchToolConfig{ContextSize: contextSize, AllowedDomains: allowedDomains, Location: location}
	return nil
}

// ToolsV2 is the public v2 tool configuration. Unknown top-level fields are
// accepted for Rust-compatible evolution; known nested configs stay strict.
type ToolsV2 struct {
	WebSearch *WebSearchToolConfig `json:"web_search"`
}

func (t *ToolsV2) UnmarshalJSON(data []byte) error {
	if t == nil {
		return errors.New("decode v2 tools config into nil receiver")
	}
	const objectName = "v2 tools config"
	payload, err := decodeOpenConfigPrerequisiteObject(data, objectName)
	if err != nil {
		return err
	}
	webSearch, err := decodeOptionalNullableConfigRequirementValue[WebSearchToolConfig](
		payload, objectName, "web_search",
	)
	if err != nil {
		return err
	}
	*t = ToolsV2{WebSearch: webSearch}
	return nil
}

func decodeOpenConfigPrerequisiteObject(data []byte, objectName string) (map[string]json.RawMessage, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("decode %s: %w", objectName, err)
	}
	if payload == nil {
		return nil, fmt.Errorf("%s must be an object", objectName)
	}
	return payload, nil
}

func decodeDefaultedConfigPrerequisiteStringArray(
	payload map[string]json.RawMessage,
	objectName string,
	fieldName string,
) ([]string, error) {
	raw, ok := payload[fieldName]
	if !ok {
		return []string{}, nil
	}
	if isJSONNull(raw) {
		return nil, fmt.Errorf("decode %s %s: value cannot be null", objectName, fieldName)
	}
	var elements []json.RawMessage
	if err := json.Unmarshal(raw, &elements); err != nil {
		return nil, fmt.Errorf("decode %s %s: %w", objectName, fieldName, err)
	}
	values := make([]string, len(elements))
	for index, element := range elements {
		if isJSONNull(element) {
			return nil, fmt.Errorf("decode %s %s[%d]: value cannot be null", objectName, fieldName, index)
		}
		if err := json.Unmarshal(element, &values[index]); err != nil {
			return nil, fmt.Errorf("decode %s %s[%d]: %w", objectName, fieldName, index, err)
		}
	}
	return values, nil
}

var (
	_ json.Marshaler   = AutoCompactTokenLimitScope("")
	_ json.Unmarshaler = (*AutoCompactTokenLimitScope)(nil)
	_ json.Marshaler   = ForcedLoginMethod("")
	_ json.Unmarshaler = (*ForcedLoginMethod)(nil)
	_ json.Marshaler   = Verbosity("")
	_ json.Unmarshaler = (*Verbosity)(nil)
	_ json.Marshaler   = WebSearchContextSize("")
	_ json.Unmarshaler = (*WebSearchContextSize)(nil)
	_ json.Marshaler   = ForcedChatgptWorkspaceIds{}
	_ json.Unmarshaler = (*ForcedChatgptWorkspaceIds)(nil)
	_ json.Marshaler   = AnalyticsConfig{}
	_ json.Unmarshaler = (*AnalyticsConfig)(nil)
	_ json.Marshaler   = SandboxWorkspaceWrite{}
	_ json.Unmarshaler = (*SandboxWorkspaceWrite)(nil)
	_ json.Unmarshaler = (*WebSearchLocation)(nil)
	_ json.Unmarshaler = (*WebSearchToolConfig)(nil)
	_ json.Unmarshaler = (*ToolsV2)(nil)
)
