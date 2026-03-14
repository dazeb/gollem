package core

import (
	"context"
	"errors"
	"io"
)

// AgentRun represents an in-progress agent execution that can be iterated step-by-step.
type AgentRun[T any] struct {
	agent  *Agent[T]
	ctx    context.Context
	state  *RunState
	deps   any
	prompt string
	engine *turnEngine[T]
	done   bool
	result *RunResult[T]
	err    error
	ended  bool
}

// Iter starts an agent run that can be iterated step-by-step.
func (a *Agent[T]) Iter(ctx context.Context, prompt string, opts ...RunOption) *AgentRun[T] {
	cfg := &runConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	a.ensureOutputSchema()

	exec := a.initializeRunExecution(ctx, cfg)
	state := exec.state
	settings := exec.settings
	limits := exec.limits
	deps := exec.deps

	var err error
	prompt, err = a.applyInputGuardrails(ctx, state, deps, prompt, false)
	if err != nil {
		return &AgentRun[T]{
			done: true,
			err:  err,
		}
	}

	if err := a.bootstrapRunMessages(ctx, state, prompt, deps, cfg.deferredResults, cfg.initialRequestParts); err != nil {
		return &AgentRun[T]{
			done: true,
			err:  err,
		}
	}

	ctx = a.beginRun(ctx, state, deps, prompt)

	return &AgentRun[T]{
		agent:  a,
		ctx:    ctx,
		state:  state,
		deps:   deps,
		prompt: prompt,
		engine: a.newTurnEngine(ctx, state, prompt, settings, limits, deps),
	}
}

func (ar *AgentRun[T]) finish(runErr error) {
	if ar == nil || ar.ended || ar.agent == nil || ar.state == nil {
		return
	}
	ar.ended = true
	ar.agent.endRun(ar.ctx, ar.state, ar.deps, ar.prompt, runErr)
}

// Next executes one iteration of the agent loop (one model call + tool execution).
// Returns the ModelResponse for that step, or (nil, io.EOF) when done.
func (ar *AgentRun[T]) Next() (*ModelResponse, error) {
	if ar.done {
		if ar.err != nil {
			return nil, ar.err
		}
		return nil, io.EOF
	}

	resp, result, err := ar.engine.Step()
	if err != nil {
		ar.done = true
		ar.err = err
		ar.finish(err)
		return resp, err
	}
	if result != nil {
		ar.done = true
		ar.result = result
		ar.finish(nil)
	}
	return resp, nil
}

// Result returns the final result after iteration completes. Returns error if not done.
func (ar *AgentRun[T]) Result() (*RunResult[T], error) {
	if ar.err != nil {
		return nil, ar.err
	}
	if !ar.done {
		return nil, errors.New("agent run not yet complete")
	}
	if ar.result == nil {
		return nil, errors.New("agent run completed without a result")
	}
	return ar.result, nil
}

// Messages returns the current message history.
func (ar *AgentRun[T]) Messages() []ModelMessage {
	if ar.engine == nil || ar.engine.state == nil {
		return nil
	}
	return ar.engine.state.messages
}

// Done returns true if the agent run has completed.
func (ar *AgentRun[T]) Done() bool {
	return ar.done
}
