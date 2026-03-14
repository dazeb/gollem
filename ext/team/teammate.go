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
	"github.com/google/uuid"
)

// TeammateState represents the lifecycle state of a teammate.
type TeammateState int

const (
	TeammateStarting TeammateState = iota
	TeammateRunning
	TeammateIdle
	TeammateShuttingDown
	TeammateStopped
)

func (s TeammateState) String() string {
	switch s {
	case TeammateStarting:
		return "starting"
	case TeammateRunning:
		return "running"
	case TeammateIdle:
		return "idle"
	case TeammateShuttingDown:
		return "shutting_down"
	case TeammateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// TeammateInfo is a snapshot of teammate state for tools/prompts.
type TeammateInfo struct {
	Name  string        `json:"name"`
	State TeammateState `json:"state"`
}

type shutdownRequest struct {
	ID            string
	CorrelationID string
	From          string
	Reason        string
}

type activeTask struct {
	TaskID     string
	LeaseToken string
	RunID      string
	runEnded   bool
	settled    bool
}

type failedCurrentTaskError struct {
	Reason string
}

func (e *failedCurrentTaskError) Error() string {
	if e == nil || e.Reason == "" {
		return "current team task failed"
	}
	return "current team task failed: " + e.Reason
}

// Teammate represents a worker agent running as a goroutine.
type Teammate struct {
	mu                sync.Mutex
	name              string
	state             TeammateState
	agent             *core.Agent[string]
	cancel            context.CancelFunc
	team              *Team
	active            *activeTask
	runCancel         context.CancelCauseFunc
	shutdownRequested bool
	shutdown          shutdownRequest
}

// Name returns the teammate's name.
func (tm *Teammate) Name() string {
	return tm.name
}

// State returns the current state.
func (tm *Teammate) State() TeammateState {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.state
}

func (tm *Teammate) setState(s TeammateState) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.state = s
}

func (tm *Teammate) requestShutdown(from, reason, correlationID string) shutdownRequest {
	tm.mu.Lock()
	shouldStop := false
	cancel := tm.cancel
	if !tm.shutdownRequested {
		id := uuid.NewString()
		if correlationID == "" {
			correlationID = id
		}
		tm.shutdownRequested = true
		tm.shutdown = shutdownRequest{
			ID:            id,
			CorrelationID: correlationID,
			From:          from,
			Reason:        reason,
		}
		if tm.state == TeammateIdle || tm.state == TeammateStarting {
			tm.state = TeammateShuttingDown
			shouldStop = true
		}
	}
	req := tm.shutdown
	tm.mu.Unlock()

	if shouldStop && cancel != nil {
		cancel()
	}
	return req
}

func (tm *Teammate) shutdownSignal() (shutdownRequest, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if !tm.shutdownRequested {
		return shutdownRequest{}, false
	}
	return tm.shutdown, true
}

func (tm *Teammate) setActiveClaim(claim *orchestrator.ClaimedTask) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if claim == nil || claim.Task == nil || claim.Lease == nil {
		tm.active = nil
		return
	}
	runID := ""
	if claim.Run != nil {
		runID = claim.Run.ID
	}
	tm.active = &activeTask{
		TaskID:     claim.Task.ID,
		LeaseToken: claim.Lease.Token,
		RunID:      runID,
	}
}

func (tm *Teammate) clearActiveClaim() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.active = nil
}

func (tm *Teammate) activeClaim() (activeTask, bool) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if tm.active == nil {
		return activeTask{}, false
	}
	return *tm.active, true
}

func (tm *Teammate) beginRun(cancel context.CancelCauseFunc) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.runCancel = cancel
}

func (tm *Teammate) endRun() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.runCancel = nil
}

func (tm *Teammate) abortCurrentRun(cause error) bool {
	tm.mu.Lock()
	cancel := tm.runCancel
	tm.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel(cause)
	return true
}

func (tm *Teammate) enterIdle() {
	tm.setState(TeammateIdle)
	if tm.team.eventBus != nil {
		core.PublishAsync(tm.team.eventBus, TeammateIdleEvent{
			TeamName:     tm.team.name,
			TeammateName: tm.name,
		})
	}
}

func (tm *Teammate) markRunEnded(taskID string) {
	tm.markTaskProgress(taskID, true, false)
}

func (tm *Teammate) markTaskSettled(taskID string) {
	tm.markTaskProgress(taskID, false, true)
}

func (tm *Teammate) markTaskProgress(taskID string, runEnded, settled bool) {
	action := ""

	tm.mu.Lock()
	if tm.active != nil && tm.active.TaskID == taskID {
		if runEnded {
			tm.active.runEnded = true
		}
		if settled {
			tm.active.settled = true
		}
		if tm.active.runEnded && tm.active.settled {
			tm.active = nil
			if tm.shutdownRequested {
				tm.state = TeammateShuttingDown
				action = "shutdown"
			} else {
				tm.state = TeammateIdle
				action = "idle"
			}
		}
	}
	tm.mu.Unlock()

	switch action {
	case "shutdown":
		tm.cancelScheduler()
	case "idle":
		if tm.team.eventBus != nil {
			core.PublishAsync(tm.team.eventBus, TeammateIdleEvent{
				TeamName:     tm.team.name,
				TeammateName: tm.name,
			})
		}
	}
}

func (tm *Teammate) cancelScheduler() {
	tm.mu.Lock()
	cancel := tm.cancel
	tm.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// run is the main goroutine loop for a teammate.
func (tm *Teammate) run(ctx context.Context) {
	defer func() {
		tm.setState(TeammateStopped)
		reason := "stopped"
		if sig, ok := tm.shutdownSignal(); ok {
			reason = "shutdown_requested"
			if sig.Reason != "" {
				reason = sig.Reason
			}
		} else if ctx.Err() != nil {
			reason = "context_cancelled"
		}
		if tm.team.eventBus != nil {
			core.PublishAsync(tm.team.eventBus, TeammateTerminatedEvent{
				TeamName:     tm.team.name,
				TeammateName: tm.name,
				Reason:       reason,
			})
		}
		tm.team.removeTeammate(tm.name)
		tm.team.wg.Done()
	}()

	store := newTeamWorkerStore(tm.team, tm.name)
	runner := &teammateRunner{tm: tm}
	scheduler := orchestrator.NewScheduler(
		store,
		store,
		runner,
		orchestrator.WithWorkerID(tm.name),
		orchestrator.WithMaxConcurrentRuns(1),
		orchestrator.WithPollInterval(teamSchedulerPollRate),
		orchestrator.WithLeaseTTL(teamTaskLeaseTTL),
		orchestrator.WithSchedulerErrorHandler(func(err error) {
			if ctx.Err() != nil || err == nil {
				return
			}
			fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s scheduler error: %v\n", tm.team.name, tm.name, err)
		}),
	)
	if err := scheduler.Run(ctx); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s scheduler stopped with error: %v\n", tm.team.name, tm.name, err)
	}
	if sig, ok := tm.shutdownSignal(); ok {
		tm.setState(TeammateShuttingDown)
		fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s stopping after shutdown request from %s: %s\n",
			tm.team.name, tm.name, sig.From, previewForLog(sig.Reason, 240))
	}
}

type teammateRunner struct {
	tm *Teammate
}

func (r *teammateRunner) RunTask(ctx context.Context, claim *orchestrator.ClaimedTask) (*orchestrator.TaskOutcome, error) {
	if claim == nil || claim.Task == nil {
		return nil, nil
	}

	tm := r.tm
	tm.setActiveClaim(claim)
	tm.setState(TeammateRunning)
	fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s running task:%s\n", tm.team.name, tm.name, claim.Task.ID)

	defer func() {
		tm.markRunEnded(claim.Task.ID)
	}()

	runCtx, cancelRun := context.WithCancelCause(ctx)
	tm.beginRun(cancelRun)
	defer func() {
		tm.endRun()
		cancelRun(nil)
	}()

	runCtx = core.ContextWithToolCallID(runCtx, "")
	if claim.Run != nil && claim.Run.ID != "" {
		runCtx = core.ContextWithRunID(runCtx, claim.Run.ID)
	}

	result, runErr := tm.agent.Run(runCtx, teamTaskPrompt(claim.Task))
	if runErr != nil && errors.Is(runErr, context.Canceled) {
		if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, context.Canceled) {
			runErr = cause
		}
	}
	if runErr != nil {
		var failedCurrent *failedCurrentTaskError
		if errors.As(runErr, &failedCurrent) {
			return nil, nil
		}
		var canceledTask *orchestrator.TaskCancelCause
		if errors.As(runErr, &canceledTask) {
			return nil, nil
		}
		return nil, runErr
	}

	outcome := &orchestrator.TaskOutcome{
		Result: &orchestrator.TaskResult{
			RunnerRunID: result.RunID,
			Output:      result.Output,
			Usage:       result.Usage,
			ToolState:   cloneAnyMap(result.ToolState),
			Metadata: map[string]any{
				"teammate": tm.name,
			},
			CompletedAt: time.Now(),
		},
	}
	fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s completed task:%s (tokens: %d in, %d out)\n",
		tm.team.name, tm.name, claim.Task.ID, result.Usage.InputTokens, result.Usage.OutputTokens)
	fmt.Fprintf(os.Stderr, "[gollem] team:%s teammate:%s output: %s\n",
		tm.team.name, tm.name, previewForLog(result.Output, 320))
	return outcome, nil
}
