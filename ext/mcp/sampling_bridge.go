package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

const defaultSamplingMaxTokens = 4096

// SamplingRequester can issue MCP sampling requests.
type SamplingRequester interface {
	CreateMessage(context.Context, *CreateMessageParams) (*CreateMessageResult, error)
}

// MCPModel exposes MCP sampling as a gollem core.Model.
type MCPModel struct {
	requester SamplingRequester
	config    mcpModelConfig

	mu        sync.Mutex
	modelName string
}

type mcpModelConfig struct {
	modelName        string
	modelPreferences *ModelPreferences
	includeContext   string
	metadata         map[string]any
}

// MCPModelOption configures an MCPModel.
type MCPModelOption func(*mcpModelConfig)

// WithMCPModelName sets the display name returned by ModelName before the first response.
func WithMCPModelName(name string) MCPModelOption {
	return func(cfg *mcpModelConfig) {
		cfg.modelName = name
	}
}

// WithMCPModelPreferences sets default model preferences for sampling requests.
func WithMCPModelPreferences(prefs ModelPreferences) MCPModelOption {
	return func(cfg *mcpModelConfig) {
		copied := prefs
		cfg.modelPreferences = &copied
	}
}

// WithMCPIncludeContext sets CreateMessageParams.includeContext for every request.
func WithMCPIncludeContext(includeContext string) MCPModelOption {
	return func(cfg *mcpModelConfig) {
		cfg.includeContext = includeContext
	}
}

// WithMCPMetadata sets metadata included on every sampling request.
func WithMCPMetadata(metadata map[string]any) MCPModelOption {
	return func(cfg *mcpModelConfig) {
		cfg.metadata = cloneAnyMap(metadata)
	}
}

// NewMCPModel constructs a core.Model backed by MCP sampling/createMessage.
func NewMCPModel(requester SamplingRequester, opts ...MCPModelOption) *MCPModel {
	cfg := mcpModelConfig{
		modelName: "mcp-sampling",
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return &MCPModel{
		requester: requester,
		config:    cfg,
		modelName: cfg.modelName,
	}
}

// ModelSamplingHandler adapts a gollem core.Model into an MCP SamplingHandler.
func ModelSamplingHandler(model core.Model) SamplingHandler {
	handler := SamplingHandler(func(ctx context.Context, req *CreateMessageParams) (*CreateMessageResult, error) {
		messages, settings, params, err := coreRequestFromSampling(req)
		if err != nil {
			return nil, err
		}
		resp, err := model.Request(ctx, messages, settings, params)
		if err != nil {
			return nil, err
		}
		return createMessageResultFromCore(resp)
	})
	return registerSamplingCapabilities(handler, &ClientSamplingCapability{
		Tools: &EmptyCapability{},
	})
}

// Request implements core.Model.
func (m *MCPModel) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	if m == nil || m.requester == nil {
		return nil, errors.New("mcp: no sampling requester configured")
	}

	req, err := createMessageRequestFromCore(messages, settings, params, m.config)
	if err != nil {
		return nil, err
	}
	result, err := m.requester.CreateMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := coreResponseFromCreateMessage(result)
	if err != nil {
		return nil, err
	}

	if result != nil && result.Model != "" {
		m.mu.Lock()
		m.modelName = result.Model
		m.mu.Unlock()
		resp.ModelName = result.Model
	}

	return resp, nil
}

// RequestStream implements core.Model.
func (m *MCPModel) RequestStream(context.Context, []core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return nil, errors.New("mcp: streaming sampling is not implemented")
}

// ModelName implements core.Model.
func (m *MCPModel) ModelName() string {
	if m == nil {
		return "mcp-sampling"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.modelName
}

func createMessageRequestFromCore(messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters, cfg mcpModelConfig) (*CreateMessageParams, error) {
	samplingMessages, systemPrompt, err := samplingMessagesFromCore(messages)
	if err != nil {
		return nil, err
	}

	req := &CreateMessageParams{
		Messages:         samplingMessages,
		SystemPrompt:     systemPrompt,
		MaxTokens:        defaultSamplingMaxTokens,
		Metadata:         cloneAnyMap(cfg.metadata),
		ModelPreferences: cfg.modelPreferences,
	}
	if cfg.includeContext != "" {
		req.IncludeContext = cfg.includeContext
	}

	if settings != nil {
		if settings.MaxTokens != nil && *settings.MaxTokens > 0 {
			req.MaxTokens = *settings.MaxTokens
		}
		if settings.Temperature != nil {
			temperature := *settings.Temperature
			req.Temperature = &temperature
		}
	}

	if params != nil {
		req.Tools = samplingToolsFromCore(params.AllToolDefs())
		req.ToolChoice = samplingToolChoiceFromCore(settings, params)
		if settings != nil && settings.ToolChoice != nil && settings.ToolChoice.ToolName != "" {
			req.Tools = restrictSamplingTools(req.Tools, settings.ToolChoice.ToolName)
		}
	}

	return req, nil
}

func coreRequestFromSampling(req *CreateMessageParams) ([]core.ModelMessage, *core.ModelSettings, *core.ModelRequestParameters, error) {
	if req == nil {
		return nil, nil, nil, errors.New("mcp: nil sampling request")
	}

	var messages []core.ModelMessage
	if strings.TrimSpace(req.SystemPrompt) != "" {
		messages = append(messages, core.ModelRequest{
			Parts: []core.ModelRequestPart{
				core.SystemPromptPart{Content: req.SystemPrompt},
			},
		})
	}

	for _, msg := range req.Messages {
		converted, err := coreMessageFromSampling(msg)
		if err != nil {
			return nil, nil, nil, err
		}
		if converted != nil {
			messages = append(messages, converted)
		}
	}

	settings := &core.ModelSettings{}
	hasSettings := false
	if req.MaxTokens > 0 {
		maxTokens := req.MaxTokens
		settings.MaxTokens = &maxTokens
		hasSettings = true
	}
	if req.Temperature != nil {
		temperature := *req.Temperature
		settings.Temperature = &temperature
		hasSettings = true
	}
	if req.ToolChoice != nil && req.ToolChoice.Mode != "" {
		settings.ToolChoice = &core.ToolChoice{Mode: req.ToolChoice.Mode}
		hasSettings = true
	}
	if !hasSettings {
		settings = nil
	}

	modelParams := &core.ModelRequestParameters{
		FunctionTools:   coreToolsFromSampling(req.Tools),
		AllowTextOutput: true,
	}
	if len(modelParams.FunctionTools) == 0 {
		modelParams = nil
	}

	return messages, settings, modelParams, nil
}

func samplingMessagesFromCore(messages []core.ModelMessage) ([]SamplingMessage, string, error) {
	var out []SamplingMessage
	var systemParts []string

	for _, message := range messages {
		switch msg := message.(type) {
		case core.ModelRequest:
			system, segments, err := samplingUserMessagesFromCore(msg)
			if err != nil {
				return nil, "", err
			}
			systemParts = append(systemParts, system...)
			out = append(out, segments...)
		case core.ModelResponse:
			segment, err := samplingAssistantMessageFromCore(msg)
			if err != nil {
				return nil, "", err
			}
			if segment != nil {
				out = append(out, *segment)
			}
		default:
			return nil, "", fmt.Errorf("mcp: unsupported model message type %T", message)
		}
	}

	return out, strings.Join(systemParts, "\n\n"), nil
}

func samplingUserMessagesFromCore(msg core.ModelRequest) ([]string, []SamplingMessage, error) {
	var systemParts []string
	var segments []SamplingMessage
	var regular []Content
	var toolResults []Content

	flushRegular := func() {
		if len(regular) == 0 {
			return
		}
		segments = append(segments, SamplingMessage{
			Role:    "user",
			Content: marshalSamplingBlocks(regular),
		})
		regular = nil
	}
	flushToolResults := func() {
		if len(toolResults) == 0 {
			return
		}
		segments = append(segments, SamplingMessage{
			Role:    "user",
			Content: marshalSamplingBlocks(toolResults),
		})
		toolResults = nil
	}

	for _, part := range msg.Parts {
		switch p := part.(type) {
		case core.SystemPromptPart:
			if strings.TrimSpace(p.Content) != "" {
				systemParts = append(systemParts, p.Content)
			}
		case core.UserPromptPart:
			flushToolResults()
			regular = append(regular, Content{Type: "text", Text: p.Content})
		case core.ImagePart:
			flushToolResults()
			regular = append(regular, Content{Type: "image", URI: p.URL, MIMEType: p.MIMEType})
		case core.AudioPart:
			flushToolResults()
			regular = append(regular, Content{Type: "audio", URI: p.URL, MIMEType: p.MIMEType})
		case core.DocumentPart:
			flushToolResults()
			regular = append(regular, Content{
				Type: "resource",
				Resource: &ResourceContents{
					URI:      p.URL,
					MIMEType: p.MIMEType,
				},
			})
		case core.ToolReturnPart:
			flushRegular()
			content, err := toolResultContentFromCore(p)
			if err != nil {
				return nil, nil, err
			}
			toolResults = append(toolResults, content)
		case core.RetryPromptPart:
			flushToolResults()
			regular = append(regular, Content{
				Type: "text",
				Text: retryPromptText(p),
			})
		default:
			return nil, nil, fmt.Errorf("mcp: unsupported request part type %T", part)
		}
	}

	flushRegular()
	flushToolResults()
	return systemParts, segments, nil
}

func samplingAssistantMessageFromCore(msg core.ModelResponse) (*SamplingMessage, error) {
	var blocks []Content
	for _, part := range msg.Parts {
		switch p := part.(type) {
		case core.TextPart:
			blocks = append(blocks, Content{Type: "text", Text: p.Content})
		case core.ToolCallPart:
			input := json.RawMessage(`{}`)
			if strings.TrimSpace(p.ArgsJSON) != "" {
				input = json.RawMessage(p.ArgsJSON)
			}
			blocks = append(blocks, Content{
				Type:  "tool_use",
				Name:  p.ToolName,
				ID:    p.ToolCallID,
				Input: input,
			})
		case core.ThinkingPart:
			// Do not surface provider-specific hidden reasoning through MCP sampling.
		default:
			return nil, fmt.Errorf("mcp: unsupported response part type %T", part)
		}
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	return &SamplingMessage{
		Role:    "assistant",
		Content: marshalSamplingBlocks(blocks),
	}, nil
}

func createMessageResultFromCore(resp *core.ModelResponse) (*CreateMessageResult, error) {
	if resp == nil {
		return nil, errors.New("mcp: nil core response")
	}

	segment, err := samplingAssistantMessageFromCore(*resp)
	if err != nil {
		return nil, err
	}
	content := json.RawMessage(`null`)
	if segment != nil {
		content = segment.Content
	}

	return &CreateMessageResult{
		Role:       "assistant",
		Content:    content,
		Model:      resp.ModelName,
		StopReason: stopReasonFromCore(resp.FinishReason),
	}, nil
}

func coreResponseFromCreateMessage(result *CreateMessageResult) (*core.ModelResponse, error) {
	if result == nil {
		return nil, errors.New("mcp: nil createMessage result")
	}
	parts, err := coreResponsePartsFromSampling(result.Content)
	if err != nil {
		return nil, err
	}
	return &core.ModelResponse{
		Parts:        parts,
		ModelName:    result.Model,
		FinishReason: finishReasonFromSampling(result.StopReason),
	}, nil
}

func coreMessageFromSampling(msg SamplingMessage) (core.ModelMessage, error) {
	blocks, err := ParseSamplingContent(msg.Content)
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to parse sampling content: %w", err)
	}

	switch msg.Role {
	case "user":
		parts, err := coreRequestPartsFromSampling(blocks)
		if err != nil {
			return nil, err
		}
		return core.ModelRequest{Parts: parts}, nil
	case "assistant":
		parts, err := coreResponsePartsFromSampling(msg.Content)
		if err != nil {
			return nil, err
		}
		return core.ModelResponse{Parts: parts}, nil
	default:
		return nil, fmt.Errorf("mcp: unsupported sampling role %q", msg.Role)
	}
}

func coreRequestPartsFromSampling(blocks []Content) ([]core.ModelRequestPart, error) {
	parts := make([]core.ModelRequestPart, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "", "text":
			parts = append(parts, core.UserPromptPart{Content: block.Text})
		case "image":
			parts = append(parts, core.ImagePart{URL: block.URI, MIMEType: block.MIMEType})
		case "audio":
			parts = append(parts, core.AudioPart{URL: block.URI, MIMEType: block.MIMEType})
		case "resource":
			if block.Resource != nil {
				parts = append(parts, core.DocumentPart{
					URL:      block.Resource.URI,
					MIMEType: block.Resource.MIMEType,
				})
				continue
			}
			parts = append(parts, core.DocumentPart{URL: block.URI, MIMEType: block.MIMEType})
		case "tool_result":
			part, err := coreToolReturnFromSampling(block)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
		default:
			return nil, fmt.Errorf("mcp: unsupported user content block type %q", block.Type)
		}
	}
	return parts, nil
}

func coreResponsePartsFromSampling(raw json.RawMessage) ([]core.ModelResponsePart, error) {
	blocks, err := ParseSamplingContent(raw)
	if err != nil {
		return nil, fmt.Errorf("mcp: failed to parse assistant sampling content: %w", err)
	}
	parts := make([]core.ModelResponsePart, 0, len(blocks))
	for _, block := range blocks {
		switch block.Type {
		case "", "text":
			parts = append(parts, core.TextPart{Content: block.Text})
		case "tool_use":
			argsJSON := "{}"
			if len(block.Input) > 0 {
				argsJSON = string(block.Input)
			}
			parts = append(parts, core.ToolCallPart{
				ToolName:   block.Name,
				ToolCallID: block.ID,
				ArgsJSON:   argsJSON,
			})
		default:
			return nil, fmt.Errorf("mcp: unsupported assistant content block type %q", block.Type)
		}
	}
	return parts, nil
}

func toolResultContentFromCore(part core.ToolReturnPart) (Content, error) {
	blocks, structured, err := toolResultPayloadFromCore(part.Content, part.Images)
	if err != nil {
		return Content{}, err
	}
	return Content{
		Type:              "tool_result",
		ToolUseID:         part.ToolCallID,
		Content:           blocks,
		StructuredContent: structured,
	}, nil
}

func toolResultPayloadFromCore(value any, images []core.ImagePart) ([]Content, any, error) {
	var blocks []Content
	var structured any

	switch v := value.(type) {
	case nil:
	case string:
		if v != "" {
			blocks = append(blocks, Content{Type: "text", Text: v})
		}
	case json.RawMessage:
		if len(v) > 0 {
			var decoded any
			if err := json.Unmarshal(v, &decoded); err == nil {
				structured = decoded
			} else {
				blocks = append(blocks, Content{Type: "text", Text: string(v)})
			}
		}
	default:
		structured = v
	}

	for _, img := range images {
		blocks = append(blocks, Content{
			Type:     "image",
			URI:      img.URL,
			MIMEType: img.MIMEType,
		})
	}

	if structured != nil && len(blocks) == 0 {
		blocks = append(blocks, Content{
			Type: "text",
			Text: stringifyContentFallback(structured),
		})
	}

	return blocks, structured, nil
}

func coreToolReturnFromSampling(block Content) (core.ToolReturnPart, error) {
	part := core.ToolReturnPart{
		ToolCallID: block.ToolUseID,
	}

	var images []core.ImagePart
	for _, item := range block.Content {
		switch item.Type {
		case "", "text":
			if part.Content == nil {
				part.Content = item.Text
			} else {
				part.Content = strings.TrimSpace(fmt.Sprintf("%v\n%s", part.Content, item.Text))
			}
		case "image":
			images = append(images, core.ImagePart{
				URL:      item.URI,
				MIMEType: item.MIMEType,
			})
		case "audio", "resource":
			// Preserve readable fallback when possible.
			if text := item.textContent(); text != "" {
				if part.Content == nil {
					part.Content = text
				} else {
					part.Content = strings.TrimSpace(fmt.Sprintf("%v\n%s", part.Content, text))
				}
			}
		default:
			return core.ToolReturnPart{}, fmt.Errorf("mcp: unsupported tool_result inner block type %q", item.Type)
		}
	}

	if block.StructuredContent != nil {
		part.Content = block.StructuredContent
		if len(images) == 0 {
			return part, nil
		}
	}

	if len(images) > 0 {
		part.Images = images
	}

	if part.Content == nil {
		part.Content = ""
	}
	return part, nil
}

func samplingToolsFromCore(defs []core.ToolDefinition) []SamplingTool {
	if len(defs) == 0 {
		return nil
	}
	tools := make([]SamplingTool, 0, len(defs))
	for _, def := range defs {
		raw, _ := json.Marshal(def.ParametersSchema)
		tools = append(tools, SamplingTool{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: raw,
		})
	}
	return tools
}

func coreToolsFromSampling(tools []SamplingTool) []core.ToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	defs := make([]core.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		defs = append(defs, core.ToolDefinition{
			Name:             tool.Name,
			Description:      tool.Description,
			ParametersSchema: decodeToolSchema(tool.InputSchema),
			Kind:             core.ToolKindFunction,
		})
	}
	return defs
}

func restrictSamplingTools(tools []SamplingTool, name string) []SamplingTool {
	if len(tools) == 0 || name == "" {
		return tools
	}
	filtered := make([]SamplingTool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == name {
			filtered = append(filtered, tool)
		}
	}
	if len(filtered) == 0 {
		return tools
	}
	return filtered
}

func samplingToolChoiceFromCore(settings *core.ModelSettings, params *core.ModelRequestParameters) *SamplingToolChoice {
	if settings != nil && settings.ToolChoice != nil {
		choice := *settings.ToolChoice
		if choice.ToolName != "" {
			return &SamplingToolChoice{Mode: "required"}
		}
		if choice.Mode != "" {
			return &SamplingToolChoice{Mode: choice.Mode}
		}
	}
	if params != nil && params.OutputMode == core.OutputModeTool && len(params.OutputTools) > 0 && !params.AllowTextOutput {
		return &SamplingToolChoice{Mode: "required"}
	}
	return nil
}

func stopReasonFromCore(reason core.FinishReason) string {
	switch reason {
	case core.FinishReasonToolCall:
		return "toolUse"
	case core.FinishReasonLength:
		return "maxToken"
	case core.FinishReasonStop, core.FinishReasonContentFilter, core.FinishReasonError:
		return "endTurn"
	default:
		return string(reason)
	}
}

func finishReasonFromSampling(reason string) core.FinishReason {
	switch reason {
	case "toolUse", "tool_use":
		return core.FinishReasonToolCall
	case "maxToken", "max_tokens", "length":
		return core.FinishReasonLength
	case "", "endTurn", "end_turn", "stopSequence", "stop_sequence":
		return core.FinishReasonStop
	default:
		return core.FinishReasonStop
	}
}

func retryPromptText(part core.RetryPromptPart) string {
	if part.ToolName == "" {
		return part.Content
	}
	return fmt.Sprintf("Retry tool %s: %s", part.ToolName, part.Content)
}

func marshalSamplingBlocks(blocks []Content) json.RawMessage {
	switch len(blocks) {
	case 0:
		return nil
	case 1:
		return MarshalSamplingContent(blocks[0])
	default:
		return MarshalSamplingContentArray(blocks)
	}
}
