package deep

import (
	"context"
	"testing"

	"github.com/fugue-labs/gollem/core"
)

func TestPlanningTool_CreatePlan(t *testing.T) {
	tool := PlanningTool()

	argsJSON := `{
		"command": "create",
		"tasks": [
			{"id": "1", "description": "First task", "status": "pending"},
			{"id": "2", "description": "Second task", "status": "pending"}
		]
	}`

	result, err := tool.Handler(context.Background(), &core.RunContext{}, argsJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["status"] != "created" {
		t.Errorf("expected status 'created', got %v", m["status"])
	}
	if m["tasks"] != 2 {
		t.Errorf("expected 2 tasks, got %v", m["tasks"])
	}
}

func TestPlanningTool_UpdateTask(t *testing.T) {
	tool := PlanningTool()

	// Create plan first.
	createArgs := `{
		"command": "create",
		"tasks": [
			{"id": "t1", "description": "Task one", "status": "pending"},
			{"id": "t2", "description": "Task two", "status": "pending"}
		]
	}`
	_, err := tool.Handler(context.Background(), &core.RunContext{}, createArgs)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Update task status.
	updateArgs := `{"command": "update", "task_id": "t1", "status": "in_progress", "notes": "Working on it"}`
	result, err := tool.Handler(context.Background(), &core.RunContext{}, updateArgs)
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if m["status"] != "updated" {
		t.Errorf("expected status 'updated', got %v", m["status"])
	}
	task, ok := m["task"].(PlanTask)
	if !ok {
		t.Fatalf("expected PlanTask, got %T", m["task"])
	}
	if task.Status != "in_progress" {
		t.Errorf("expected task status 'in_progress', got %q", task.Status)
	}
	if task.Notes != "Working on it" {
		t.Errorf("expected notes 'Working on it', got %q", task.Notes)
	}
}

func TestPlanningTool_GetPlan(t *testing.T) {
	tool := PlanningTool()

	// Create plan.
	createArgs := `{
		"command": "create",
		"tasks": [
			{"id": "x1", "description": "Do X", "status": "pending"},
			{"id": "x2", "description": "Do Y", "status": "completed"}
		]
	}`
	_, err := tool.Handler(context.Background(), &core.RunContext{}, createArgs)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Get plan.
	getArgs := `{"command": "get"}`
	result, err := tool.Handler(context.Background(), &core.RunContext{}, getArgs)
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	tasks, ok := m["tasks"].([]PlanTask)
	if !ok {
		t.Fatalf("expected []PlanTask, got %T", m["tasks"])
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
	progress, ok := m["progress"].(string)
	if !ok {
		t.Fatalf("expected progress string, got %T", m["progress"])
	}
	if progress != "1/2 completed" {
		t.Errorf("expected '1/2 completed', got %q", progress)
	}
}

func TestPlanningTool_UpdateNotFound(t *testing.T) {
	tool := PlanningTool()

	// Create plan.
	_, err := tool.Handler(context.Background(), &core.RunContext{}, `{
		"command": "create",
		"tasks": [{"id": "a", "description": "A", "status": "pending"}]
	}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Try to update non-existent task.
	_, err = tool.Handler(context.Background(), &core.RunContext{}, `{"command": "update", "task_id": "z", "status": "done"}`)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestPlanningTool_UnknownCommand(t *testing.T) {
	tool := PlanningTool()
	_, err := tool.Handler(context.Background(), &core.RunContext{}, `{"command": "explode"}`)
	if err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestPlanningTool_AddTasks(t *testing.T) {
	tool := PlanningTool()

	// Create initial plan.
	_, err := tool.Handler(context.Background(), &core.RunContext{}, `{
		"command": "create",
		"tasks": [{"id": "1", "description": "First", "status": "pending"}]
	}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Add via tasks list.
	result, err := tool.Handler(context.Background(), &core.RunContext{}, `{
		"command": "add",
		"tasks": [{"id": "2", "description": "Second", "status": "pending"}]
	}`)
	if err != nil {
		t.Fatalf("add tasks: %v", err)
	}
	m := result.(map[string]any)
	if m["total"] != 2 {
		t.Errorf("expected 2 total, got %v", m["total"])
	}

	// Add via task_id + description.
	result, err = tool.Handler(context.Background(), &core.RunContext{}, `{
		"command": "add",
		"task_id": "3",
		"description": "Third task"
	}`)
	if err != nil {
		t.Fatalf("add single: %v", err)
	}
	m = result.(map[string]any)
	if m["total"] != 3 {
		t.Errorf("expected 3 total, got %v", m["total"])
	}

	// Verify via get.
	result, err = tool.Handler(context.Background(), &core.RunContext{}, `{"command": "get"}`)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	tasks := result.(map[string]any)["tasks"].([]PlanTask)
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(tasks))
	}
}

func TestPlanningTool_DeleteTask(t *testing.T) {
	tool := PlanningTool()

	// Create plan.
	_, err := tool.Handler(context.Background(), &core.RunContext{}, `{
		"command": "create",
		"tasks": [
			{"id": "a", "description": "A", "status": "pending"},
			{"id": "b", "description": "B", "status": "pending"},
			{"id": "c", "description": "C", "status": "pending"}
		]
	}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Delete middle task.
	result, err := tool.Handler(context.Background(), &core.RunContext{}, `{"command": "delete", "task_id": "b"}`)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	m := result.(map[string]any)
	if m["remaining"] != 2 {
		t.Errorf("expected 2 remaining, got %v", m["remaining"])
	}

	// Verify remaining tasks.
	result, err = tool.Handler(context.Background(), &core.RunContext{}, `{"command": "get"}`)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	tasks := result.(map[string]any)["tasks"].([]PlanTask)
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
	if tasks[0].ID != "a" || tasks[1].ID != "c" {
		t.Errorf("expected tasks a,c but got %s,%s", tasks[0].ID, tasks[1].ID)
	}

	// Delete non-existent task.
	_, err = tool.Handler(context.Background(), &core.RunContext{}, `{"command": "delete", "task_id": "z"}`)
	if err == nil {
		t.Fatal("expected error for non-existent task")
	}
}

func TestPlanningTool_Integration(t *testing.T) {
	// Simulate: model creates a plan, marks task in_progress, completes it.
	tool := PlanningTool()

	model := core.NewTestModel(
		core.ToolCallResponseWithID("planning", `{
			"command": "create",
			"tasks": [
				{"id": "step1", "description": "Research topic", "status": "pending"},
				{"id": "step2", "description": "Write draft", "status": "pending"}
			]
		}`, "tc1"),
		core.ToolCallResponseWithID("planning", `{"command": "update", "task_id": "step1", "status": "completed"}`, "tc2"),
		core.TextResponse("Done with planning and execution."),
	)

	agent := core.NewAgent[string](model, core.WithTools[string](tool))
	result, err := agent.Run(context.Background(), "Do the task")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "Done with planning and execution." {
		t.Errorf("unexpected output: %s", result.Output)
	}
}
