package core

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrAgentRunClosed is returned when an iterative run is closed before it reaches
// a terminal model result.
var ErrAgentRunClosed = errors.New("agent run closed before completion")

// AgentRun represents an in-progress agent execution that can be iterated step-by-step.
type AgentRun[T any] struct {
	agent             *Agent[T]
	ctx               context.Context
	state             *RunState
	deps              any
	prompt            string
	engine            *turnEngine[T]
	done              bool
	result            *RunResult[T]
	err               error
	started           bool
	ended             bool
	startTimeDeferred bool
}

// Iter starts an agent run that can be iterated step-by-step.
// The run lifecycle begins on the first call to Next. If the caller stops
// iterating before completion, it should call Close to end the run cleanly.
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

	return &AgentRun[T]{
		agent:             a,
		ctx:               ctx,
		state:             state,
		deps:              deps,
		prompt:            prompt,
		engine:            a.newTurnEngine(ctx, state, prompt, settings, limits, deps),
		startTimeDeferred: cfg.snapshot == nil || cfg.snapshot.RunStartTime.IsZero(),
	}
}

func (ar *AgentRun[T]) start() {
	if ar == nil || ar.started || ar.agent == nil || ar.state == nil {
		return
	}
	if ar.startTimeDeferred {
		ar.state.startTime = time.Now()
	}
	ar.ctx = ar.agent.beginRun(ar.ctx, ar.state, ar.deps, ar.prompt)
	if ar.engine != nil {
		ar.engine.ctx = ar.ctx
	}
	ar.started = true
}

func (ar *AgentRun[T]) finish(runErr error) {
	if ar == nil || ar.ended || !ar.started || ar.agent == nil || ar.state == nil {
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
	ar.start()

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

// Close ends an iterative run before completion. Closing a started run emits
// the normal run-complete lifecycle notifications with ErrAgentRunClosed.
func (ar *AgentRun[T]) Close() error {
	if ar == nil || ar.done {
		return nil
	}
	ar.done = true
	ar.err = ErrAgentRunClosed
	ar.finish(ar.err)
	return nil
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
