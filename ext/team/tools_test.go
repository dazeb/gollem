package team

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

func TestSpawnTool(t *testing.T) {
	tm := NewTeam(TeamConfig{
		Name:   "tool-test",
		Leader: "leader",
		Model:  core.NewTestModel(core.TextResponse("done")),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tool := spawnTool(tm)
	result, err := tool.Handler(ctx, nil, `{"name":"helper","task":"do something"}`)
	if err != nil {
		t.Fatal(err)
	}

	resultMap := result.(map[string]any)
	if resultMap["status"] != "spawned" {
		t.Fatalf("expected spawned status, got %v", resultMap["status"])
	}
	if resultMap["name"] != "helper" {
		t.Fatalf("expected helper name, got %v", resultMap["name"])
	}

	helper := tm.GetTeammate("helper")
	if helper == nil {
		t.Fatal("expected helper teammate to exist")
	}
	waitForState(t, helper, TeammateIdle, 3*time.Second)
}

func TestShutdownTool(t *testing.T) {
	tm := NewTeam(TeamConfig{
		Name:   "test",
		Leader: "leader",
		Model:  core.NewTestModel(core.TextResponse("done")),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := tm.SpawnTeammate(ctx, "worker", "task")
	if err != nil {
		t.Fatal(err)
	}
	worker := tm.GetTeammate("worker")
	waitForState(t, worker, TeammateIdle, 3*time.Second)

	tool := shutdownTool(tm)
	result, err := tool.Handler(ctx, nil, `{"name":"worker","reason":"all done"}`)
	if err != nil {
		t.Fatal(err)
	}
	resultMap := result.(map[string]any)
	if resultMap["status"] != "shutdown_requested" {
		t.Fatalf("expected shutdown_requested, got %v", resultMap["status"])
	}
	if resultMap["requested_by"] != "leader" {
		t.Fatalf("expected requested_by leader, got %v", resultMap["requested_by"])
	}
}

func TestTaskCreateTool(t *testing.T) {
	tm := NewTeam(TeamConfig{Name: "test", Model: core.NewTestModel(core.TextResponse("done"))})

	ctx := context.Background()
	if _, err := tm.SpawnTeammate(ctx, "worker", "initial"); err != nil {
		t.Fatal(err)
	}

	tool := taskCreateTool(tm, "leader")
	result, err := tool.Handler(ctx, nil, `{"subject":"Fix bug","description":"In auth module","assignee":"worker"}`)
	if err != nil {
		t.Fatal(err)
	}
	resultMap := result.(map[string]any)
	if resultMap["status"] != "created" {
		t.Fatalf("expected created status, got %v", resultMap["status"])
	}

	taskID := resultMap["task_id"].(string)
	task, err := tm.getTeamTask(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Subject != "Fix bug" {
		t.Fatalf("expected subject Fix bug, got %q", task.Subject)
	}
	if got := teamTaskAssignee(task); got != "worker" {
		t.Fatalf("expected assignee worker, got %q", got)
	}
}

func TestTaskCreateTool_UnknownAssignee(t *testing.T) {
	tm := NewTeam(TeamConfig{Name: "test", Model: core.NewTestModel(core.TextResponse("done"))})
	tool := taskCreateTool(tm, "leader")
	if _, err := tool.Handler(context.Background(), nil, `{"subject":"Fix bug","assignee":"nobody"}`); err == nil {
		t.Fatal("expected unknown assignee error")
	}
}

func TestTaskListAndGetTools(t *testing.T) {
	tm := NewTeam(TeamConfig{Name: "test", Model: core.NewTestModel(core.TextResponse("done"))})
	task, err := tm.createTeamTask(context.Background(), "Task A", "Desc", "", "leader")
	if err != nil {
		t.Fatal(err)
	}

	listTool := taskListTool(tm)
	listResult, err := listTool.Handler(context.Background(), nil, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	rawList := listResult.(json.RawMessage)
	var tasks []teamTaskView
	if err := json.Unmarshal(rawList, &tasks); err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || tasks[0].ID != task.ID {
		t.Fatalf("unexpected task list: %#v", tasks)
	}

	getTool := taskGetTool(tm)
	getResult, err := getTool.Handler(context.Background(), nil, `{"id":"`+task.ID+`"}`)
	if err != nil {
		t.Fatal(err)
	}
	var view teamTaskView
	if err := json.Unmarshal(getResult.(json.RawMessage), &view); err != nil {
		t.Fatal(err)
	}
	if view.ID != task.ID || view.Subject != task.Subject {
		t.Fatalf("unexpected task view: %#v", view)
	}
}

func TestTaskFailCurrentTool_FailsClaimedTask(t *testing.T) {
	unexpectedCalls := 0
	unexpectedTool := core.FuncTool[struct{}](
		"unexpected",
		"should not run after task_fail_current",
		func(_ context.Context, _ struct{}) (string, error) {
			unexpectedCalls++
			return "unexpected", nil
		},
	)

	model := core.NewTestModel(
		core.ToolCallResponseWithID("task_fail_current", `{"reason":"blocked on missing API"}`, "call_fail"),
		core.ToolCallResponseWithID("unexpected", `{}`, "call_unexpected"),
		core.TextResponse("reported failure"),
	)
	tm := NewTeam(TeamConfig{
		Name:             "test",
		Leader:           "leader",
		Model:            model,
		WorkerExtraTools: []core.Tool{unexpectedTool},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	worker, err := tm.SpawnTeammate(ctx, "worker", "do something impossible")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, worker, TeammateIdle, 3*time.Second)

	tasks, err := tm.listTeamTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Status != orchestrator.TaskFailed {
		t.Fatalf("expected failed task, got %s", tasks[0].Status)
	}
	if tasks[0].LastError != "blocked on missing API" {
		t.Fatalf("expected failure reason to be recorded, got %q", tasks[0].LastError)
	}
	if unexpectedCalls != 0 {
		t.Fatalf("expected no follow-on tool calls after task failure, got %d", unexpectedCalls)
	}
	if got := len(model.Calls()); got != 1 {
		t.Fatalf("expected current run to stop after first model response, got %d model calls", got)
	}
}

func TestLeaderTools_Count(t *testing.T) {
	tm := NewTeam(TeamConfig{Name: "test", Model: core.NewTestModel(core.TextResponse("done"))})
	tools := LeaderTools(tm)
	if len(tools) != 5 {
		t.Fatalf("expected 5 leader tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Definition.Name] = true
	}
	for _, name := range []string{"spawn_teammate", "shutdown_teammate", "task_create", "task_list", "task_get"} {
		if !names[name] {
			t.Fatalf("missing leader tool %q", name)
		}
	}
}

func TestWorkerTools_Count(t *testing.T) {
	tm := NewTeam(TeamConfig{Name: "test", Model: core.NewTestModel(core.TextResponse("done"))})
	worker := &Teammate{name: "worker"}
	tools := WorkerTools(tm, worker)
	if len(tools) != 4 {
		t.Fatalf("expected 4 worker tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Definition.Name] = true
	}
	if names["spawn_teammate"] || names["shutdown_teammate"] {
		t.Fatal("worker tools should not include team lifecycle controls")
	}
	for _, name := range []string{"task_create", "task_list", "task_get", "task_fail_current"} {
		if !names[name] {
			t.Fatalf("missing worker tool %q", name)
		}
	}
}
