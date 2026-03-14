package temporal

import (
	"context"
	"errors"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

// WorkflowClient is the subset of Temporal client functionality needed by the
// orchestrator runner adapter.
type WorkflowClient interface {
	ExecuteWorkflow(ctx context.Context, options client.StartWorkflowOptions, workflow interface{}, args ...interface{}) (client.WorkflowRun, error)
}

// WorkflowRunnerOption customizes a Temporal workflow runner.
type WorkflowRunnerOption[T any] func(*WorkflowRunner[T])

// WorkflowRunner executes orchestrator tasks through a Temporal durable workflow.
type WorkflowRunner[T any] struct {
	client         WorkflowClient
	agent          *TemporalAgent[T]
	startOptions   client.StartWorkflowOptions
	inputBuilder   func(*orchestrator.ClaimedTask) (WorkflowInput, error)
	optionsBuilder func(*orchestrator.ClaimedTask, client.StartWorkflowOptions) (client.StartWorkflowOptions, error)
	metadataFn     func(client.WorkflowRun, *WorkflowOutput, *core.RunResult[T]) map[string]any
}

// NewWorkflowRunner adapts a TemporalAgent into an orchestrator.Runner.
func NewWorkflowRunner[T any](workflowClient WorkflowClient, agent *TemporalAgent[T], taskQueue string, opts ...WorkflowRunnerOption[T]) *WorkflowRunner[T] {
	if workflowClient == nil {
		panic("gollem/temporal: workflow runner requires a non-nil client")
	}
	if agent == nil {
		panic("gollem/temporal: workflow runner requires a non-nil TemporalAgent")
	}
	if taskQueue == "" {
		panic("gollem/temporal: workflow runner requires a non-empty task queue")
	}

	runner := &WorkflowRunner[T]{
		client: workflowClient,
		agent:  agent,
		startOptions: client.StartWorkflowOptions{
			TaskQueue:                                taskQueue,
			WorkflowExecutionErrorWhenAlreadyStarted: true,
		},
		inputBuilder: func(claim *orchestrator.ClaimedTask) (WorkflowInput, error) {
			input := WorkflowInput{}
			if claim != nil && claim.Task != nil {
				input.Prompt = claim.Task.Input
			}
			if claim != nil && claim.Run != nil {
				input.ParentRunID = claim.Run.ID
			}
			return input, nil
		},
		optionsBuilder: func(claim *orchestrator.ClaimedTask, startOptions client.StartWorkflowOptions) (client.StartWorkflowOptions, error) {
			if claim != nil && claim.Run != nil && claim.Run.ID != "" && startOptions.ID == "" {
				startOptions.ID = claim.Run.ID
			}
			return startOptions, nil
		},
		metadataFn: defaultWorkflowRunnerMetadata[T],
	}
	for _, opt := range opts {
		opt(runner)
	}
	return runner
}

// WithWorkflowInputBuilder overrides how claimed tasks become WorkflowInput.
func WithWorkflowInputBuilder[T any](fn func(*orchestrator.ClaimedTask) (WorkflowInput, error)) WorkflowRunnerOption[T] {
	return func(r *WorkflowRunner[T]) {
		r.inputBuilder = fn
	}
}

// WithWorkflowStartOptions customizes base Temporal workflow start options.
func WithWorkflowStartOptions[T any](opts client.StartWorkflowOptions) WorkflowRunnerOption[T] {
	return func(r *WorkflowRunner[T]) {
		r.startOptions = mergeWorkflowStartOptions(r.startOptions, opts)
	}
}

// WithWorkflowStartOptionsBuilder customizes start options per claim.
func WithWorkflowStartOptionsBuilder[T any](fn func(*orchestrator.ClaimedTask, client.StartWorkflowOptions) (client.StartWorkflowOptions, error)) WorkflowRunnerOption[T] {
	return func(r *WorkflowRunner[T]) {
		r.optionsBuilder = fn
	}
}

// WithWorkflowResultMetadata customizes metadata stored on the orchestrator task result.
func WithWorkflowResultMetadata[T any](fn func(client.WorkflowRun, *WorkflowOutput, *core.RunResult[T]) map[string]any) WorkflowRunnerOption[T] {
	return func(r *WorkflowRunner[T]) {
		r.metadataFn = fn
	}
}

// RunTask implements orchestrator.Runner.
func (r *WorkflowRunner[T]) RunTask(ctx context.Context, claim *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
	input, err := r.inputBuilder(claim)
	if err != nil {
		return nil, err
	}

	startOptions := r.startOptions
	if r.optionsBuilder != nil {
		startOptions, err = r.optionsBuilder(claim, startOptions)
		if err != nil {
			return nil, err
		}
	}
	if startOptions.TaskQueue == "" {
		return nil, errors.New("gollem/temporal: workflow runner start options require TaskQueue")
	}

	run, err := r.client.ExecuteWorkflow(ctx, startOptions, r.agent.WorkflowName(), input)
	if err != nil {
		return nil, err
	}

	var output WorkflowOutput
	if err := run.Get(ctx, &output); err != nil {
		return nil, err
	}

	result, err := r.agent.DecodeWorkflowOutput(&output)
	if err != nil {
		return nil, err
	}

	taskResult := &orchestrator.TaskResult{
		RunnerRunID: result.RunID,
		Output:      result.Output,
		Usage:       result.Usage,
		ToolState:   cloneAnyMap(result.ToolState),
		CompletedAt: outputCompletedAt(&output, result),
	}
	if r.metadataFn != nil {
		taskResult.Metadata = cloneAnyMap(r.metadataFn(run, &output, result))
	}
	return &orchestrator.TaskOutcome{Result: taskResult}, nil
}

func defaultWorkflowRunnerMetadata[T any](run client.WorkflowRun, output *WorkflowOutput, result *core.RunResult[T]) map[string]any {
	metadata := map[string]any{
		"temporal_workflow_id": run.GetID(),
		"temporal_run_id":      run.GetRunID(),
	}
	if output != nil {
		metadata["temporal_completed"] = output.Completed
		metadata["temporal_continue_as_new_count"] = output.ContinueAsNewCount
		if output.Cost != nil {
			metadata["temporal_cost_total"] = output.Cost.TotalCost
			metadata["temporal_cost_currency"] = output.Cost.Currency
		}
	}
	if result != nil && result.Trace != nil {
		metadata["temporal_trace_steps"] = len(result.Trace.Steps)
	}
	return metadata
}

func outputCompletedAt[T any](output *WorkflowOutput, result *core.RunResult[T]) time.Time {
	if output != nil {
		if snapshot, err := decodeSerializedSnapshot(output.Snapshot, output.SnapshotJSON); err == nil && snapshot != nil && !snapshot.Timestamp.IsZero() {
			return snapshot.Timestamp
		}
	}
	if result != nil && result.Trace != nil && !result.Trace.EndTime.IsZero() {
		return result.Trace.EndTime
	}
	return time.Now()
}

func mergeWorkflowStartOptions(base, override client.StartWorkflowOptions) client.StartWorkflowOptions {
	merged := override
	if merged.TaskQueue == "" {
		merged.TaskQueue = base.TaskQueue
	}
	if !merged.WorkflowExecutionErrorWhenAlreadyStarted {
		merged.WorkflowExecutionErrorWhenAlreadyStarted = base.WorkflowExecutionErrorWhenAlreadyStarted
	}
	return merged
}
