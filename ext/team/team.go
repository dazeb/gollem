package team

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
	omemory "github.com/fugue-labs/gollem/ext/orchestrator/memory"
	"github.com/fugue-labs/gollem/modelutil"
)

const (
	teamTaskKind          = "team"
	teamMetadataName      = "team.name"
	teamMetadataAssignee  = "team.assignee"
	teamMetadataCreatedBy = "team.created_by"
	teamTaskLeaseTTL      = 24 * time.Hour
	teamSchedulerPollRate = 100 * time.Millisecond
)

// OrchestratorStore is the backend contract ext/team needs from orchestrator.
type OrchestratorStore interface {
	orchestrator.TaskStore
	orchestrator.LeaseStore
	orchestrator.CommandStore
	orchestrator.ArtifactStore
	orchestrator.LeaseRecoveryStore
	orchestrator.CommandRecoveryStore
}

// TeamConfig configures a new team.
type TeamConfig struct {
	// Name identifies this team.
	Name string

	// Leader names the coordinating agent using LeaderTools.
	Leader string

	// Model is the LLM used for spawned teammates.
	Model core.Model

	// Toolset is the set of tools given to each teammate (e.g. coding tools).
	// When both Toolset and ToolsetFactory are set, ToolsetFactory takes precedence.
	Toolset *core.Toolset

	// ToolsetFactory creates a fresh toolset for each spawned teammate.
	ToolsetFactory func() *core.Toolset

	// WorkerExtraTools are additional tools attached to each spawned teammate.
	WorkerExtraTools []core.Tool

	// EventBus receives teammate lifecycle events and orchestrator store events.
	EventBus *core.EventBus

	// Store overrides the default in-memory orchestrator backend.
	// Pass a store dedicated to this team instance so tasks, commands,
	// recovery, and durable history stay scoped to one team.
	Store OrchestratorStore

	// WorkerMaxTokens sets the default max output tokens per model request
	// for all spawned teammates.
	WorkerMaxTokens int

	// WorkerHooks are lifecycle hooks added to every spawned teammate.
	WorkerHooks []core.Hook

	// PersonalityGenerator generates task-specific system prompts for
	// teammates. Falls back to WorkerSystemPrompt on generation failure.
	PersonalityGenerator modelutil.PersonalityGeneratorFunc
}

// Team manages a group of teammate agents backed by an orchestrator store.
type Team struct {
	mu              sync.RWMutex
	name            string
	leader          string
	members         map[string]*Teammate
	closing         bool
	store           OrchestratorStore
	eventBus        *core.EventBus
	model           core.Model
	toolset         *core.Toolset
	toolsetFactory  func() *core.Toolset
	workerTools     []core.Tool
	workerMaxTokens int
	workerHooks     []core.Hook
	personalityGen  modelutil.PersonalityGeneratorFunc
	wg              sync.WaitGroup
}

// NewTeam creates a team with the given configuration.
func NewTeam(cfg TeamConfig) *Team {
	store := cfg.Store
	if store == nil {
		store = omemory.NewStore(omemory.WithEventBus(cfg.EventBus))
	}
	return &Team{
		name:            cfg.Name,
		leader:          cfg.Leader,
		members:         make(map[string]*Teammate),
		store:           store,
		eventBus:        cfg.EventBus,
		model:           cfg.Model,
		toolset:         cfg.Toolset,
		toolsetFactory:  cfg.ToolsetFactory,
		workerTools:     cfg.WorkerExtraTools,
		workerMaxTokens: cfg.WorkerMaxTokens,
		workerHooks:     cfg.WorkerHooks,
		personalityGen:  cfg.PersonalityGenerator,
	}
}

// TeammateOption configures a teammate.
type TeammateOption func(*teammateConfig)

type teammateConfig struct {
	systemPrompt string
	hooks        []core.Hook
	endStrategy  *core.EndStrategy
	maxTokens    int
	agentOpts    []core.AgentOption[string]
}

// WithTeammateSystemPrompt overrides the default worker system prompt.
func WithTeammateSystemPrompt(prompt string) TeammateOption {
	return func(c *teammateConfig) { c.systemPrompt = prompt }
}

// WithTeammateHooks adds lifecycle hooks to a spawned teammate.
func WithTeammateHooks(hooks ...core.Hook) TeammateOption {
	return func(c *teammateConfig) { c.hooks = append(c.hooks, hooks...) }
}

// WithTeammateEndStrategy sets the end strategy for a spawned teammate.
func WithTeammateEndStrategy(s core.EndStrategy) TeammateOption {
	return func(c *teammateConfig) { c.endStrategy = &s }
}

// WithTeammateMaxTokens sets the max output tokens per model request.
func WithTeammateMaxTokens(n int) TeammateOption {
	return func(c *teammateConfig) { c.maxTokens = n }
}

// WithTeammateAgentOptions appends arbitrary agent options to a spawned teammate.
func WithTeammateAgentOptions(opts ...core.AgentOption[string]) TeammateOption {
	return func(c *teammateConfig) { c.agentOpts = append(c.agentOpts, opts...) }
}

// SpawnTeammate creates a new teammate agent and assigns its initial task via the orchestrator store.
func (t *Team) SpawnTeammate(ctx context.Context, name, task string, opts ...TeammateOption) (*Teammate, error) {
	if name == "" {
		return nil, errors.New("teammate name must not be empty")
	}
	if task == "" {
		return nil, errors.New("initial task must not be empty")
	}

	cfg := &teammateConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if cfg.systemPrompt == "" && t.personalityGen != nil {
		basePrompt := WorkerSystemPrompt(name, t.name)
		if generated, err := t.personalityGen(ctx, modelutil.PersonalityRequest{
			Task:       task,
			Role:       fmt.Sprintf("teammate %q in team %q", name, t.name),
			BasePrompt: basePrompt,
		}); err == nil {
			cfg.systemPrompt = generated
			fmt.Fprintf(os.Stderr, "[gollem] team:%s personality generated for %s (%d chars)\n", t.name, name, len(generated))
		} else {
			fmt.Fprintf(os.Stderr, "[gollem] team:%s personality fallback for %s: %v\n", t.name, name, err)
		}
	}
	if cfg.systemPrompt == "" {
		cfg.systemPrompt = WorkerSystemPrompt(name, t.name)
	}

	tm := &Teammate{
		name:  name,
		state: TeammateStarting,
		team:  t,
	}

	t.mu.Lock()
	if t.closing {
		t.mu.Unlock()
		return nil, errors.New("team is shutting down")
	}
	if _, exists := t.members[name]; exists {
		t.mu.Unlock()
		return nil, fmt.Errorf("teammate %q already exists", name)
	}
	t.members[name] = tm
	t.wg.Add(1)
	t.mu.Unlock()

	endStrategy := core.EndStrategyExhaustive
	if cfg.endStrategy != nil {
		endStrategy = *cfg.endStrategy
	}

	agentOpts := []core.AgentOption[string]{
		core.WithSystemPrompt[string](cfg.systemPrompt),
		core.WithTools[string](WorkerTools(t, tm)...),
		core.WithEndStrategy[string](endStrategy),
		core.WithMaxRetries[string](2),
		core.WithUsageLimits[string](core.UsageLimits{RequestLimit: core.IntPtr(200)}),
		core.WithTurnGuardrail[string]("max-turns", core.MaxTurns(200)),
		core.WithDefaultToolTimeout[string](2 * time.Minute),
	}
	maxTokens := t.workerMaxTokens
	if cfg.maxTokens > 0 {
		maxTokens = cfg.maxTokens
	}
	if maxTokens > 0 {
		agentOpts = append(agentOpts, core.WithMaxTokens[string](maxTokens))
	}
	if t.toolsetFactory != nil {
		agentOpts = append(agentOpts, core.WithToolsets[string](t.toolsetFactory()))
	} else if t.toolset != nil {
		agentOpts = append(agentOpts, core.WithToolsets[string](t.toolset))
	}
	if len(t.workerTools) > 0 {
		agentOpts = append(agentOpts, core.WithTools[string](t.workerTools...))
	}
	if t.eventBus != nil {
		agentOpts = append(agentOpts, core.WithEventBus[string](t.eventBus))
	}
	allHooks := append(t.workerHooks, cfg.hooks...)
	if len(allHooks) > 0 {
		agentOpts = append(agentOpts, core.WithHooks[string](allHooks...))
	}
	agentOpts = append(agentOpts, cfg.agentOpts...)

	tm.agent = core.NewAgent[string](t.model, agentOpts...)

	if _, err := t.createTeamTask(ctx, task, "", name, t.leaderSenderName()); err != nil {
		t.mu.Lock()
		delete(t.members, name)
		t.mu.Unlock()
		t.wg.Done()
		return nil, fmt.Errorf("create initial team task: %w", err)
	}

	tmCtx, cancel := context.WithCancel(ctx)
	tm.cancel = cancel
	startCh := make(chan struct{})

	go tm.run(tmCtx, startCh)

	if t.eventBus != nil {
		core.PublishAsync(t.eventBus, TeammateSpawnedEvent{
			TeamName:     t.name,
			TeammateName: name,
			Task:         task,
		})
	}
	close(startCh)

	fmt.Fprintf(os.Stderr, "[gollem] team:%s spawned teammate:%s\n", t.name, name)
	return tm, nil
}

func (t *Team) leaderName() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.leader
}

func (t *Team) removeTeammate(name string) {
	t.mu.Lock()
	delete(t.members, name)
	t.mu.Unlock()

	if err := t.releaseAssignedTasks(context.Background(), name); err != nil {
		fmt.Fprintf(os.Stderr, "[gollem] team:%s failed to release tasks for teammate:%s: %v\n", t.name, name, err)
	}
}

func (t *Team) leaderSenderName() string {
	if leader := t.leaderName(); leader != "" {
		return leader
	}
	return "leader"
}

func (t *Team) createTeamTask(ctx context.Context, subject, description, assignee, createdBy string) (*orchestrator.Task, error) {
	if subject == "" {
		return nil, errors.New("team task subject must not be empty")
	}
	if assignee != "" && t.GetTeammate(assignee) == nil {
		return nil, fmt.Errorf("teammate %q not found", assignee)
	}

	task, err := t.store.CreateTask(ctx, orchestrator.CreateTaskRequest{
		Kind:        teamTaskKind,
		Subject:     subject,
		Description: description,
		Input:       buildTeamTaskPrompt(subject, description),
		MaxAttempts: 1,
		Metadata:    newTeamTaskMetadata(t.name, assignee, createdBy),
	})
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (t *Team) releaseAssignedTasks(ctx context.Context, assignee string) error {
	if assignee == "" {
		return nil
	}

	tasks, err := t.listTeamTasks(ctx, orchestrator.TaskFilter{
		Statuses: []orchestrator.TaskStatus{orchestrator.TaskPending},
	})
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if teamTaskAssignee(task) != assignee {
			continue
		}
		_, updateErr := t.store.UpdateTask(ctx, orchestrator.UpdateTaskRequest{
			ID: task.ID,
			Metadata: map[string]any{
				teamMetadataAssignee: nil,
			},
		})
		if updateErr != nil {
			return updateErr
		}
	}
	return nil
}

func (t *Team) listTeamTasks(ctx context.Context, filter orchestrator.TaskFilter) ([]*orchestrator.Task, error) {
	filter.Kinds = []string{teamTaskKind}
	tasks, err := t.store.ListTasks(ctx, filter)
	if err != nil {
		return nil, err
	}
	filtered := make([]*orchestrator.Task, 0, len(tasks))
	for _, task := range tasks {
		if t.isTeamTask(task) {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (t *Team) getTeamTask(ctx context.Context, id string) (*orchestrator.Task, error) {
	task, err := t.store.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	if !t.isTeamTask(task) {
		return nil, orchestrator.ErrTaskNotFound
	}
	return task, nil
}

func (t *Team) isTeamTask(task *orchestrator.Task) bool {
	if task == nil || task.Kind != teamTaskKind {
		return false
	}
	return teamTaskName(task) == t.name
}

func (t *Team) requestShutdown(name, from, reason, correlationID string) (shutdownRequest, error) {
	tm := t.GetTeammate(name)
	if tm == nil {
		return shutdownRequest{}, fmt.Errorf("teammate %q not found", name)
	}
	if reason == "" {
		reason = "work complete"
	}
	req := tm.requestShutdown(from, reason, correlationID)
	return req, nil
}

// Shutdown gracefully shuts down all teammates.
func (t *Team) Shutdown(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "[gollem] team:%s shutting down\n", t.name)

	t.mu.Lock()
	t.closing = true
	members := make([]*Teammate, 0, len(t.members))
	for _, tm := range t.members {
		members = append(members, tm)
	}
	t.mu.Unlock()

	for _, tm := range members {
		tm.requestShutdown("team", "team shutdown", "")
	}

	doneCh := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		fmt.Fprintf(os.Stderr, "[gollem] team:%s all teammates stopped\n", t.name)
		return nil
	case <-ctx.Done():
		t.mu.RLock()
		for _, tm := range t.members {
			if tm.cancel != nil {
				tm.cancel()
			}
		}
		t.mu.RUnlock()
		return ctx.Err()
	}
}

// Members returns a snapshot of all teammates.
func (t *Team) Members() []TeammateInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	infos := make([]TeammateInfo, 0, len(t.members))
	for _, tm := range t.members {
		infos = append(infos, TeammateInfo{
			Name:  tm.name,
			State: tm.State(),
		})
	}
	return infos
}

// Name returns the team name.
func (t *Team) Name() string {
	return t.name
}

// Store exposes the underlying orchestrator store used by the team.
func (t *Team) Store() OrchestratorStore {
	return t.store
}

// TaskStore exposes the team's orchestrator-backed task store.
func (t *Team) TaskStore() orchestrator.TaskStore {
	return t.store
}

// LeaseStore exposes the team's orchestrator-backed lease store.
func (t *Team) LeaseStore() orchestrator.LeaseStore {
	return t.store
}

// CommandStore exposes the team's orchestrator-backed command store.
func (t *Team) CommandStore() orchestrator.CommandStore {
	return t.store
}

// ArtifactStore exposes the team's orchestrator-backed artifact store.
func (t *Team) ArtifactStore() orchestrator.ArtifactStore {
	return t.store
}

// GetTeammate returns the teammate with the given name, or nil.
func (t *Team) GetTeammate(name string) *Teammate {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.members[name]
}
