package gollem

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// EndStrategy determines when to stop after finding a final result.
type EndStrategy string

const (
	// EndStrategyEarly stops at the first valid result, skipping remaining tool calls.
	EndStrategyEarly EndStrategy = "early"
	// EndStrategyExhaustive processes all tool calls even after finding a result.
	EndStrategyExhaustive EndStrategy = "exhaustive"
)

// Agent is the central type for running LLM interactions with structured output.
type Agent[T any] struct {
	model            Model
	systemPrompts    []string
	tools            []Tool
	outputSchema     *OutputSchema
	outputValidators []OutputValidatorFunc[T]
	outputOpts       []OutputOption
	maxRetries       int
	endStrategy      EndStrategy
	usageLimits      UsageLimits
	modelSettings    *ModelSettings
}

// RunResult is the outcome of a successful agent run.
type RunResult[T any] struct {
	// Output is the parsed/validated output of type T.
	Output T
	// Messages is the full conversation history.
	Messages []ModelMessage
	// Usage is the aggregate token usage for this run.
	Usage RunUsage
	// RunID is the unique identifier for this run.
	RunID string
}

// AgentOption configures the agent via functional options.
type AgentOption[T any] func(*Agent[T])

// WithSystemPrompt adds a system prompt to the agent.
func WithSystemPrompt[T any](prompt string) AgentOption[T] {
	return func(a *Agent[T]) {
		a.systemPrompts = append(a.systemPrompts, prompt)
	}
}

// WithTools adds tools to the agent.
func WithTools[T any](tools ...Tool) AgentOption[T] {
	return func(a *Agent[T]) {
		a.tools = append(a.tools, tools...)
	}
}

// WithMaxRetries sets the maximum number of result validation retries.
func WithMaxRetries[T any](n int) AgentOption[T] {
	return func(a *Agent[T]) {
		a.maxRetries = n
	}
}

// WithEndStrategy sets the end strategy for the agent.
func WithEndStrategy[T any](s EndStrategy) AgentOption[T] {
	return func(a *Agent[T]) {
		a.endStrategy = s
	}
}

// WithUsageLimits sets the usage limits for the agent.
func WithUsageLimits[T any](l UsageLimits) AgentOption[T] {
	return func(a *Agent[T]) {
		a.usageLimits = l
	}
}

// WithModelSettings sets the model settings for the agent.
func WithModelSettings[T any](s ModelSettings) AgentOption[T] {
	return func(a *Agent[T]) {
		a.modelSettings = &s
	}
}

// WithTemperature sets the temperature for the agent.
func WithTemperature[T any](t float64) AgentOption[T] {
	return func(a *Agent[T]) {
		if a.modelSettings == nil {
			a.modelSettings = &ModelSettings{}
		}
		a.modelSettings.Temperature = &t
	}
}

// WithMaxTokens sets the max tokens for the agent.
func WithMaxTokens[T any](n int) AgentOption[T] {
	return func(a *Agent[T]) {
		if a.modelSettings == nil {
			a.modelSettings = &ModelSettings{}
		}
		a.modelSettings.MaxTokens = &n
	}
}

// WithOutputValidator adds an output validator to the agent.
func WithOutputValidator[T any](fn OutputValidatorFunc[T]) AgentOption[T] {
	return func(a *Agent[T]) {
		a.outputValidators = append(a.outputValidators, fn)
	}
}

// WithOutputOptions sets output configuration options.
func WithOutputOptions[T any](opts ...OutputOption) AgentOption[T] {
	return func(a *Agent[T]) {
		a.outputOpts = append(a.outputOpts, opts...)
	}
}

// NewAgent creates a new Agent with the given model and options.
func NewAgent[T any](model Model, opts ...AgentOption[T]) *Agent[T] {
	a := &Agent[T]{
		model:       model,
		maxRetries:  1,
		endStrategy: EndStrategyEarly,
		usageLimits: DefaultUsageLimits(),
	}
	for _, opt := range opts {
		opt(a)
	}
	// Build output schema lazily on first run if not already set.
	return a
}

// RunOption configures a specific run invocation.
type RunOption func(*runConfig)

type runConfig struct {
	deps          any
	modelSettings *ModelSettings
	usageLimits   *UsageLimits
	messages      []ModelMessage
}

// WithRunDeps sets dependencies available to tools via RunContext.
func WithRunDeps(deps any) RunOption {
	return func(c *runConfig) {
		c.deps = deps
	}
}

// WithRunModelSettings overrides model settings for this run.
func WithRunModelSettings(s ModelSettings) RunOption {
	return func(c *runConfig) {
		c.modelSettings = &s
	}
}

// WithRunUsageLimits overrides usage limits for this run.
func WithRunUsageLimits(l UsageLimits) RunOption {
	return func(c *runConfig) {
		c.usageLimits = &l
	}
}

// WithMessages provides conversation history to continue from.
func WithMessages(msgs ...ModelMessage) RunOption {
	return func(c *runConfig) {
		c.messages = msgs
	}
}

// agentRunState tracks mutable state across the agent loop.
type agentRunState struct {
	messages    []ModelMessage
	usage       RunUsage
	retries     int
	toolRetries map[string]int
	runStep     int
	runID       string
	mu          sync.Mutex // protects usage and toolRetries during concurrent tool execution
}

// Run executes the agent loop synchronously and returns the final result.
func (a *Agent[T]) Run(ctx context.Context, prompt string, opts ...RunOption) (*RunResult[T], error) {
	cfg := &runConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build output schema.
	if a.outputSchema == nil {
		a.outputSchema = buildOutputSchema[T](a.outputOpts...)
	}

	// Initialize state.
	state := &agentRunState{
		toolRetries: make(map[string]int),
		runID:       newRunID(),
	}

	// Copy any provided history.
	if len(cfg.messages) > 0 {
		state.messages = make([]ModelMessage, len(cfg.messages))
		copy(state.messages, cfg.messages)
	}

	// Resolve settings.
	settings := a.modelSettings
	if cfg.modelSettings != nil {
		settings = cfg.modelSettings
	}
	limits := a.usageLimits
	if cfg.usageLimits != nil {
		limits = *cfg.usageLimits
	}

	return a.runLoop(ctx, state, prompt, settings, limits, cfg.deps)
}

// RunStream executes the agent with streaming output.
func (a *Agent[T]) RunStream(ctx context.Context, prompt string, opts ...RunOption) (*StreamResult[T], error) {
	cfg := &runConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	// Build output schema.
	if a.outputSchema == nil {
		a.outputSchema = buildOutputSchema[T](a.outputOpts...)
	}

	// Initialize state.
	state := &agentRunState{
		toolRetries: make(map[string]int),
		runID:       newRunID(),
	}
	if len(cfg.messages) > 0 {
		state.messages = make([]ModelMessage, len(cfg.messages))
		copy(state.messages, cfg.messages)
	}

	settings := a.modelSettings
	if cfg.modelSettings != nil {
		settings = cfg.modelSettings
	}

	// Build the initial request.
	req := a.buildInitialRequest(prompt)
	state.messages = append(state.messages, req)

	// Build model request parameters.
	params := buildModelRequestParams(a.tools, a.outputSchema)

	// Make streaming request.
	stream, err := a.model.RequestStream(ctx, state.messages, settings, params)
	if err != nil {
		return nil, fmt.Errorf("model stream request failed: %w", err)
	}

	return newStreamResult[T](stream, a.outputSchema, a.outputValidators, state.messages), nil
}

// runLoop is the core agent loop.
func (a *Agent[T]) runLoop(ctx context.Context, state *agentRunState, prompt string, settings *ModelSettings, limits UsageLimits, deps any) (*RunResult[T], error) {
	// Build the initial request.
	req := a.buildInitialRequest(prompt)
	state.messages = append(state.messages, req)

	// Build model request parameters.
	params := buildModelRequestParams(a.tools, a.outputSchema)

	// Build tool lookup map.
	toolMap := make(map[string]*Tool)
	for i := range a.tools {
		toolMap[a.tools[i].Definition.Name] = &a.tools[i]
	}

	// Build output tool name set.
	outputToolNames := make(map[string]bool)
	for _, ot := range a.outputSchema.OutputTools {
		outputToolNames[ot.Name] = true
	}

	for {
		// Check context.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// Check usage limits before request.
		if err := limits.CheckBeforeRequest(state.usage); err != nil {
			return nil, err
		}

		state.runStep++

		// Call the model.
		resp, err := a.model.Request(ctx, state.messages, settings, params)
		if err != nil {
			return nil, fmt.Errorf("model request failed: %w", err)
		}

		// Track usage.
		state.usage.IncrRequest(resp.Usage)

		// Append response to history.
		state.messages = append(state.messages, *resp)

		// Check token limits.
		if limits.HasTokenLimits() {
			if err := limits.CheckTokens(state.usage); err != nil {
				return nil, err
			}
		}

		// Process the response.
		result, nextParts, err := a.processResponse(ctx, state, resp, toolMap, outputToolNames, deps, prompt)
		if err != nil {
			return nil, err
		}
		if result != nil {
			return &RunResult[T]{
				Output:   result.output,
				Messages: state.messages,
				Usage:    state.usage,
				RunID:    state.runID,
			}, nil
		}

		// No final result yet — append tool results and continue.
		if len(nextParts) > 0 {
			nextReq := ModelRequest{
				Parts:     nextParts,
				Timestamp: time.Now(),
			}
			state.messages = append(state.messages, nextReq)
		}
	}
}

type finalResult[T any] struct {
	output     T
	toolName   string
	toolCallID string
}

// processResponse handles a model response: executes tool calls or extracts final result.
//
//nolint:cyclop
func (a *Agent[T]) processResponse(
	ctx context.Context,
	state *agentRunState,
	resp *ModelResponse,
	toolMap map[string]*Tool,
	outputToolNames map[string]bool,
	deps any,
	prompt string,
) (*finalResult[T], []ModelRequestPart, error) {
	toolCalls := resp.ToolCalls()
	hasText := resp.TextContent() != ""

	// If no tool calls and text is allowed, try text output.
	if len(toolCalls) == 0 && hasText && a.outputSchema.AllowsText {
		text := resp.TextContent()
		output, err := deserializeOutput[T](text, a.outputSchema.OuterTypedDictKey)
		if err != nil {
			// For text mode with T=string, the text IS the output.
			// Try direct assignment.
			if a.outputSchema.Mode == OutputModeText {
				// T should be string in text mode.
				output = any(text).(T)
			} else {
				return nil, nil, fmt.Errorf("failed to parse text output: %w", err)
			}
		}

		// Validate output.
		rc := &RunContext{
			Deps:    deps,
			Usage:   state.usage,
			Prompt:  prompt,
			RunStep: state.runStep,
			RunID:   state.runID,
		}
		output, err = validateOutput(ctx, rc, output, a.outputValidators)
		if err != nil {
			var retryErr *ModelRetryError
			if errors.As(err, &retryErr) {
				if retryErr := incrementRetries(&state.retries, a.maxRetries, state.messages); retryErr != nil {
					return nil, nil, retryErr
				}
				part := buildRetryParts(retryErr.Message, "", "")
				return nil, []ModelRequestPart{part}, nil
			}
			return nil, nil, err
		}

		return &finalResult[T]{output: output}, nil, nil
	}

	// If no tool calls and no valid text, handle empty response.
	if len(toolCalls) == 0 {
		if resp.FinishReason == FinishReasonLength {
			return nil, nil, &UnexpectedModelBehavior{
				Message: "model response ended due to token limit without producing a result",
			}
		}
		if resp.FinishReason == FinishReasonContentFilter {
			return nil, nil, &ContentFilterError{
				UnexpectedModelBehavior: UnexpectedModelBehavior{
					Message: "content filter triggered",
				},
			}
		}
		// Retry on empty response.
		if retryErr := incrementRetries(&state.retries, a.maxRetries, state.messages); retryErr != nil {
			return nil, nil, retryErr
		}
		part := buildRetryParts("empty response, please provide a result", "", "")
		return nil, []ModelRequestPart{part}, nil
	}

	// Process tool calls.
	var resultParts []ModelRequestPart
	var result *finalResult[T]

	// Separate output tool calls from function tool calls.
	var outputCalls []ToolCallPart
	var functionCalls []ToolCallPart
	var unknownCalls []ToolCallPart

	for _, tc := range toolCalls {
		if outputToolNames[tc.ToolName] {
			outputCalls = append(outputCalls, tc)
		} else if _, ok := toolMap[tc.ToolName]; ok {
			functionCalls = append(functionCalls, tc)
		} else {
			unknownCalls = append(unknownCalls, tc)
		}
	}

	// Handle unknown tools.
	for _, tc := range unknownCalls {
		if retryErr := incrementRetries(&state.retries, a.maxRetries, state.messages); retryErr != nil {
			return nil, nil, retryErr
		}
		part := buildRetryParts(
			fmt.Sprintf("unknown tool %q, available tools: %s", tc.ToolName, a.availableToolNames()),
			tc.ToolName,
			tc.ToolCallID,
		)
		resultParts = append(resultParts, part)
	}

	// Handle output tool calls.
	for _, tc := range outputCalls {
		output, err := deserializeOutput[T](tc.ArgsJSON, a.outputSchema.OuterTypedDictKey)
		if err != nil {
			if retryErr := incrementRetries(&state.retries, a.maxRetries, state.messages); retryErr != nil {
				return nil, nil, retryErr
			}
			part := buildRetryParts(
				"failed to parse output: "+err.Error(),
				tc.ToolName,
				tc.ToolCallID,
			)
			resultParts = append(resultParts, part)
			continue
		}

		// Validate output.
		rc := &RunContext{
			Deps:       deps,
			Usage:      state.usage,
			Prompt:     prompt,
			ToolName:   tc.ToolName,
			ToolCallID: tc.ToolCallID,
			RunStep:    state.runStep,
			RunID:      state.runID,
		}
		output, err = validateOutput(ctx, rc, output, a.outputValidators)
		if err != nil {
			var retryErr *ModelRetryError
			if errors.As(err, &retryErr) {
				if incErr := incrementRetries(&state.retries, a.maxRetries, state.messages); incErr != nil {
					return nil, nil, incErr
				}
				part := buildRetryParts(retryErr.Message, tc.ToolName, tc.ToolCallID)
				resultParts = append(resultParts, part)
				continue
			}
			return nil, nil, err
		}

		if result == nil {
			result = &finalResult[T]{
				output:     output,
				toolName:   tc.ToolName,
				toolCallID: tc.ToolCallID,
			}
			if a.endStrategy == EndStrategyEarly {
				// Return tool result part and skip remaining.
				resultParts = append(resultParts, ToolReturnPart{
					ToolName:   tc.ToolName,
					Content:    "output accepted",
					ToolCallID: tc.ToolCallID,
					Timestamp:  time.Now(),
				})
				// Skip remaining function calls in early mode.
				return result, resultParts, nil
			}
		}
	}

	// Execute function tool calls.
	if len(functionCalls) > 0 {
		funcParts, err := a.executeFunctionTools(ctx, state, functionCalls, toolMap, deps, prompt)
		if err != nil {
			return nil, nil, err
		}
		resultParts = append(resultParts, funcParts...)
	}

	// If we found a result in exhaustive mode, return it.
	if result != nil {
		return result, resultParts, nil
	}

	return nil, resultParts, nil
}

// executeFunctionTools executes function tool calls, running non-sequential
// tools concurrently.
func (a *Agent[T]) executeFunctionTools(
	ctx context.Context,
	state *agentRunState,
	calls []ToolCallPart,
	toolMap map[string]*Tool,
	deps any,
	prompt string,
) ([]ModelRequestPart, error) {
	// Separate sequential and concurrent calls.
	type indexedCall struct {
		idx  int
		call ToolCallPart
		tool *Tool
	}
	var sequentialCalls []indexedCall
	var concurrentCalls []indexedCall

	for i, tc := range calls {
		tool := toolMap[tc.ToolName]
		if tool.Definition.Sequential {
			sequentialCalls = append(sequentialCalls, indexedCall{idx: i, call: tc, tool: tool})
		} else {
			concurrentCalls = append(concurrentCalls, indexedCall{idx: i, call: tc, tool: tool})
		}
	}

	results := make([]ModelRequestPart, len(calls))

	// Execute concurrent tools.
	if len(concurrentCalls) > 0 {
		var wg sync.WaitGroup
		var mu sync.Mutex

		for _, ic := range concurrentCalls {
			wg.Add(1)
			go func(ic indexedCall) {
				defer wg.Done()
				part := a.executeSingleTool(ctx, state, ic.call, ic.tool, deps, prompt)
				mu.Lock()
				results[ic.idx] = part
				mu.Unlock()
			}(ic)
		}
		wg.Wait()
	}

	// Execute sequential tools.
	for _, ic := range sequentialCalls {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		part := a.executeSingleTool(ctx, state, ic.call, ic.tool, deps, prompt)
		results[ic.idx] = part
	}

	return results, nil
}

// executeSingleTool executes a single tool call and returns the result part.
func (a *Agent[T]) executeSingleTool(
	ctx context.Context,
	state *agentRunState,
	call ToolCallPart,
	tool *Tool,
	deps any,
	prompt string,
) ModelRequestPart {
	// Lock state for reading/writing shared fields.
	state.mu.Lock()
	state.usage.IncrToolCall()

	// Determine max retries for this tool.
	maxRetries := a.maxRetries
	if tool.MaxRetries != nil {
		maxRetries = *tool.MaxRetries
	}

	rc := &RunContext{
		Deps:       deps,
		Usage:      state.usage,
		Prompt:     prompt,
		Retry:      state.toolRetries[call.ToolName],
		MaxRetries: maxRetries,
		ToolName:   call.ToolName,
		ToolCallID: call.ToolCallID,
		Messages:   state.messages,
		RunStep:    state.runStep,
		RunID:      state.runID,
	}
	state.mu.Unlock()

	result, err := tool.Handler(ctx, rc, call.ArgsJSON)
	if err != nil {
		// Check for ModelRetryError.
		var retryErr *ModelRetryError
		if errors.As(err, &retryErr) {
			state.mu.Lock()
			state.toolRetries[call.ToolName]++
			retryCount := state.toolRetries[call.ToolName]
			state.mu.Unlock()
			if retryCount > maxRetries {
				return RetryPromptPart{
					Content:    fmt.Sprintf("tool %q exceeded maximum retries (%d): %s", call.ToolName, maxRetries, retryErr.Message),
					ToolName:   call.ToolName,
					ToolCallID: call.ToolCallID,
					Timestamp:  time.Now(),
				}
			}
			return RetryPromptPart{
				Content:    retryErr.Message,
				ToolName:   call.ToolName,
				ToolCallID: call.ToolCallID,
				Timestamp:  time.Now(),
			}
		}

		// Other errors become tool return with error content.
		return ToolReturnPart{
			ToolName:   call.ToolName,
			Content:    "error: " + err.Error(),
			ToolCallID: call.ToolCallID,
			Timestamp:  time.Now(),
		}
	}

	// Serialize result.
	content, err := serializeToolResult(result)
	if err != nil {
		return ToolReturnPart{
			ToolName:   call.ToolName,
			Content:    "error serializing result: " + err.Error(),
			ToolCallID: call.ToolCallID,
			Timestamp:  time.Now(),
		}
	}

	return ToolReturnPart{
		ToolName:   call.ToolName,
		Content:    content,
		ToolCallID: call.ToolCallID,
		Timestamp:  time.Now(),
	}
}

// serializeToolResult converts a tool result to a string.
func serializeToolResult(result any) (string, error) {
	if result == nil {
		return "", nil
	}
	if s, ok := result.(string); ok {
		return s, nil
	}
	b, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// buildInitialRequest constructs the initial ModelRequest with system prompts and user prompt.
func (a *Agent[T]) buildInitialRequest(prompt string) ModelRequest {
	var parts []ModelRequestPart

	// Add system prompts.
	for _, sp := range a.systemPrompts {
		parts = append(parts, SystemPromptPart{
			Content:   sp,
			Timestamp: time.Now(),
		})
	}

	// Add user prompt.
	parts = append(parts, UserPromptPart{
		Content:   prompt,
		Timestamp: time.Now(),
	})

	return ModelRequest{
		Parts:     parts,
		Timestamp: time.Now(),
	}
}

// newRunID generates a random run identifier.
func newRunID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// availableToolNames returns a formatted string of available tool names.
func (a *Agent[T]) availableToolNames() string {
	var names []string
	for _, t := range a.tools {
		names = append(names, t.Definition.Name)
	}
	for _, ot := range a.outputSchema.OutputTools {
		names = append(names, ot.Name)
	}
	if len(names) == 0 {
		return "(none)"
	}
	result := names[0]
	for _, n := range names[1:] {
		result += ", " + n
	}
	return result
}
