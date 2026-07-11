package appserver

import (
	"encoding/json"
	"strings"

	"github.com/fugue-labs/gollem/core"
)

type RuntimeModelParams struct {
	ProviderID       string          `json:"providerId,omitempty"`
	Provider         string          `json:"provider,omitempty"`
	Model            string          `json:"model,omitempty"`
	MaxTokens        *int            `json:"maxTokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"topP,omitempty"`
	ThinkingBudget   *int            `json:"thinkingBudget,omitempty"`
	AdaptiveThinking *bool           `json:"adaptiveThinking,omitempty"`
	ReasoningEffort  *string         `json:"reasoningEffort,omitempty"`
	StopSequences    []string        `json:"stopSequences,omitempty"`
	Settings         map[string]any  `json:"settings,omitempty"`
	Input            json.RawMessage `json:"input,omitempty"`
}

type threadStartParams struct {
	Title     string         `json:"title,omitempty"`
	Workspace string         `json:"workspace,omitempty"`
	Prompt    string         `json:"prompt,omitempty"`
	Message   string         `json:"message,omitempty"`
	Text      string         `json:"text,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

type threadResumeParams struct {
	ID       string         `json:"id,omitempty"`
	ThreadID string         `json:"threadId,omitempty"`
	Prompt   string         `json:"prompt,omitempty"`
	Message  string         `json:"message,omitempty"`
	Text     string         `json:"text,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

func (p threadResumeParams) turnStartParams() turnStartParams {
	return turnStartParams(p)
}

type threadSettingsUpdateParams struct {
	ID       string         `json:"id,omitempty"`
	ThreadID string         `json:"threadId,omitempty"`
	Settings map[string]any `json:"settings,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Replace  bool           `json:"replace,omitempty"`
}

const (
	threadGoalSettingKey       = "goal"
	threadMemoryModeSettingKey = "memoryMode"
)

type threadMetadataUpdateParams struct {
	ID       string         `json:"id,omitempty"`
	ThreadID string         `json:"threadId,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Replace  bool           `json:"replace,omitempty"`
}

func (p threadMetadataUpdateParams) threadID() string {
	return firstNonEmpty(p.ThreadID, p.ID)
}

type threadMemoryModeSetParams struct {
	ID         string `json:"id,omitempty"`
	ThreadID   string `json:"threadId,omitempty"`
	Mode       string `json:"mode,omitempty"`
	MemoryMode string `json:"memoryMode,omitempty"`
}

func (p threadMemoryModeSetParams) threadID() string {
	return firstNonEmpty(p.ThreadID, p.ID)
}

type turnStartParams struct {
	ID       string         `json:"id,omitempty"`
	ThreadID string         `json:"threadId,omitempty"`
	Prompt   string         `json:"prompt,omitempty"`
	Message  string         `json:"message,omitempty"`
	Text     string         `json:"text,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

type turnIDParams struct {
	ID       string `json:"id,omitempty"`
	TurnID   string `json:"turnId,omitempty"`
	ThreadID string `json:"threadId,omitempty"`
}

func (p turnIDParams) turnID() string {
	return firstNonEmpty(p.TurnID, p.ID)
}

type turnSteerParams struct {
	ID      string `json:"id,omitempty"`
	TurnID  string `json:"turnId,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Message string `json:"message,omitempty"`
	Text    string `json:"text,omitempty"`
}

type turnRetryParams struct {
	ID       string         `json:"id,omitempty"`
	TurnID   string         `json:"turnId,omitempty"`
	Prompt   string         `json:"prompt,omitempty"`
	Message  string         `json:"message,omitempty"`
	Text     string         `json:"text,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
	RuntimeModelParams
}

func runtimePromptFromStartParams(prompt, message, text string, input json.RawMessage) string {
	return strings.TrimSpace(firstNonEmpty(prompt, message, text, runtimePromptFromInput(input)))
}

func runtimeSelectionFromParams(providerID, provider, model string) RuntimeModelSelection {
	return RuntimeModelSelection{
		ProviderID: strings.TrimSpace(providerID),
		Provider:   strings.TrimSpace(provider),
		Model:      strings.TrimSpace(model),
	}
}

func runtimeSelectionWithThreadDefaults(selection RuntimeModelSelection, settings map[string]any) RuntimeModelSelection {
	if selection.ProviderID == "" {
		selection.ProviderID = stringSetting(settings, "providerId")
	}
	if selection.Provider == "" {
		selection.Provider = stringSetting(settings, "provider")
	}
	if selection.Model == "" {
		selection.Model = stringSetting(settings, "model")
	}
	return selection
}

func runtimeModelSettingsFromParams(params RuntimeModelParams) core.ModelSettings {
	settings := core.ModelSettings{
		MaxTokens:        params.MaxTokens,
		Temperature:      params.Temperature,
		TopP:             params.TopP,
		ThinkingBudget:   params.ThinkingBudget,
		AdaptiveThinking: params.AdaptiveThinking,
		ReasoningEffort:  params.ReasoningEffort,
		StopSequences:    append([]string(nil), params.StopSequences...),
	}
	if settings.ReasoningEffort == nil {
		if effort := stringSetting(params.Settings, "reasoningEffort"); effort != "" {
			settings.ReasoningEffort = &effort
		}
	}
	return settings
}

func cloneSettings(settings map[string]any) map[string]any {
	if len(settings) == 0 {
		return nil
	}
	out := make(map[string]any, len(settings))
	for key, value := range settings {
		out[key] = value
	}
	return out
}

func mergeRuntimeSelectionIntoSettings(settings map[string]any, providerID, provider, model string) map[string]any {
	if settings == nil {
		settings = make(map[string]any)
	}
	if strings.TrimSpace(providerID) != "" {
		settings["providerId"] = strings.TrimSpace(providerID)
	}
	if strings.TrimSpace(provider) != "" {
		settings["provider"] = strings.TrimSpace(provider)
	}
	if strings.TrimSpace(model) != "" {
		settings["model"] = strings.TrimSpace(model)
	}
	if len(settings) == 0 {
		return nil
	}
	return settings
}

func stringSetting(settings map[string]any, key string) string {
	value, ok := settings[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func firstRaw(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(value) > 0 {
			out := make([]byte, len(value))
			copy(out, value)
			return out
		}
	}
	return nil
}

func runtimeSteerReason(accepted bool) string {
	if accepted {
		return "steer message recorded for the active turn"
	}
	return "turn is not currently active; steer message was recorded for audit only"
}
