package gollem

import (
	"context"
	"errors"
	"fmt"
)

// handoffStep represents a single step in a handoff pipeline.
type handoffStep[T any] struct {
	name     string
	agent    *Agent[T]
	promptFn func(prevOutput T) string
}

// Handoff runs agents in sequence, passing each agent's output to the next.
// The message history accumulates across agents.
type Handoff[T any] struct {
	steps []handoffStep[T]
}

// NewHandoff creates a multi-agent handoff pipeline.
func NewHandoff[T any]() *Handoff[T] {
	return &Handoff[T]{}
}

// AddStep adds an agent step to the pipeline.
// The promptFn generates the prompt for this agent from the previous agent's output.
func (h *Handoff[T]) AddStep(name string, agent *Agent[T], promptFn func(prevOutput T) string) *Handoff[T] {
	h.steps = append(h.steps, handoffStep[T]{
		name:     name,
		agent:    agent,
		promptFn: promptFn,
	})
	return h
}

// Run executes the pipeline with the initial prompt.
func (h *Handoff[T]) Run(ctx context.Context, initialPrompt string) (*RunResult[T], error) {
	if len(h.steps) == 0 {
		return nil, errors.New("handoff pipeline has no steps")
	}

	var totalUsage RunUsage
	var lastResult *RunResult[T]

	for i, step := range h.steps {
		var prompt string
		if i == 0 {
			prompt = initialPrompt
		} else {
			prompt = step.promptFn(lastResult.Output)
		}

		var opts []RunOption
		if lastResult != nil {
			// Pass accumulated messages from previous steps.
			opts = append(opts, WithMessages(lastResult.Messages...))
		}

		result, err := step.agent.Run(ctx, prompt, opts...)
		if err != nil {
			return nil, fmt.Errorf("handoff step %q failed: %w", step.name, err)
		}

		totalUsage.IncrRun(result.Usage)
		lastResult = result
	}

	lastResult.Usage = totalUsage
	return lastResult, nil
}
