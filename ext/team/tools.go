package team

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

type spawnParams struct {
	Name string `json:"name" jsonschema:"description=A short descriptive name for the teammate (e.g. 'researcher' or 'test-runner')"`
	Task string `json:"task" jsonschema:"description=The initial task for this teammate. Include all necessary context — the teammate starts by claiming this orchestrator task."`
}

type shutdownParams struct {
	Name   string `json:"name" jsonschema:"description=Name of the teammate to shut down"`
	Reason string `json:"reason,omitempty" jsonschema:"description=Reason for shutdown"`
}

type taskCreateParams struct {
	Subject     string `json:"subject" jsonschema:"description=Brief task title in imperative form (e.g. 'Fix auth bug')"`
	Description string `json:"description,omitempty" jsonschema:"description=Detailed description of what needs to be done"`
	Assignee    string `json:"assignee,omitempty" jsonschema:"description=Optional teammate name to assign this task to. Leave empty for any teammate."`
}

type taskGetParams struct {
	ID string `json:"id" jsonschema:"description=Task ID to retrieve"`
}

type taskFailCurrentParams struct {
	Reason string `json:"reason" jsonschema:"description=Why the currently claimed task cannot be completed"`
}

// LeaderTools returns tools available only to the team leader.
func LeaderTools(t *Team) []core.Tool {
	return []core.Tool{
		spawnTool(t),
		shutdownTool(t),
		taskCreateTool(t, t.leaderSenderName()),
		taskListTool(t),
		taskGetTool(t),
	}
}

// WorkerTools returns tools available to a worker teammate.
func WorkerTools(t *Team, tm *Teammate) []core.Tool {
	return []core.Tool{
		taskCreateTool(t, tm.name),
		taskListTool(t),
		taskGetTool(t),
		taskFailCurrentTool(t, tm),
	}
}

func spawnTool(t *Team) core.Tool {
	return core.FuncTool[spawnParams](
		"spawn_teammate",
		"Spawn a new teammate agent that claims orchestrator-backed team tasks. "+
			"Use for parallel exploration, divide-and-conquer implementation, "+
			"background verification, or speculative attempts on disjoint files.",
		func(ctx context.Context, params spawnParams) (any, error) {
			if params.Name == "" {
				return nil, &core.ModelRetryError{Message: "name must not be empty"}
			}
			if params.Task == "" {
				return nil, &core.ModelRetryError{Message: "task must not be empty"}
			}

			tm, err := t.SpawnTeammate(ctx, params.Name, params.Task)
			if err != nil {
				return nil, fmt.Errorf("failed to spawn teammate: %w", err)
			}

			return map[string]any{
				"status":  "spawned",
				"name":    tm.Name(),
				"message": fmt.Sprintf("Teammate %q spawned and assigned its initial task.", tm.Name()),
			}, nil
		},
	)
}

func shutdownTool(t *Team) core.Tool {
	return core.FuncTool[shutdownParams](
		"shutdown_teammate",
		"Request a teammate to stop after its current claimed task completes.",
		func(_ context.Context, params shutdownParams) (any, error) {
			if params.Name == "" {
				return nil, &core.ModelRetryError{Message: "name must not be empty"}
			}

			reason := params.Reason
			if reason == "" {
				reason = "work complete"
			}

			req, err := t.requestShutdown(params.Name, t.leaderSenderName(), reason, "")
			if err != nil {
				return nil, err
			}

			return map[string]any{
				"status":         "shutdown_requested",
				"name":           params.Name,
				"requested_by":   req.From,
				"shutdown_id":    req.ID,
				"correlation_id": req.CorrelationID,
				"message":        fmt.Sprintf("Shutdown request sent to %q", params.Name),
			}, nil
		},
	)
}

func taskCreateTool(t *Team, selfName string) core.Tool {
	return core.FuncTool[taskCreateParams](
		"task_create",
		"Create a new orchestrator-backed team task. "+
			"Set assignee to target a specific teammate or leave it empty for any teammate.",
		func(ctx context.Context, params taskCreateParams) (any, error) {
			if params.Subject == "" {
				return nil, &core.ModelRetryError{Message: "subject must not be empty"}
			}
			task, err := t.createTeamTask(ctx, params.Subject, params.Description, params.Assignee, selfName)
			if err != nil {
				return nil, err
			}
			return map[string]any{
				"status":   "created",
				"task_id":  task.ID,
				"assignee": teamTaskAssignee(task),
			}, nil
		},
	)
}

func taskListTool(t *Team) core.Tool {
	return core.FuncTool[struct{}](
		"task_list",
		"List orchestrator-backed team tasks with status, assignee, and current run information.",
		func(ctx context.Context, _ struct{}) (any, error) {
			tasks, err := t.listTeamTasks(ctx, orchestrator.TaskFilter{})
			if err != nil {
				return nil, err
			}
			result := make([]teamTaskView, 0, len(tasks))
			for _, task := range tasks {
				result = append(result, taskView(task))
			}
			data, _ := json.Marshal(result)
			return json.RawMessage(data), nil
		},
	)
}

func taskGetTool(t *Team) core.Tool {
	return core.FuncTool[taskGetParams](
		"task_get",
		"Get full details of a specific orchestrator-backed team task by ID.",
		func(ctx context.Context, params taskGetParams) (any, error) {
			if params.ID == "" {
				return nil, &core.ModelRetryError{Message: "task ID must not be empty"}
			}
			task, err := t.getTeamTask(ctx, params.ID)
			if err != nil {
				return nil, err
			}
			data, _ := json.Marshal(taskView(task))
			return json.RawMessage(data), nil
		},
	)
}

func taskFailCurrentTool(t *Team, tm *Teammate) core.Tool {
	return core.FuncTool[taskFailCurrentParams](
		"task_fail_current",
		"Fail the task you currently have claimed. Use this when the task is blocked, impossible, or invalid.",
		func(ctx context.Context, params taskFailCurrentParams) (any, error) {
			if params.Reason == "" {
				return nil, &core.ModelRetryError{Message: "reason must not be empty"}
			}
			active, ok := tm.activeClaim()
			if !ok {
				return nil, errors.New("no active claimed task")
			}
			task, err := t.getTeamTask(ctx, active.TaskID)
			if err != nil {
				return nil, err
			}
			if task.Status != orchestrator.TaskRunning || task.Run == nil || task.Run.ID != active.RunID {
				return nil, errors.New("current task is no longer active")
			}
			persistCtx := context.WithoutCancel(ctx)
			if _, err := t.store.FailTask(persistCtx, active.TaskID, active.LeaseToken, fmt.Errorf("%s", params.Reason), time.Now()); err != nil {
				return nil, err
			}
			tm.markTaskSettled(active.TaskID)
			if !tm.abortCurrentRun(&failedCurrentTaskError{Reason: params.Reason}) {
				return nil, errors.New("current run is not active")
			}
			return map[string]any{
				"status":  "failed",
				"task_id": active.TaskID,
				"reason":  params.Reason,
			}, nil
		},
	)
}
