package team

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/modelutil"
)

// TeamConfig configures a new team.
type TeamConfig struct {
	// Name identifies this team.
	Name string

	// Leader is the name of the leader agent (receives idle notifications).
	Leader string

	// Model is the LLM used for spawned teammates.
	Model core.Model

	// Toolset is the set of tools given to each teammate (e.g. coding tools).
	// The team package is decoupled from codetool — the caller provides the toolset.
	Toolset *core.Toolset

	// EventBus receives team lifecycle events. Optional.
	EventBus *core.EventBus

	// MailboxSize is the buffer size for teammate mailboxes. Defaults to 64.
	MailboxSize int

	// PersonalityGenerator generates task-specific system prompts for
	// teammates. When set, each spawned teammate gets a dynamically
	// generated personality tailored to its assigned task. Falls back
	// to WorkerSystemPrompt on generation failure.
	PersonalityGenerator modelutil.PersonalityGeneratorFunc
}

// Team manages a group of teammate agents.
type Team struct {
	mu             sync.RWMutex
	name           string
	leader         string
	members        map[string]*Teammate
	taskBoard      *TaskBoard
	eventBus       *core.EventBus
	model          core.Model
	toolset        *core.Toolset
	mailboxSize    int
	personalityGen modelutil.PersonalityGeneratorFunc
	done           chan struct{}
	wg             sync.WaitGroup
}

// NewTeam creates a team with the given configuration.
func NewTeam(cfg TeamConfig) *Team {
	mailboxSize := cfg.MailboxSize
	if mailboxSize <= 0 {
		mailboxSize = 64
	}
	return &Team{
		name:           cfg.Name,
		leader:         cfg.Leader,
		members:        make(map[string]*Teammate),
		taskBoard:      NewTaskBoard(),
		eventBus:       cfg.EventBus,
		model:          cfg.Model,
		toolset:        cfg.Toolset,
		mailboxSize:    mailboxSize,
		personalityGen: cfg.PersonalityGenerator,
		done:           make(chan struct{}),
	}
}

// TeammateOption configures a teammate.
type TeammateOption func(*teammateConfig)

type teammateConfig struct {
	systemPrompt string
}

// WithTeammateSystemPrompt overrides the default worker system prompt.
func WithTeammateSystemPrompt(prompt string) TeammateOption {
	return func(c *teammateConfig) { c.systemPrompt = prompt }
}

// RegisterLeader registers the leader agent's mailbox in the team so that
// workers can send messages to the leader. Returns a middleware that drains
// the leader's mailbox between model turns (injecting messages as UserPromptParts).
func (t *Team) RegisterLeader(name string) core.AgentMiddleware {
	mb := NewMailbox(t.mailboxSize)
	tm := &Teammate{
		name:    name,
		state:   TeammateRunning,
		mailbox: mb,
		team:    t,
		wakeCh:  make(chan struct{}, 1),
	}
	t.mu.Lock()
	t.members[name] = tm
	t.mu.Unlock()
	return TeamAwarenessMiddleware(tm)
}

// SpawnTeammate creates a new teammate agent and starts it in a goroutine.
func (t *Team) SpawnTeammate(ctx context.Context, name, task string, opts ...TeammateOption) (*Teammate, error) {
	cfg := &teammateConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Resolve system prompt: explicit override > personality generator > default.
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

	mailbox := NewMailbox(t.mailboxSize)

	tm := &Teammate{
		name:    name,
		state:   TeammateStarting,
		mailbox: mailbox,
		team:    t,
		wakeCh:  make(chan struct{}, 1),
	}

	// Reserve the name atomically to prevent TOCTOU races.
	t.mu.Lock()
	if _, exists := t.members[name]; exists {
		t.mu.Unlock()
		return nil, fmt.Errorf("teammate %q already exists", name)
	}
	t.members[name] = tm
	t.mu.Unlock()

	// Build agent with the configured toolset + team tools + awareness middleware.
	agentOpts := []core.AgentOption[string]{
		core.WithSystemPrompt[string](cfg.systemPrompt),
		core.WithTools[string](WorkerTools(t, tm)...),
		core.WithAgentMiddleware[string](TeamAwarenessMiddleware(tm)),
		core.WithMaxRetries[string](2),
		core.WithUsageLimits[string](core.UsageLimits{RequestLimit: core.IntPtr(200)}),
		core.WithTurnGuardrail[string]("max-turns", core.MaxTurns(200)),
		core.WithDefaultToolTimeout[string](2 * time.Minute),
	}
	if t.toolset != nil {
		agentOpts = append(agentOpts, core.WithToolsets[string](t.toolset))
	}
	if t.eventBus != nil {
		agentOpts = append(agentOpts, core.WithEventBus[string](t.eventBus))
	}

	tm.agent = core.NewAgent[string](t.model, agentOpts...)

	tmCtx, cancel := context.WithCancel(ctx)
	tm.cancel = cancel

	t.wg.Add(1)
	go tm.run(tmCtx, task)

	if t.eventBus != nil {
		core.PublishAsync(t.eventBus, TeammateSpawnedEvent{
			TeamName:     t.name,
			TeammateName: name,
			Task:         task,
		})
	}

	fmt.Fprintf(os.Stderr, "[gollem] team:%s spawned teammate:%s\n", t.name, name)
	return tm, nil
}

// Shutdown gracefully shuts down all teammates.
func (t *Team) Shutdown(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "[gollem] team:%s shutting down\n", t.name)

	// Signal all teammates to stop.
	t.mu.RLock()
	for _, tm := range t.members {
		tm.mailbox.Send(Message{
			From:      "team",
			To:        tm.name,
			Type:      MessageShutdownRequest,
			Content:   "Team is shutting down",
			Timestamp: time.Now(),
		})
		tm.Wake()
	}
	t.mu.RUnlock()

	// Close done channel to unblock any waiting teammates.
	close(t.done)

	// Wait for all goroutines with context deadline.
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
		// Force cancel all teammates.
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

// TaskBoard returns the shared task board.
func (t *Team) TaskBoard() *TaskBoard {
	return t.taskBoard
}

// Name returns the team name.
func (t *Team) Name() string {
	return t.name
}

// getMailbox returns the mailbox for the named member, or nil.
func (t *Team) getMailbox(name string) *Mailbox {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if tm, ok := t.members[name]; ok {
		return tm.mailbox
	}
	return nil
}

// GetTeammate returns the teammate with the given name, or nil.
func (t *Team) GetTeammate(name string) *Teammate {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.members[name]
}
