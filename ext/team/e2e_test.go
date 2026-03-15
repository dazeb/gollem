package team

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	"github.com/fugue-labs/gollem/modelutil"
)

func TestE2E_FullTeamLifecycle(t *testing.T) {
	bus := core.NewEventBus()

	var mu sync.Mutex
	var events []string
	core.Subscribe(bus, func(e TeammateSpawnedEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, "spawned:"+e.TeammateName)
	})
	core.Subscribe(bus, func(e TeammateTerminatedEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, "terminated:"+e.TeammateName)
	})

	tm := NewTeam(TeamConfig{
		Name:     "e2e-team",
		Leader:   "leader",
		Model:    core.NewTestModel(core.TextResponse("task complete")),
		EventBus: bus,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	w1, err := tm.SpawnTeammate(ctx, "worker-1", "implement feature A")
	if err != nil {
		t.Fatal(err)
	}
	w2, err := tm.SpawnTeammate(ctx, "worker-2", "implement feature B")
	if err != nil {
		t.Fatal(err)
	}

	waitForState(t, w1, TeammateIdle, 3*time.Second)
	waitForState(t, w2, TeammateIdle, 3*time.Second)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := tm.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if !containsString(events, "spawned:worker-1") || !containsString(events, "spawned:worker-2") {
		t.Fatalf("missing spawn events: %#v", events)
	}
	if !containsString(events, "terminated:worker-1") || !containsString(events, "terminated:worker-2") {
		t.Fatalf("missing termination events: %#v", events)
	}
}

func TestE2E_AssignedFollowUpTaskRunsWithoutMailboxDispatch(t *testing.T) {
	var requests atomic.Int32
	model := &requestCountingModel{
		inner: core.NewTestModel(
			core.TextResponse("initial complete"),
			core.TextResponse("follow-up complete"),
		),
		count: &requests,
	}

	tm := NewTeam(TeamConfig{
		Name:   "assigned-team",
		Leader: "leader",
		Model:  model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worker, err := tm.SpawnTeammate(ctx, "worker", "initial task")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, worker, TeammateIdle, 3*time.Second)

	if _, err := tm.createTeamTask(context.Background(), "follow-up task", "verify the change", "worker", "leader"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		tasks, err := tm.listTeamTasks(context.Background(), orchestrator.TaskFilter{})
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) == 2 && tasks[1].Status == orchestrator.TaskCompleted {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("follow-up task was not completed: %#v", tasks)
		case <-time.After(20 * time.Millisecond):
		}
	}

	if got := requests.Load(); got < 2 {
		t.Fatalf("expected at least 2 model requests, got %d", got)
	}
}

func TestE2E_ShutdownStopsAfterCurrentTask(t *testing.T) {
	toolStarted := make(chan struct{})
	releaseTool := make(chan struct{})
	waitTool := core.FuncTool[struct{}](
		"wait",
		"Wait until released",
		func(ctx context.Context, _ struct{}) (string, error) {
			select {
			case <-toolStarted:
			default:
				close(toolStarted)
			}
			select {
			case <-releaseTool:
				return "released", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	)

	model := core.NewTestModel(
		core.ToolCallResponseWithID("wait", `{}`, "call_wait"),
		core.TextResponse("final"),
	)
	tm := NewTeam(TeamConfig{
		Name:             "shutdown-team",
		Leader:           "leader",
		Model:            model,
		WorkerExtraTools: []core.Tool{waitTool},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worker, err := tm.SpawnTeammate(ctx, "worker", "task")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-toolStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for blocking tool")
	}

	if _, err := tm.requestShutdown("worker", "leader", "all done", ""); err != nil {
		t.Fatal(err)
	}
	close(releaseTool)

	deadline := time.After(3 * time.Second)
	for worker.State() != TeammateStopped {
		select {
		case <-deadline:
			t.Fatalf("worker did not stop after shutdown, state=%v", worker.State())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestE2E_CommandStoreCancelStopsRunningTask(t *testing.T) {
	toolStarted := make(chan struct{})
	releaseTool := make(chan struct{})
	waitTool := core.FuncTool[struct{}](
		"wait",
		"Wait until released",
		func(ctx context.Context, _ struct{}) (string, error) {
			select {
			case <-toolStarted:
			default:
				close(toolStarted)
			}
			select {
			case <-releaseTool:
				return "released", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		},
	)

	model := core.NewTestModel(
		core.ToolCallResponseWithID("wait", `{}`, "call_wait"),
		core.TextResponse("final"),
	)
	tm := NewTeam(TeamConfig{
		Name:             "command-cancel-team",
		Leader:           "leader",
		Model:            model,
		WorkerExtraTools: []core.Tool{waitTool},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worker, err := tm.SpawnTeammate(ctx, "worker", "task")
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-toolStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for blocking tool")
	}

	tasks, err := tm.listTeamTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 team task, got %d", len(tasks))
	}

	if _, err := tm.CommandStore().CreateCommand(context.Background(), orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: tasks[0].ID,
		Reason: "stop this run",
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(3 * time.Second)
	for {
		task, err := tm.getTeamTask(context.Background(), tasks[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		if task.Status == orchestrator.TaskCanceled {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("task was not canceled by command: %#v", task)
		case <-time.After(20 * time.Millisecond):
		}
	}

	close(releaseTool)
	waitForState(t, worker, TeammateIdle, 3*time.Second)
}

func TestE2E_PersonalityGeneration(t *testing.T) {
	var genCalls atomic.Int32
	personalityGen := func(ctx context.Context, req modelutil.PersonalityRequest) (string, error) {
		genCalls.Add(1)
		return fmt.Sprintf("You are a specialist for: %s. Role: %s", req.Task, req.Role), nil
	}

	tm := NewTeam(TeamConfig{
		Name:                 "personality-team",
		Leader:               "leader",
		Model:                core.NewTestModel(core.TextResponse("done")),
		PersonalityGenerator: personalityGen,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worker, err := tm.SpawnTeammate(ctx, "specialist", "implement OAuth2 flow")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, worker, TeammateIdle, 3*time.Second)

	if genCalls.Load() != 1 {
		t.Fatalf("expected 1 personality generation call, got %d", genCalls.Load())
	}
}

type requestCountingModel struct {
	inner core.Model
	count *atomic.Int32
}

func (m *requestCountingModel) ModelName() string { return m.inner.ModelName() }

func (m *requestCountingModel) Request(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (*core.ModelResponse, error) {
	m.count.Add(1)
	return m.inner.Request(ctx, messages, settings, params)
}

func (m *requestCountingModel) RequestStream(ctx context.Context, messages []core.ModelMessage, settings *core.ModelSettings, params *core.ModelRequestParameters) (core.StreamedResponse, error) {
	return m.inner.RequestStream(ctx, messages, settings, params)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
