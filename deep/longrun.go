package deep

import (
	"context"

	"github.com/trevorprater/gollem"
)

// LongRunAgent wraps a gollem.Agent with deep capabilities for long-running tasks.
type LongRunAgent[T any] struct {
	model          gollem.Model
	agentOpts      []gollem.AgentOption[T]
	contextManager *ContextManager
	planningTool   *gollem.Tool
}

// LongRunOption configures the long-running agent.
type LongRunOption[T any] func(*longRunConfig[T])

type longRunConfig[T any] struct {
	contextWindow    int
	planningEnabled  bool
	contextOpts      []ContextOption
	agentOpts        []gollem.AgentOption[T]
}

// WithContextWindow sets the max context window size for automatic compression.
func WithContextWindow[T any](tokens int) LongRunOption[T] {
	return func(c *longRunConfig[T]) {
		c.contextWindow = tokens
	}
}

// WithPlanningEnabled enables the built-in planning tool.
func WithPlanningEnabled[T any]() LongRunOption[T] {
	return func(c *longRunConfig[T]) {
		c.planningEnabled = true
	}
}

// WithLongRunContextOptions sets context manager options.
func WithLongRunContextOptions[T any](opts ...ContextOption) LongRunOption[T] {
	return func(c *longRunConfig[T]) {
		c.contextOpts = opts
	}
}

// WithLongRunAgentOptions passes additional agent options to the underlying agent.
func WithLongRunAgentOptions[T any](opts ...gollem.AgentOption[T]) LongRunOption[T] {
	return func(c *longRunConfig[T]) {
		c.agentOpts = opts
	}
}

// NewLongRunAgent creates an agent configured for long-running operations.
func NewLongRunAgent[T any](model gollem.Model, opts ...LongRunOption[T]) *LongRunAgent[T] {
	cfg := &longRunConfig[T]{
		contextWindow: 100000,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	lra := &LongRunAgent[T]{
		model:     model,
		agentOpts: cfg.agentOpts,
	}

	// Set up context management.
	contextOpts := append([]ContextOption{WithMaxContextTokens(cfg.contextWindow)}, cfg.contextOpts...)
	lra.contextManager = NewContextManager(model, contextOpts...)

	// Set up planning tool if enabled.
	if cfg.planningEnabled {
		pt := PlanningTool()
		lra.planningTool = &pt
	}

	return lra
}

// Run executes the long-running agent with all deep capabilities active.
func (a *LongRunAgent[T]) Run(ctx context.Context, prompt string, opts ...gollem.RunOption) (*gollem.RunResult[T], error) {
	// Build agent options.
	agentOpts := make([]gollem.AgentOption[T], 0, len(a.agentOpts)+3)
	agentOpts = append(agentOpts, a.agentOpts...)

	// Add context management as a history processor.
	agentOpts = append(agentOpts, gollem.WithHistoryProcessor[T](a.contextManager.AsHistoryProcessor()))

	// Add planning tool if enabled.
	if a.planningTool != nil {
		agentOpts = append(agentOpts, gollem.WithTools[T](*a.planningTool))
	}

	agent := gollem.NewAgent[T](a.model, agentOpts...)
	return agent.Run(ctx, prompt, opts...)
}
