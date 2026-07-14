package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Config is the exact standalone public effective-config value. It is kept
// separate from Gollem's memory-backed app-server config service.
type Config struct {
	Model                           *string                     `json:"model"`
	ReviewModel                     *string                     `json:"review_model"`
	ModelContextWindow              *int64                      `json:"model_context_window"`
	ModelAutoCompactTokenLimit      *int64                      `json:"model_auto_compact_token_limit"`
	ModelAutoCompactTokenLimitScope *AutoCompactTokenLimitScope `json:"model_auto_compact_token_limit_scope"`
	ModelProvider                   *string                     `json:"model_provider"`
	ApprovalPolicy                  *AskForApproval             `json:"approval_policy"`
	ApprovalsReviewer               *ApprovalsReviewer          `json:"approvals_reviewer"`
	SandboxMode                     *SandboxMode                `json:"sandbox_mode"`
	SandboxWorkspaceWrite           *SandboxWorkspaceWrite      `json:"sandbox_workspace_write"`
	ForcedChatgptWorkspaceID        *ForcedChatgptWorkspaceIds  `json:"forced_chatgpt_workspace_id"`
	ForcedLoginMethod               *ForcedLoginMethod          `json:"forced_login_method"`
	WebSearch                       *WebSearchMode              `json:"web_search"`
	Tools                           *ToolsV2                    `json:"tools"`
	Instructions                    *string                     `json:"instructions"`
	DeveloperInstructions           *string                     `json:"developer_instructions"`
	CompactPrompt                   *string                     `json:"compact_prompt"`
	ModelReasoningEffort            *ReasoningEffort            `json:"model_reasoning_effort"`
	ModelReasoningSummary           *ReasoningSummary           `json:"model_reasoning_summary"`
	ModelVerbosity                  *Verbosity                  `json:"model_verbosity"`
	ServiceTier                     *string                     `json:"service_tier"`
	Analytics                       *AnalyticsConfig            `json:"analytics"`
	Desktop                         map[string]JsonValue        `json:"desktop"`
	Additional                      map[string]JsonValue        `json:"-"`
}

func (c Config) MarshalJSON() ([]byte, error) {
	fields := make(map[string]json.RawMessage, len(c.Additional)+len(publicConfigKnownFields))
	for name, value := range c.Additional {
		if isPublicConfigKnownField(name) {
			return nil, fmt.Errorf("Config additional fields cannot redefine %s", name)
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode Config additional field %q: %w", name, err)
		}
		fields[name] = encoded
	}
	for name, value := range map[string]any{
		"model":                                c.Model,
		"review_model":                         c.ReviewModel,
		"model_context_window":                 c.ModelContextWindow,
		"model_auto_compact_token_limit":       c.ModelAutoCompactTokenLimit,
		"model_auto_compact_token_limit_scope": c.ModelAutoCompactTokenLimitScope,
		"model_provider":                       c.ModelProvider,
		"approval_policy":                      c.ApprovalPolicy,
		"approvals_reviewer":                   c.ApprovalsReviewer,
		"sandbox_mode":                         c.SandboxMode,
		"sandbox_workspace_write":              c.SandboxWorkspaceWrite,
		"forced_chatgpt_workspace_id":          c.ForcedChatgptWorkspaceID,
		"forced_login_method":                  c.ForcedLoginMethod,
		"web_search":                           c.WebSearch,
		"tools":                                c.Tools,
		"instructions":                         c.Instructions,
		"developer_instructions":               c.DeveloperInstructions,
		"compact_prompt":                       c.CompactPrompt,
		"model_reasoning_effort":               c.ModelReasoningEffort,
		"model_reasoning_summary":              c.ModelReasoningSummary,
		"model_verbosity":                      c.ModelVerbosity,
		"service_tier":                         c.ServiceTier,
		"analytics":                            c.Analytics,
		"desktop":                              c.Desktop,
	} {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("encode Config field %s: %w", name, err)
		}
		fields[name] = encoded
	}
	return json.Marshal(fields)
}

func (c *Config) UnmarshalJSON(data []byte) error {
	if c == nil {
		return errors.New("decode Config into nil receiver")
	}
	payload, err := decodePublicConfigObject(data)
	if err != nil {
		return err
	}
	model, err := decodePublicConfigValue[string](payload, "model")
	if err != nil {
		return err
	}
	reviewModel, err := decodePublicConfigValue[string](payload, "review_model")
	if err != nil {
		return err
	}
	modelContextWindow, err := decodePublicConfigValue[int64](payload, "model_context_window")
	if err != nil {
		return err
	}
	modelAutoCompactTokenLimit, err := decodePublicConfigValue[int64](payload, "model_auto_compact_token_limit")
	if err != nil {
		return err
	}
	modelAutoCompactLimitScope, err := decodePublicConfigValue[AutoCompactTokenLimitScope](
		payload, "model_auto_compact_token_limit_scope",
	)
	if err != nil {
		return err
	}
	modelProvider, err := decodePublicConfigValue[string](payload, "model_provider")
	if err != nil {
		return err
	}
	approvalPolicy, err := decodePublicConfigValue[AskForApproval](payload, "approval_policy")
	if err != nil {
		return err
	}
	approvalsReviewer, err := decodePublicConfigValue[ApprovalsReviewer](payload, "approvals_reviewer")
	if err != nil {
		return err
	}
	sandboxMode, err := decodePublicConfigValue[SandboxMode](payload, "sandbox_mode")
	if err != nil {
		return err
	}
	sandboxWorkspaceWrite, err := decodePublicConfigValue[SandboxWorkspaceWrite](
		payload, "sandbox_workspace_write",
	)
	if err != nil {
		return err
	}
	forcedChatgptWorkspaceID, err := decodePublicConfigValue[ForcedChatgptWorkspaceIds](
		payload, "forced_chatgpt_workspace_id",
	)
	if err != nil {
		return err
	}
	forcedLoginMethod, err := decodePublicConfigValue[ForcedLoginMethod](payload, "forced_login_method")
	if err != nil {
		return err
	}
	webSearch, err := decodePublicConfigValue[WebSearchMode](payload, "web_search")
	if err != nil {
		return err
	}
	tools, err := decodePublicConfigValue[ToolsV2](payload, "tools")
	if err != nil {
		return err
	}
	instructions, err := decodePublicConfigValue[string](payload, "instructions")
	if err != nil {
		return err
	}
	developerInstructions, err := decodePublicConfigValue[string](payload, "developer_instructions")
	if err != nil {
		return err
	}
	compactPrompt, err := decodePublicConfigValue[string](payload, "compact_prompt")
	if err != nil {
		return err
	}
	modelReasoningEffort, err := decodePublicConfigValue[ReasoningEffort](payload, "model_reasoning_effort")
	if err != nil {
		return err
	}
	modelReasoningSummary, err := decodePublicConfigValue[ReasoningSummary](payload, "model_reasoning_summary")
	if err != nil {
		return err
	}
	modelVerbosity, err := decodePublicConfigValue[Verbosity](payload, "model_verbosity")
	if err != nil {
		return err
	}
	serviceTier, err := decodePublicConfigValue[string](payload, "service_tier")
	if err != nil {
		return err
	}
	analytics, err := decodePublicConfigValue[AnalyticsConfig](payload, "analytics")
	if err != nil {
		return err
	}
	desktop, err := decodePublicConfigValue[map[string]JsonValue](payload, "desktop")
	if err != nil {
		return err
	}

	additional := make(map[string]JsonValue, len(payload))
	for name, raw := range payload {
		if isPublicConfigKnownField(name) {
			continue
		}
		additional[name] = JsonValue{raw: append(json.RawMessage(nil), raw...)}
	}
	*c = Config{
		Model:                           model,
		ReviewModel:                     reviewModel,
		ModelContextWindow:              modelContextWindow,
		ModelAutoCompactTokenLimit:      modelAutoCompactTokenLimit,
		ModelAutoCompactTokenLimitScope: modelAutoCompactLimitScope,
		ModelProvider:                   modelProvider,
		ApprovalPolicy:                  approvalPolicy,
		ApprovalsReviewer:               approvalsReviewer,
		SandboxMode:                     sandboxMode,
		SandboxWorkspaceWrite:           sandboxWorkspaceWrite,
		ForcedChatgptWorkspaceID:        forcedChatgptWorkspaceID,
		ForcedLoginMethod:               forcedLoginMethod,
		WebSearch:                       webSearch,
		Tools:                           tools,
		Instructions:                    instructions,
		DeveloperInstructions:           developerInstructions,
		CompactPrompt:                   compactPrompt,
		ModelReasoningEffort:            modelReasoningEffort,
		ModelReasoningSummary:           modelReasoningSummary,
		ModelVerbosity:                  modelVerbosity,
		ServiceTier:                     serviceTier,
		Analytics:                       analytics,
		Desktop:                         dereferencePublicConfigMap(desktop),
		Additional:                      additional,
	}
	return nil
}

func decodePublicConfigObject(data []byte) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode Config: %w", err)
	}
	if opening != json.Delim('{') {
		return nil, errors.New("Config must be an object")
	}
	payload := make(map[string]json.RawMessage)
	seenKnown := make(map[string]bool, len(publicConfigKnownFields))
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode Config field name: %w", err)
		}
		name := token.(string)
		if isPublicConfigKnownField(name) {
			if seenKnown[name] {
				return nil, fmt.Errorf("duplicate Config field %q", name)
			}
			seenKnown[name] = true
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, fmt.Errorf("decode Config field %q: %w", name, err)
		}
		payload[name] = append(json.RawMessage(nil), raw...)
	}
	_, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode Config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("Config must contain one JSON value")
		}
		return nil, fmt.Errorf("decode Config trailing value: %w", err)
	}
	return payload, nil
}

func decodePublicConfigValue[T any](payload map[string]json.RawMessage, name string) (*T, error) {
	value, err := decodeOptionalNullableConfigRequirementValue[T](payload, "Config", name)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func dereferencePublicConfigMap(value *map[string]JsonValue) map[string]JsonValue {
	if value == nil {
		return nil
	}
	return *value
}

func isPublicConfigKnownField(name string) bool {
	for _, known := range publicConfigKnownFields {
		if name == known {
			return true
		}
	}
	return false
}

var publicConfigKnownFields = []string{
	"model",
	"review_model",
	"model_context_window",
	"model_auto_compact_token_limit",
	"model_auto_compact_token_limit_scope",
	"model_provider",
	"approval_policy",
	"approvals_reviewer",
	"sandbox_mode",
	"sandbox_workspace_write",
	"forced_chatgpt_workspace_id",
	"forced_login_method",
	"web_search",
	"tools",
	"instructions",
	"developer_instructions",
	"compact_prompt",
	"model_reasoning_effort",
	"model_reasoning_summary",
	"model_verbosity",
	"service_tier",
	"analytics",
	"desktop",
}

var (
	_ json.Marshaler   = Config{}
	_ json.Unmarshaler = (*Config)(nil)
)
