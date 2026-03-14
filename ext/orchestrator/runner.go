package orchestrator

import (
	"context"
	"time"

	"github.com/fugue-labs/gollem/core"
)

// Runner executes a claimed task run and returns its terminal result.
type Runner interface {
	RunTask(ctx context.Context, claim *ClaimedTask) (*TaskResult, error)
}

// RunnerFunc adapts a function into a Runner.
type RunnerFunc func(ctx context.Context, claim *ClaimedTask) (*TaskResult, error)

// RunTask implements Runner.
func (fn RunnerFunc) RunTask(ctx context.Context, claim *ClaimedTask) (*TaskResult, error) {
	return fn(ctx, claim)
}

// AgentRunnerOption customizes an in-process agent runner.
type AgentRunnerOption[T any] func(*AgentRunner[T])

// AgentRunner executes a core.Agent in-process for each claimed task.
type AgentRunner[T any] struct {
	agent      *core.Agent[T]
	promptFn   func(*Task) (string, error)
	runOptsFn  func(*Task, RunRef) []core.RunOption
	resultMeta func(*core.RunResult[T]) map[string]any
}

// NewAgentRunner wraps a core.Agent with the Runner interface.
func NewAgentRunner[T any](agent *core.Agent[T], opts ...AgentRunnerOption[T]) *AgentRunner[T] {
	if agent == nil {
		panic("gollem/orchestrator: agent runner requires a non-nil agent")
	}
	runner := &AgentRunner[T]{
		agent: agent,
		promptFn: func(task *Task) (string, error) {
			if task == nil {
				return "", nil
			}
			return task.Input, nil
		},
	}
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

// WithTaskPrompt customizes how task input becomes an agent prompt.
func WithTaskPrompt[T any](fn func(*Task) (string, error)) AgentRunnerOption[T] {
	return func(r *AgentRunner[T]) {
		r.promptFn = fn
	}
}

// WithTaskRunOptions appends per-task run options when invoking the wrapped agent.
func WithTaskRunOptions[T any](fn func(*Task, RunRef) []core.RunOption) AgentRunnerOption[T] {
	return func(r *AgentRunner[T]) {
		r.runOptsFn = fn
	}
}

// WithTaskResultMetadata stores additional result metadata from the wrapped run.
func WithTaskResultMetadata[T any](fn func(*core.RunResult[T]) map[string]any) AgentRunnerOption[T] {
	return func(r *AgentRunner[T]) {
		r.resultMeta = fn
	}
}

// RunTask implements Runner.
func (r *AgentRunner[T]) RunTask(ctx context.Context, claim *ClaimedTask) (*TaskResult, error) {
	task := (*Task)(nil)
	run := RunRef{}
	if claim != nil {
		task = claim.Task
		if claim.Run != nil {
			run = *claim.Run
			if run.ID != "" {
				ctx = core.ContextWithRunID(ctx, run.ID)
			}
		}
	}

	prompt, err := r.promptFn(task)
	if err != nil {
		return nil, err
	}

	var opts []core.RunOption
	if r.runOptsFn != nil {
		opts = append(opts, r.runOptsFn(task, run)...)
	}

	result, err := r.agent.Run(ctx, prompt, opts...)
	if err != nil {
		return nil, err
	}

	taskResult := &TaskResult{
		RunnerRunID: result.RunID,
		Output:      result.Output,
		Usage:       result.Usage,
		ToolState:   cloneAnyMap(result.ToolState),
		CompletedAt: time.Now(),
	}
	if r.resultMeta != nil {
		taskResult.Metadata = cloneAnyMap(r.resultMeta(result))
	}
	return taskResult, nil
}
