package team

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	omemory "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

func TestTeam_NewTeam(t *testing.T) {
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  core.NewTestModel(core.TextResponse("done")),
	})

	if tm.Name() != "test-team" {
		t.Fatalf("expected name test-team, got %q", tm.Name())
	}
	if len(tm.Members()) != 0 {
		t.Fatalf("expected no members, got %d", len(tm.Members()))
	}
	if tm.TaskStore() == nil || tm.LeaseStore() == nil || tm.CommandStore() == nil || tm.ArtifactStore() == nil {
		t.Fatal("expected orchestrator-backed stores to be exposed")
	}
}

func TestTeam_NewTeam_UsesInjectedStore(t *testing.T) {
	store := omemory.NewStore()
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  store,
	})

	if tm.Store() != store {
		t.Fatal("expected team to expose the injected orchestrator store")
	}
	if tm.TaskStore() != store || tm.LeaseStore() != store || tm.CommandStore() != store || tm.ArtifactStore() != store {
		t.Fatal("expected all team store accessors to use the injected store")
	}
}

func TestTeam_SpawnTeammate_CompletesInitialTask(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("task complete"))
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	teammate, err := tm.SpawnTeammate(ctx, "worker-1", "do something")
	if err != nil {
		t.Fatal(err)
	}

	waitForState(t, teammate, TeammateIdle, 3*time.Second)

	tasks, err := tm.listTeamTasks(context.Background(), orchestrator.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 team task, got %d", len(tasks))
	}
	if tasks[0].Status != orchestrator.TaskCompleted {
		t.Fatalf("expected completed task, got %s", tasks[0].Status)
	}
	if got := teamTaskAssignee(tasks[0]); got != "worker-1" {
		t.Fatalf("expected assignee worker-1, got %q", got)
	}
	if tasks[0].Result == nil || tasks[0].Result.Output != "task complete" {
		t.Fatalf("expected task result output to be recorded, got %#v", tasks[0].Result)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	if err := tm.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}
}

func TestTeam_SpawnTeammate_InitialTaskPromptIsNotDuplicated(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("done"))
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	worker, err := tm.SpawnTeammate(ctx, "worker-1", "investigate auth regression")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, worker, TeammateIdle, 3*time.Second)

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected at least one model call")
	}
	lastMsg, ok := calls[0].Messages[len(calls[0].Messages)-1].(core.ModelRequest)
	if !ok {
		t.Fatalf("expected final message to be a ModelRequest, got %T", calls[0].Messages[len(calls[0].Messages)-1])
	}

	var prompt string
	for _, part := range lastMsg.Parts {
		userPart, ok := part.(core.UserPromptPart)
		if ok {
			prompt = userPart.Content
			break
		}
	}
	if prompt != "investigate auth regression" {
		t.Fatalf("expected initial worker prompt to match original task, got %q", prompt)
	}
}

func TestTeam_SpawnDuplicate(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("done"))
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := tm.SpawnTeammate(ctx, "worker", "task 1"); err != nil {
		t.Fatal(err)
	}
	if _, err := tm.SpawnTeammate(ctx, "worker", "task 2"); err == nil {
		t.Fatal("expected duplicate teammate spawn to fail")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = tm.Shutdown(shutdownCtx)
}

func TestTeam_SpawnTeammate_AfterShutdownRejected(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("done"))
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  model,
	})

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	if err := tm.Shutdown(shutdownCtx); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := tm.SpawnTeammate(ctx, "worker", "task"); err == nil {
		t.Fatal("expected spawn after shutdown to fail")
	}
}

func TestTeamWorkerStore_ClaimPendingCommand_ScopesToVisibleTeamTasks(t *testing.T) {
	ctx := context.Background()
	shared := omemory.NewStore()
	teamA := NewTeam(TeamConfig{
		Name:   "team-a",
		Leader: "leader-a",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  shared,
	})
	teamB := NewTeam(TeamConfig{
		Name:   "team-b",
		Leader: "leader-b",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  shared,
	})

	taskB, err := teamB.createTeamTask(ctx, "team-b task", "", "", "leader-b")
	if err != nil {
		t.Fatal(err)
	}
	taskA, err := teamA.createTeamTask(ctx, "team-a task", "", "", "leader-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimTask(ctx, taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker",
		LeaseTTL: time.Minute,
		Now:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimTask(ctx, taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker",
		LeaseTTL: time.Minute,
		Now:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	commandB, err := shared.CreateCommand(ctx, orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: taskB.ID,
		Reason: "cancel team b",
	})
	if err != nil {
		t.Fatal(err)
	}
	commandA, err := shared.CreateCommand(ctx, orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: taskA.ID,
		Reason: "cancel team a",
	})
	if err != nil {
		t.Fatal(err)
	}

	workerStore := newTeamWorkerStore(teamA, "worker")
	claimed, err := workerStore.ClaimPendingCommand(ctx, orchestrator.ClaimCommandRequest{
		WorkerID: "worker",
		Now:      time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed.ID != commandA.ID {
		t.Fatalf("expected to claim team-a command %q, got %q", commandA.ID, claimed.ID)
	}

	persistedA, err := shared.GetCommand(ctx, commandA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedA.Status != orchestrator.CommandClaimed {
		t.Fatalf("expected team-a command to be claimed, got %s", persistedA.Status)
	}

	persistedB, err := shared.GetCommand(ctx, commandB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedB.Status != orchestrator.CommandPending {
		t.Fatalf("expected foreign team command to remain pending, got %s", persistedB.Status)
	}
}

func TestTeamWorkerStore_RecoverExpiredLeases_ScopesToVisibleTeamTasks(t *testing.T) {
	ctx := context.Background()
	base := time.Unix(10, 0).UTC()
	shared := omemory.NewStore()
	teamA := NewTeam(TeamConfig{
		Name:   "team-a",
		Leader: "leader-a",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  shared,
	})
	teamB := NewTeam(TeamConfig{
		Name:   "team-b",
		Leader: "leader-b",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  shared,
	})

	taskA, err := teamA.createTeamTask(ctx, "team-a task", "", "", "leader-a")
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := teamB.createTeamTask(ctx, "team-b task", "", "", "leader-b")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := shared.ClaimTask(ctx, taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Second,
		Now:      base,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimTask(ctx, taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Second,
		Now:      base,
	}); err != nil {
		t.Fatal(err)
	}

	workerStore := newTeamWorkerStore(teamA, "worker-a")
	recovered, err := workerStore.RecoverExpiredLeases(ctx, base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0] == nil || recovered[0].Task == nil || recovered[0].Task.ID != taskA.ID {
		t.Fatalf("expected only team-a lease recovery, got %#v", recovered)
	}

	persistedA, err := shared.GetTask(ctx, taskA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedA.Status != orchestrator.TaskFailed {
		t.Fatalf("expected team-a task to be recovered to failed, got %s", persistedA.Status)
	}

	persistedB, err := shared.GetTask(ctx, taskB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedB.Status != orchestrator.TaskRunning {
		t.Fatalf("expected foreign team task to remain running, got %s", persistedB.Status)
	}
}

func TestTeamWorkerStore_RecoverClaimedCommands_ScopesToVisibleTeamTasks(t *testing.T) {
	ctx := context.Background()
	base := time.Unix(20, 0).UTC()
	shared := omemory.NewStore()
	teamA := NewTeam(TeamConfig{
		Name:   "team-a",
		Leader: "leader-a",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  shared,
	})
	teamB := NewTeam(TeamConfig{
		Name:   "team-b",
		Leader: "leader-b",
		Model:  core.NewTestModel(core.TextResponse("done")),
		Store:  shared,
	})

	taskA, err := teamA.createTeamTask(ctx, "team-a task", "", "", "leader-a")
	if err != nil {
		t.Fatal(err)
	}
	taskB, err := teamB.createTeamTask(ctx, "team-b task", "", "", "leader-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimTask(ctx, taskA.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-a",
		LeaseTTL: time.Minute,
		Now:      base,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimTask(ctx, taskB.ID, orchestrator.ClaimTaskRequest{
		WorkerID: "worker-b",
		LeaseTTL: time.Minute,
		Now:      base,
	}); err != nil {
		t.Fatal(err)
	}

	commandA, err := shared.CreateCommand(ctx, orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: taskA.ID,
		Reason: "cancel team a",
	})
	if err != nil {
		t.Fatal(err)
	}
	commandB, err := shared.CreateCommand(ctx, orchestrator.CreateCommandRequest{
		Kind:   orchestrator.CommandCancelTask,
		TaskID: taskB.ID,
		Reason: "cancel team b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimCommand(ctx, commandA.ID, orchestrator.ClaimCommandRequest{
		WorkerID: "worker-a",
		Now:      base,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ClaimCommand(ctx, commandB.ID, orchestrator.ClaimCommandRequest{
		WorkerID: "worker-b",
		Now:      base,
	}); err != nil {
		t.Fatal(err)
	}

	workerStore := newTeamWorkerStore(teamA, "worker-a")
	recovered, err := workerStore.RecoverClaimedCommands(ctx, base.Add(time.Second), base.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(recovered) != 1 || recovered[0] == nil || recovered[0].Command == nil || recovered[0].Command.ID != commandA.ID {
		t.Fatalf("expected only team-a command recovery, got %#v", recovered)
	}

	persistedA, err := shared.GetCommand(ctx, commandA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedA.Status != orchestrator.CommandPending {
		t.Fatalf("expected team-a command to be recovered to pending, got %s", persistedA.Status)
	}

	persistedB, err := shared.GetCommand(ctx, commandB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persistedB.Status != orchestrator.CommandClaimed {
		t.Fatalf("expected foreign team command to remain claimed, got %s", persistedB.Status)
	}
}

func TestTeam_RemoveTeammate_ReleasesAssignedPendingTasks(t *testing.T) {
	model := core.NewTestModel(
		core.TextResponse("worker-1 initial complete"),
		core.TextResponse("worker-2 initial complete"),
		core.TextResponse("reclaimed follow-up complete"),
	)
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	worker1, err := tm.SpawnTeammate(ctx, "worker-1", "initial task 1")
	if err != nil {
		t.Fatal(err)
	}
	worker2, err := tm.SpawnTeammate(ctx, "worker-2", "initial task 2")
	if err != nil {
		t.Fatal(err)
	}

	waitForState(t, worker1, TeammateIdle, 3*time.Second)
	waitForState(t, worker2, TeammateIdle, 3*time.Second)

	task, err := tm.createTeamTask(ctx, "follow-up", "only worker-1 was assigned", "worker-1", "leader")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := tm.requestShutdown("worker-1", "leader", "done", ""); err != nil {
		t.Fatal(err)
	}

	waitForState(t, worker1, TeammateStopped, 3*time.Second)

	deadline := time.After(4 * time.Second)
	for {
		persisted, err := tm.getTeamTask(context.Background(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Status == orchestrator.TaskCompleted {
			if got := teamTaskAssignee(persisted); got != "" {
				t.Fatalf("expected task assignee to be cleared after worker removal, got %q", got)
			}
			break
		}

		select {
		case <-deadline:
			t.Fatalf("assigned task was not reclaimed after worker shutdown: %#v", persisted)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestTeam_Events(t *testing.T) {
	bus := core.NewEventBus()

	var mu sync.Mutex
	var spawned []TeammateSpawnedEvent
	var idle []TeammateIdleEvent
	var sequence []string
	core.Subscribe(bus, func(e TeammateSpawnedEvent) {
		mu.Lock()
		defer mu.Unlock()
		spawned = append(spawned, e)
		sequence = append(sequence, "spawned:"+e.TeammateName)
	})
	core.Subscribe(bus, func(e TeammateIdleEvent) {
		mu.Lock()
		defer mu.Unlock()
		idle = append(idle, e)
		sequence = append(sequence, "idle:"+e.TeammateName)
	})

	tm := NewTeam(TeamConfig{
		Name:     "test-team",
		Leader:   "leader",
		Model:    core.NewTestModel(core.TextResponse("done")),
		EventBus: bus,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	teammate, err := tm.SpawnTeammate(ctx, "worker", "task")
	if err != nil {
		t.Fatal(err)
	}
	waitForState(t, teammate, TeammateIdle, 3*time.Second)
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(spawned) != 1 {
		t.Fatalf("expected 1 spawned event, got %d", len(spawned))
	}
	if len(idle) < 1 {
		t.Fatalf("expected at least 1 idle event, got %d", len(idle))
	}
	if len(sequence) == 0 || sequence[0] != "spawned:worker" {
		t.Fatalf("expected spawned event before any idle event, got %#v", sequence)
	}
}

func TestTeam_SpawnTeammate_WithMaxTokens(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("done"))
	tm := NewTeam(TeamConfig{
		Name:   "test-team",
		Leader: "leader",
		Model:  model,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	teammate, err := tm.SpawnTeammate(ctx, "worker", "do something", WithTeammateMaxTokens(16384))
	if err != nil {
		t.Fatal(err)
	}

	waitForState(t, teammate, TeammateIdle, 3*time.Second)

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected at least one model call")
	}
	if calls[0].Settings == nil || calls[0].Settings.MaxTokens == nil || *calls[0].Settings.MaxTokens != 16384 {
		t.Fatalf("expected MaxTokens=16384, got %#v", calls[0].Settings)
	}
}

func TestTeam_WorkerMaxTokens_OverriddenByTeammateOption(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("done"))
	tm := NewTeam(TeamConfig{
		Name:            "test-team",
		Leader:          "leader",
		Model:           model,
		WorkerMaxTokens: 32768,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	teammate, err := tm.SpawnTeammate(ctx, "worker", "do something", WithTeammateMaxTokens(8192))
	if err != nil {
		t.Fatal(err)
	}

	waitForState(t, teammate, TeammateIdle, 3*time.Second)

	calls := model.Calls()
	if len(calls) == 0 {
		t.Fatal("expected at least one model call")
	}
	if calls[0].Settings == nil || calls[0].Settings.MaxTokens == nil || *calls[0].Settings.MaxTokens != 8192 {
		t.Fatalf("expected MaxTokens=8192, got %#v", calls[0].Settings)
	}
}

func TestTeam_ToolsetFactory(t *testing.T) {
	model := core.NewTestModel(core.TextResponse("done"))

	var callCount int
	var mu sync.Mutex
	factory := func() *core.Toolset {
		mu.Lock()
		callCount++
		mu.Unlock()
		return core.NewToolset("per-worker",
			core.FuncTool[struct{}]("noop", "noop", func(_ context.Context, _ struct{}) (string, error) {
				return "ok", nil
			}),
		)
	}

	tm := NewTeam(TeamConfig{
		Name:           "factory-test",
		Leader:         "leader",
		Model:          model,
		ToolsetFactory: factory,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := tm.SpawnTeammate(ctx, "w1", "task 1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = tm.SpawnTeammate(ctx, "w2", "task 2")
	if err != nil {
		t.Fatal(err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shutdownCancel()
	_ = tm.Shutdown(shutdownCtx)

	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Fatalf("expected factory called twice, got %d", callCount)
	}
}

func TestTeammateConfig_Options(t *testing.T) {
	cfg := &teammateConfig{}

	WithTeammateSystemPrompt("test prompt")(cfg)
	if cfg.systemPrompt != "test prompt" {
		t.Fatalf("expected test prompt, got %q", cfg.systemPrompt)
	}

	es := core.EndStrategyEarly
	WithTeammateEndStrategy(es)(cfg)
	if cfg.endStrategy == nil || *cfg.endStrategy != core.EndStrategyEarly {
		t.Fatal("expected EndStrategyEarly")
	}

	WithTeammateMaxTokens(4096)(cfg)
	if cfg.maxTokens != 4096 {
		t.Fatalf("expected 4096, got %d", cfg.maxTokens)
	}

	opt := core.WithMaxRetries[string](5)
	WithTeammateAgentOptions(opt)(cfg)
	if len(cfg.agentOpts) != 1 {
		t.Fatalf("expected 1 agent option, got %d", len(cfg.agentOpts))
	}
}

func waitForState(t *testing.T, tm *Teammate, state TeammateState, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		current := tm.State()
		if current == state || current == TeammateStopped {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("teammate did not reach state %v within %v, current: %v", state, timeout, current)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
