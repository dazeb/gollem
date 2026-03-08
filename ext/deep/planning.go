package deep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

const planningToolName = "planning"

// PlanFromToolState decodes the exported planning tool state snapshot.
func PlanFromToolState(state map[string]any) (Plan, bool) {
	if len(state) == 0 {
		return Plan{}, false
	}
	raw, ok := state[planningToolName]
	if !ok {
		return Plan{}, false
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return Plan{}, false
	}
	rawTasks, ok := m["tasks"]
	if !ok {
		return Plan{}, true
	}
	b, err := json.Marshal(rawTasks)
	if err != nil {
		return Plan{}, false
	}
	var tasks []PlanTask
	if err := json.Unmarshal(b, &tasks); err != nil {
		return Plan{}, false
	}
	return Plan{Tasks: tasks}, true
}

// CurrentPlan returns the currently exported planning state for this run context.
func CurrentPlan(rc *core.RunContext) (Plan, bool) {
	if rc == nil {
		return Plan{}, false
	}
	return PlanFromToolState(rc.ToolState())
}

// Plan represents the agent's current plan.
type Plan struct {
	Tasks []PlanTask `json:"tasks"`
}

// PlanTask represents a single task in the plan.
type PlanTask struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"` // "pending", "in_progress", "completed", "blocked"
	Notes       string `json:"notes,omitempty"`
}

// planCommand is the input schema for the planning tool.
type planCommand struct {
	Command     string     `json:"command" jsonschema:"description=The command to execute: create, add, update, delete, or get"`
	Tasks       []PlanTask `json:"tasks,omitempty" jsonschema:"description=Tasks for create/add command"`
	TaskID      string     `json:"task_id,omitempty" jsonschema:"description=Task ID for update/delete command"`
	Status      string     `json:"status,omitempty" jsonschema:"description=New status for update command"`
	Notes       string     `json:"notes,omitempty" jsonschema:"description=Notes for update command"`
	Description string     `json:"description,omitempty" jsonschema:"description=Description for add command (when adding a single task)"`
}

// planState holds the current plan, shared across tool calls within a run.
type planState struct {
	mu   sync.Mutex
	plan Plan
}

func (s *planState) ExportState() (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]PlanTask, len(s.plan.Tasks))
	copy(cp, s.plan.Tasks)
	return map[string]any{"tasks": cp}, nil
}

func (s *planState) RestoreState(state any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	m, ok := state.(map[string]any)
	if !ok {
		return errors.New("invalid planning state")
	}
	if raw, ok := m["tasks"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("marshal planning state tasks: %w", err)
		}
		var tasks []PlanTask
		if err := json.Unmarshal(b, &tasks); err != nil {
			return fmt.Errorf("unmarshal planning state tasks: %w", err)
		}
		s.plan.Tasks = tasks
	}
	return nil
}

// PlanningTool creates a tool that maintains a persistent todo list.
// The model uses this tool to plan, track progress, and manage tasks.
func PlanningTool() core.Tool {
	state := &planState{}

	tool := core.FuncTool[planCommand](
		"planning",
		"Manage a persistent todo list. Commands: 'create' (create a new plan with tasks), "+
			"'add' (add tasks to existing plan), 'update' (update a task's status or notes), "+
			"'delete' (remove a task), 'get' (retrieve the current plan with progress).",
		func(_ context.Context, cmd planCommand) (any, error) {
			return executePlanCommand(state, cmd)
		},
	)
	tool.Stateful = state
	return tool
}

func executePlanCommand(state *planState, cmd planCommand) (any, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	switch cmd.Command {
	case "create":
		if len(cmd.Tasks) == 0 {
			return nil, errors.New("create command requires at least one task")
		}
		state.plan = Plan{Tasks: cmd.Tasks}
		return map[string]any{
			"status":  "created",
			"tasks":   len(state.plan.Tasks),
			"plan_id": "plan_1",
		}, nil

	case "add":
		// Add tasks to the existing plan. Accepts either a list of tasks
		// or a single task via task_id + description.
		if len(cmd.Tasks) > 0 {
			state.plan.Tasks = append(state.plan.Tasks, cmd.Tasks...)
			return map[string]any{
				"status": "added",
				"added":  len(cmd.Tasks),
				"total":  len(state.plan.Tasks),
			}, nil
		}
		if cmd.TaskID != "" && cmd.Description != "" {
			task := PlanTask{
				ID:          cmd.TaskID,
				Description: cmd.Description,
				Status:      "pending",
			}
			state.plan.Tasks = append(state.plan.Tasks, task)
			return map[string]any{
				"status": "added",
				"task":   task,
				"total":  len(state.plan.Tasks),
			}, nil
		}
		return nil, errors.New("add command requires either tasks list or task_id + description")

	case "update":
		if cmd.TaskID == "" {
			return nil, errors.New("update command requires task_id")
		}
		for i := range state.plan.Tasks {
			if state.plan.Tasks[i].ID == cmd.TaskID {
				if cmd.Status != "" {
					state.plan.Tasks[i].Status = cmd.Status
				}
				if cmd.Notes != "" {
					state.plan.Tasks[i].Notes = cmd.Notes
				}
				return map[string]any{
					"status": "updated",
					"task":   state.plan.Tasks[i],
				}, nil
			}
		}
		return nil, fmt.Errorf("task %q not found", cmd.TaskID)

	case "delete":
		if cmd.TaskID == "" {
			return nil, errors.New("delete command requires task_id")
		}
		for i := range state.plan.Tasks {
			if state.plan.Tasks[i].ID == cmd.TaskID {
				state.plan.Tasks = append(state.plan.Tasks[:i], state.plan.Tasks[i+1:]...)
				return map[string]any{
					"status":    "deleted",
					"task_id":   cmd.TaskID,
					"remaining": len(state.plan.Tasks),
				}, nil
			}
		}
		return nil, fmt.Errorf("task %q not found", cmd.TaskID)

	case "get":
		// Count completed vs total for progress tracking.
		completed := 0
		inProgress := 0
		for _, t := range state.plan.Tasks {
			switch t.Status {
			case "completed":
				completed++
			case "in_progress":
				inProgress++
			}
		}
		total := len(state.plan.Tasks)
		result := map[string]any{
			"tasks": state.plan.Tasks,
		}
		if total > 0 {
			result["progress"] = fmt.Sprintf("%d/%d completed", completed, total)
			if inProgress > 0 {
				result["in_progress"] = inProgress
			}
		}
		return result, nil

	default:
		return nil, fmt.Errorf("unknown command %q (use create, add, update, delete, or get)", cmd.Command)
	}
}
