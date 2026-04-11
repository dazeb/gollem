package anthropic

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// --- API request types ---

type apiRequest struct {
	Model       string           `json:"model"`
	MaxTokens   int              `json:"max_tokens"`
	System      []apiSystemBlock `json:"system,omitempty"`
	Messages    []apiMessage     `json:"messages"`
	Tools       []any            `json:"tools,omitempty"` // apiTool or apiBuiltinTool
	ToolChoice  *apiToolChoice   `json:"tool_choice,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	Thinking    *apiThinking     `json:"thinking,omitempty"`
}

type apiToolChoice struct {
	Type string `json:"type"`           // "auto", "any", "tool"
	Name string `json:"name,omitempty"` // required when type is "tool"
}

type apiThinking struct {
	Type         string `json:"type"`          // "enabled" or "disabled"
	BudgetTokens int    `json:"budget_tokens"` // Max tokens for thinking
}

type apiCacheControl struct {
	Type string `json:"type"` // "ephemeral"
}

type apiSystemBlock struct {
	Type         string           `json:"type"`
	Text         string           `json:"text"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
}

type apiMessage struct {
	Role    string            `json:"role"`
	Content []apiContentBlock `json:"content"`
}

type apiContentBlock struct {
	Type string `json:"type"`
	// text block
	Text string `json:"text,omitempty"`
	// tool_use block
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
	// thinking block
	Thinking  string `json:"thinking,omitempty"`
	Signature string `json:"signature,omitempty"`

	// rawOverride, when non-nil, replaces the entire struct during JSON
	// marshaling. Used to round-trip opaque provider content blocks
	// (server_tool_use, tool_reference, etc.) that don't fit the typed
	// fields above. The block is emitted as-is without interpretation.
	rawOverride json.RawMessage `json:"-"`
}

// MarshalJSON emits rawOverride verbatim when set, otherwise falls back
// to standard struct marshaling.
func (b apiContentBlock) MarshalJSON() ([]byte, error) {
	if b.rawOverride != nil {
		return b.rawOverride, nil
	}
	type alias apiContentBlock
	return json.Marshal(alias(b))
}

type apiTool struct {
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	InputSchema  json.RawMessage  `json:"input_schema"`
	CacheControl *apiCacheControl `json:"cache_control,omitempty"`
	DeferLoading bool             `json:"defer_loading,omitempty"`
}

// apiBuiltinTool represents an Anthropic server-side built-in tool like
// tool_search_tool_regex_20251119. Different shape from apiTool (has type,
// no input_schema). Both coexist in apiRequest.Tools via []any.
type apiBuiltinTool struct {
	Type string `json:"type"` // e.g., "tool_search_tool_regex_20251119"
	Name string `json:"name"` // e.g., "tool_search_tool_regex"
}

// --- API response types ---

type apiResponse struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Role       string            `json:"role"`
	Content    []json.RawMessage `json:"content"` // parsed per-block in parseResponse
	Model      string            `json:"model"`
	StopReason string            `json:"stop_reason"`
	Usage      apiUsage          `json:"usage"`
}

type apiUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// buildRequest converts gollem messages into an Anthropic API request.
// If enableCache is true, cache_control markers are added to the last system
// block and last tool definition to enable Anthropic's prompt caching.
func buildRequest(messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters, model string, defaultMaxTokens int, stream bool, enableCache bool, disableToolSearch bool) (*apiRequest, error) {
	req := &apiRequest{
		Model:     model,
		MaxTokens: defaultMaxTokens,
		Stream:    stream,
	}

	if settings != nil {
		if settings.MaxTokens != nil {
			req.MaxTokens = *settings.MaxTokens
		}
		req.Temperature = settings.Temperature
		req.TopP = settings.TopP

		// Enable extended thinking if ThinkingBudget is set.
		if settings.ThinkingBudget != nil && *settings.ThinkingBudget > 0 {
			req.Thinking = &apiThinking{
				Type:         "enabled",
				BudgetTokens: *settings.ThinkingBudget,
			}
			// Anthropic requires max_tokens > budget_tokens. Auto-adjust if the
			// caller didn't set MaxTokens high enough. Without this, the API
			// returns 400 when ThinkingBudget is set but MaxTokens is left at
			// the default (4096) or any value <= budget_tokens.
			if req.MaxTokens <= *settings.ThinkingBudget {
				req.MaxTokens = *settings.ThinkingBudget + 16000
			}
			// Anthropic requires temperature to be omitted when thinking is enabled.
			req.Temperature = nil
		}
	}

	// Convert tool definitions. If any tool has DeferLoading=true and the
	// model supports tool search, auto-inject the built-in tool_search
	// regex tool and emit defer_loading on the wire.
	if params != nil {
		allTools := params.AllToolDefs()
		modelSupports := supportsToolSearch(model)

		// Check if any tool requests deferred loading.
		anyDeferred := false
		for _, td := range allTools {
			if td.DeferLoading {
				anyDeferred = true
				break
			}
		}

		// Auto-inject the built-in tool_search tool when applicable.
		if anyDeferred && modelSupports && !disableToolSearch {
			req.Tools = append(req.Tools, apiBuiltinTool{
				Type: toolSearchToolRegexType,
				Name: toolSearchToolRegexName,
			})
		}

		for _, td := range allTools {
			schemaJSON, err := json.Marshal(td.ParametersSchema)
			if err != nil {
				return nil, err
			}
			at := apiTool{
				Name:        td.Name,
				Description: td.Description,
				InputSchema: schemaJSON,
			}
			// Only emit defer_loading on models that support it. Silent
			// degrade on unsupported models: the flag is preserved in core
			// but never reaches the wire, so the tool ships inline.
			if td.DeferLoading && modelSupports {
				at.DeferLoading = true
			}
			req.Tools = append(req.Tools, at)
		}
	}

	// Apply tool choice from settings.
	if settings != nil && settings.ToolChoice != nil {
		tc := settings.ToolChoice
		switch {
		case tc.Mode == "none":
			// Anthropic doesn't have a "none" tool_choice type;
			// the way to prevent tool use is to omit tools entirely.
			req.Tools = nil
		case tc.Mode == "required":
			req.ToolChoice = &apiToolChoice{Type: "any"}
		case tc.ToolName != "":
			req.ToolChoice = &apiToolChoice{Type: "tool", Name: tc.ToolName}
		case tc.Mode == "auto":
			req.ToolChoice = &apiToolChoice{Type: "auto"}
		}
	}

	// Convert messages.
	var systemBlocks []apiSystemBlock
	var apiMsgs []apiMessage

	for _, msg := range messages {
		switch m := msg.(type) {
		case core.ModelRequest:
			var userBlocks []apiContentBlock
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.SystemPromptPart:
					systemBlocks = append(systemBlocks, apiSystemBlock{
						Type: "text",
						Text: p.Content,
					})
				case core.UserPromptPart:
					userBlocks = append(userBlocks, apiContentBlock{
						Type: "text",
						Text: p.Content,
					})
				case core.ToolReturnPart:
					content := ""
					switch v := p.Content.(type) {
					case string:
						content = v
					default:
						b, _ := json.Marshal(v)
						content = string(b)
					}
					userBlocks = append(userBlocks, apiContentBlock{
						Type:      "tool_result",
						ToolUseID: p.ToolCallID,
						Content:   content,
					})
				case core.RetryPromptPart:
					if p.ToolCallID != "" {
						userBlocks = append(userBlocks, apiContentBlock{
							Type:      "tool_result",
							ToolUseID: p.ToolCallID,
							Content:   p.Content,
							IsError:   true,
						})
					} else {
						userBlocks = append(userBlocks, apiContentBlock{
							Type: "text",
							Text: p.Content,
						})
					}
				default:
					return nil, fmt.Errorf("anthropic provider: unsupported request part type %T", part)
				}
			}
			if len(userBlocks) > 0 {
				apiMsgs = append(apiMsgs, apiMessage{
					Role:    "user",
					Content: userBlocks,
				})
			} else if len(m.Parts) > 0 {
				// The ModelRequest contained only SystemPromptParts (extracted
				// above to the top-level system field). Emit a placeholder user
				// message to prevent consecutive assistant messages which violate
				// Anthropic's alternation requirement and cause 400 errors.
				apiMsgs = append(apiMsgs, apiMessage{
					Role:    "user",
					Content: []apiContentBlock{{Type: "text", Text: "[system context updated]"}},
				})
			}

		case core.ModelResponse:
			var assistantBlocks []apiContentBlock
			for _, part := range m.Parts {
				switch p := part.(type) {
				case core.TextPart:
					assistantBlocks = append(assistantBlocks, apiContentBlock{
						Type: "text",
						Text: p.Content,
					})
				case core.ToolCallPart:
					assistantBlocks = append(assistantBlocks, apiContentBlock{
						Type:  "tool_use",
						ID:    p.ToolCallID,
						Name:  p.ToolName,
						Input: json.RawMessage(p.ArgsJSON),
					})
				case core.ThinkingPart:
					assistantBlocks = append(assistantBlocks, apiContentBlock{
						Type:      "thinking",
						Thinking:  p.Content,
						Signature: p.Signature,
					})
				case core.ProviderMetadataPart:
					if p.Provider == "anthropic" {
						assistantBlocks = append(assistantBlocks, apiContentBlock{
							rawOverride: p.Payload,
						})
					}
				}
			}
			// Always emit an assistant message for a ModelResponse to maintain
			// proper user/assistant alternation. If the response had no content
			// (e.g., an empty response that triggered a retry), use a minimal
			// text block. Without this, adjacent user messages would cause a
			// 400 error from the Anthropic API.
			if len(assistantBlocks) == 0 {
				assistantBlocks = []apiContentBlock{{Type: "text", Text: ""}}
			}
			apiMsgs = append(apiMsgs, apiMessage{
				Role:    "assistant",
				Content: assistantBlocks,
			})
		}
	}

	req.System = systemBlocks
	req.Messages = apiMsgs

	// Apply cache_control markers to the last system block and last
	// non-deferred user tool. This enables Anthropic's prompt caching for
	// the stable prefix (system instructions + tool definitions).
	//
	// Walk backwards through Tools skipping:
	//   - apiBuiltinTool entries (tool_search_tool_regex at position 0)
	//   - apiTool entries with DeferLoading=true (Anthropic rejects
	//     cache_control + defer_loading on the same tool)
	if enableCache {
		ephemeral := &apiCacheControl{Type: "ephemeral"}
		if len(req.System) > 0 {
			req.System[len(req.System)-1].CacheControl = ephemeral
		}
		for i := len(req.Tools) - 1; i >= 0; i-- {
			if t, ok := req.Tools[i].(apiTool); ok && !t.DeferLoading {
				t.CacheControl = ephemeral
				req.Tools[i] = t
				break
			}
		}
	}

	return req, nil
}

// parseResponse converts an Anthropic API response to a core.ModelResponse.
// Known content block types (text, tool_use, thinking) are parsed into typed
// gollem parts. Unknown types (server_tool_use, tool_search_tool_result,
// tool_reference, and any future server tool types) are preserved as
// ProviderMetadataPart for lossless round-tripping in conversation history.
func parseResponse(resp *apiResponse, modelName string) *core.ModelResponse {
	var parts []core.ModelResponsePart

	for _, raw := range resp.Content {
		var typeOnly struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &typeOnly); err != nil {
			continue
		}

		switch typeOnly.Type {
		case "text":
			var block apiContentBlock
			if err := json.Unmarshal(raw, &block); err != nil {
				continue
			}
			parts = append(parts, core.TextPart{Content: block.Text})

		case "tool_use":
			var block apiContentBlock
			if err := json.Unmarshal(raw, &block); err != nil {
				continue
			}
			argsJSON := "{}"
			if block.Input != nil {
				argsJSON = string(block.Input)
			}
			parts = append(parts, core.ToolCallPart{
				ToolName:   block.Name,
				ArgsJSON:   argsJSON,
				ToolCallID: block.ID,
			})

		case "thinking":
			var block apiContentBlock
			if err := json.Unmarshal(raw, &block); err != nil {
				continue
			}
			parts = append(parts, core.ThinkingPart{
				Content:   block.Thinking,
				Signature: block.Signature,
			})

		default:
			// Preserve ALL unknown content blocks for round-tripping.
			// The Anthropic API contract requires assistant content blocks
			// to be sent back in conversation history exactly as received.
			// This covers server_tool_use, tool_search_tool_result,
			// tool_reference, and any future server tool types.
			parts = append(parts, core.ProviderMetadataPart{
				Provider: "anthropic",
				Kind:     typeOnly.Type,
				Payload:  append(json.RawMessage(nil), raw...),
			})
		}
	}

	return &core.ModelResponse{
		Parts:        parts,
		Usage:        mapUsage(resp.Usage),
		ModelName:    modelName,
		FinishReason: mapStopReason(resp.StopReason),
		Timestamp:    time.Now(),
	}
}

// mapStopReason maps Anthropic stop reasons to gollem FinishReasons.
func mapStopReason(reason string) core.FinishReason {
	switch reason {
	case "end_turn", "stop_sequence":
		return core.FinishReasonStop
	case "max_tokens":
		return core.FinishReasonLength
	case "tool_use":
		return core.FinishReasonToolCall
	case "refusal":
		return core.FinishReasonContentFilter
	default:
		return core.FinishReasonStop
	}
}

// mapUsage converts Anthropic usage to gollem Usage.
func mapUsage(u apiUsage) core.Usage {
	return core.Usage{
		// InputTokens is the total prompt token count including cached tokens,
		// normalized to match OpenAI semantics. Anthropic's API reports non-cached
		// tokens separately, so we sum all three categories.
		InputTokens:      u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens,
		OutputTokens:     u.OutputTokens,
		CacheWriteTokens: u.CacheCreationInputTokens,
		CacheReadTokens:  u.CacheReadInputTokens,
	}
}
