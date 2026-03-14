package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// SchedulerOption customizes scheduler behavior.
type SchedulerOption func(*SchedulerConfig)

// SchedulerConfig controls polling, lease, and worker behavior.
type SchedulerConfig struct {
	WorkerID            string
	PollInterval        time.Duration
	LeaseTTL            time.Duration
	LeaseRenewInterval  time.Duration
	RecoveryInterval    time.Duration
	CommandClaimTimeout time.Duration
	MaxConcurrentRuns   int
	Now                 func() time.Time
	OnError             func(error)
}

type activeTaskRun struct {
	claim  *ClaimedTask
	cancel context.CancelCauseFunc
}

// DefaultSchedulerConfig returns sane defaults for in-process orchestration.
func DefaultSchedulerConfig() SchedulerConfig {
	leaseTTL := 30 * time.Second
	return SchedulerConfig{
		WorkerID:            fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		PollInterval:        100 * time.Millisecond,
		LeaseTTL:            leaseTTL,
		LeaseRenewInterval:  leaseTTL / 2,
		RecoveryInterval:    leaseTTL / 2,
		CommandClaimTimeout: leaseTTL,
		MaxConcurrentRuns:   1,
		Now:                 time.Now,
		OnError:             func(error) {},
	}
}

// WithWorkerID sets the scheduler's worker identity.
func WithWorkerID(workerID string) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.WorkerID = workerID
	}
}

// WithPollInterval sets how often the scheduler looks for more work.
func WithPollInterval(interval time.Duration) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.PollInterval = interval
	}
}

// WithLeaseTTL sets the lease expiration window for claimed tasks.
func WithLeaseTTL(ttl time.Duration) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.LeaseTTL = ttl
	}
}

// WithLeaseRenewInterval sets how often the scheduler renews active leases.
func WithLeaseRenewInterval(interval time.Duration) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.LeaseRenewInterval = interval
	}
}

// WithRecoveryInterval sets how often the scheduler reclaims expired leases and stale commands.
func WithRecoveryInterval(interval time.Duration) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.RecoveryInterval = interval
	}
}

// WithCommandClaimTimeout sets how old a claimed command can be before recovery returns it to pending.
func WithCommandClaimTimeout(timeout time.Duration) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.CommandClaimTimeout = timeout
	}
}

// WithMaxConcurrentRuns sets the scheduler's concurrency limit.
func WithMaxConcurrentRuns(n int) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.MaxConcurrentRuns = n
	}
}

// WithSchedulerClock overrides the scheduler's clock source for tests.
func WithSchedulerClock(now func() time.Time) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.Now = now
	}
}

// WithSchedulerErrorHandler receives asynchronous scheduler errors.
func WithSchedulerErrorHandler(fn func(error)) SchedulerOption {
	return func(cfg *SchedulerConfig) {
		cfg.OnError = fn
	}
}

// Scheduler polls for ready tasks, acquires leases, and runs them through a Runner.
type Scheduler struct {
	tasks    TaskStore
	leases   LeaseStore
	commands CommandStore
	runner   Runner
	cfg      SchedulerConfig

	slots    chan struct{}
	wg       sync.WaitGroup
	activeMu sync.Mutex
	active   map[string]activeTaskRun
}

// NewScheduler constructs a scheduler over a task store, lease store, and runner.
func NewScheduler(tasks TaskStore, leases LeaseStore, runner Runner, opts ...SchedulerOption) *Scheduler {
	if tasks == nil {
		panic("gollem/orchestrator: scheduler requires a non-nil task store")
	}
	if leases == nil {
		panic("gollem/orchestrator: scheduler requires a non-nil lease store")
	}
	if runner == nil {
		panic("gollem/orchestrator: scheduler requires a non-nil runner")
	}

	cfg := DefaultSchedulerConfig()
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = DefaultSchedulerConfig().WorkerID
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultSchedulerConfig().PollInterval
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = DefaultSchedulerConfig().LeaseTTL
	}
	if cfg.LeaseRenewInterval <= 0 {
		cfg.LeaseRenewInterval = cfg.LeaseTTL / 2
	}
	if cfg.RecoveryInterval <= 0 {
		cfg.RecoveryInterval = cfg.LeaseTTL / 2
	}
	if cfg.CommandClaimTimeout <= 0 {
		cfg.CommandClaimTimeout = cfg.LeaseTTL
	}
	if cfg.MaxConcurrentRuns <= 0 {
		cfg.MaxConcurrentRuns = 1
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.OnError == nil {
		cfg.OnError = func(error) {}
	}

	return &Scheduler{
		tasks:    tasks,
		leases:   leases,
		commands: inferCommandStore(tasks),
		runner:   runner,
		cfg:      cfg,
		slots:    make(chan struct{}, cfg.MaxConcurrentRuns),
		active:   make(map[string]activeTaskRun),
	}
}

// Run blocks until ctx is canceled or the scheduler encounters a fatal polling error.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	defer s.wg.Wait()

	var nextRecovery time.Time
	for {
		if s.recoveryEnabled() {
			now := s.cfg.Now()
			if nextRecovery.IsZero() || !now.Before(nextRecovery) {
				if err := s.recover(context.WithoutCancel(ctx), now); err != nil {
					if shouldStopScheduler(ctx, err) {
						return nil
					}
					return err
				}
				nextRecovery = now.Add(s.cfg.RecoveryInterval)
			}
		}
		if err := s.processCommands(ctx); err != nil {
			if shouldStopScheduler(ctx, err) {
				return nil
			}
			return err
		}
		if err := s.dispatch(ctx); err != nil {
			if shouldStopScheduler(ctx, err) {
				return nil
			}
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) dispatch(ctx context.Context) error {
	for s.tryAcquireSlot() {
		claim, err := s.tasks.ClaimReadyTask(ctx, ClaimTaskRequest{
			WorkerID:       s.cfg.WorkerID,
			LeaseTTL:       s.cfg.LeaseTTL,
			Now:            s.cfg.Now(),
			ExcludeTaskIDs: s.activeTaskIDs(),
		})
		if err != nil {
			s.releaseSlot()
			if errors.Is(err, ErrNoReadyTask) {
				return nil
			}
			return err
		}
		if s.hasConflictingLocalRun(claim) {
			if err := s.leases.ReleaseLease(context.WithoutCancel(ctx), claim.Task.ID, claim.Lease.Token); err != nil && !isLeaseLoss(err) {
				s.releaseSlot()
				return err
			}
			s.releaseSlot()
			continue
		}

		s.wg.Add(1)
		go s.runClaim(ctx, claim)
	}
	return nil
}

func (s *Scheduler) runClaim(ctx context.Context, claim *ClaimedTask) {
	defer s.wg.Done()
	defer s.releaseSlot()

	runCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	s.registerActiveClaim(claim, cancel)
	defer s.unregisterActiveClaim(claim)

	updateCtx := context.WithoutCancel(runCtx)
	done := make(chan struct{})
	renewDone := make(chan struct{})
	renewErrCh := make(chan error, 1)
	go func() {
		defer close(renewDone)
		s.renewLoop(runCtx, claim, done, cancel, renewErrCh)
	}()

	outcome, runErr := s.runner.RunTask(runCtx, claim)
	close(done)
	<-renewDone

	var renewErr error
	select {
	case renewErr = <-renewErrCh:
	default:
	}

	if runErr == nil && renewErr != nil {
		runErr = renewErr
	}
	if runErr != nil && errors.Is(runErr, context.Canceled) {
		if cause := context.Cause(runCtx); cause != nil && !errors.Is(cause, context.Canceled) {
			runErr = cause
		}
	}

	now := s.cfg.Now()
	if runErr != nil {
		if renewErr == nil && errors.Is(runErr, context.Canceled) && ctx.Err() != nil {
			if err := s.leases.ReleaseLease(updateCtx, claim.Task.ID, claim.Lease.Token); err != nil && !isLeaseLoss(err) {
				s.cfg.OnError(err)
			}
			return
		}
		task, err := s.tasks.FailTask(updateCtx, claim.Task.ID, claim.Lease.Token, runErr, now)
		if err != nil {
			if isLeaseLoss(err) {
				if runErr != nil && !errors.Is(runErr, context.Canceled) {
					s.cfg.OnError(runErr)
				}
				return
			}
			s.cfg.OnError(err)
			return
		}
		if err == nil && task != nil && task.Status == TaskPending {
			return
		}
		return
	}

	if outcome != nil && outcome.Result != nil && outcome.Result.CompletedAt.IsZero() {
		outcome.Result.CompletedAt = now
	}
	if _, err := s.tasks.CompleteTask(updateCtx, claim.Task.ID, claim.Lease.Token, outcome, now); err != nil && !isLeaseLoss(err) {
		s.cfg.OnError(err)
	}
}

func (s *Scheduler) renewLoop(ctx context.Context, claim *ClaimedTask, done <-chan struct{}, cancel context.CancelCauseFunc, renewErrCh chan<- error) {
	if claim == nil || claim.Task == nil || claim.Lease == nil {
		return
	}

	ticker := time.NewTicker(s.cfg.LeaseRenewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := s.leases.RenewLease(ctx, claim.Task.ID, claim.Lease.Token, s.cfg.LeaseTTL, s.cfg.Now()); err != nil {
				select {
				case renewErrCh <- err:
				default:
				}
				cancel(err)
				return
			}
		}
	}
}

func (s *Scheduler) tryAcquireSlot() bool {
	select {
	case s.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Scheduler) releaseSlot() {
	select {
	case <-s.slots:
	default:
	}
}

func isLeaseLoss(err error) bool {
	return errors.Is(err, ErrLeaseNotFound) || errors.Is(err, ErrLeaseExpired) || errors.Is(err, ErrLeaseMismatch)
}

func (s *Scheduler) recoveryEnabled() bool {
	if s.cfg.RecoveryInterval <= 0 {
		return false
	}
	if _, ok := s.tasks.(LeaseRecoveryStore); ok {
		return true
	}
	if s.cfg.CommandClaimTimeout > 0 {
		if _, ok := s.commands.(CommandRecoveryStore); ok {
			return true
		}
	}
	return false
}

func shouldStopScheduler(ctx context.Context, err error) bool {
	if ctx == nil || ctx.Err() == nil || err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func (s *Scheduler) processCommands(ctx context.Context) error {
	if s.commands == nil {
		return nil
	}
	updateCtx := context.WithoutCancel(ctx)
	for {
		command, err := s.commands.ClaimPendingCommand(ctx, ClaimCommandRequest{
			WorkerID: s.cfg.WorkerID,
			Now:      s.cfg.Now(),
		})
		if err != nil {
			if errors.Is(err, ErrNoPendingCommand) {
				return nil
			}
			return err
		}
		if err := s.handleCommand(updateCtx, command); err != nil {
			if releaseErr := s.commands.ReleaseCommand(updateCtx, command.ID, command.ClaimToken); releaseErr != nil && !errors.Is(releaseErr, ErrCommandClaimMismatch) {
				s.cfg.OnError(releaseErr)
				return releaseErr
			}
			s.cfg.OnError(err)
			return nil
		}
	}
}

func (s *Scheduler) recover(ctx context.Context, now time.Time) error {
	controller, _ := s.runner.(RunController)
	manager := NewRecoveryManager(s.tasks, s.commands,
		WithRecoveryController(controller),
		WithRecoveryCommandClaimTimeout(s.cfg.CommandClaimTimeout),
		WithRecoveryErrorHandler(s.cfg.OnError),
		WithRecoveryLocalCanceler(func(task *Task, cause error) bool {
			active, ok := s.localActiveRun(task)
			if !ok {
				return false
			}
			active.cancel(cause)
			return true
		}),
	)
	_, err := manager.Sweep(ctx, now)
	return err
}

func (s *Scheduler) handleCommand(ctx context.Context, command *Command) error {
	if command == nil {
		return nil
	}
	now := s.cfg.Now()
	switch command.Kind {
	case CommandCancelTask:
		if err := s.applyCancelTask(ctx, command, now); err != nil {
			return err
		}
	case CommandAbortRun:
		if err := s.applyAbortRun(ctx, command); err != nil {
			return err
		}
	case CommandRetryTask:
		if err := s.applyRetryTask(ctx, command, now); err != nil {
			return err
		}
	default:
		return ErrInvalidCommand
	}
	_, err := s.commands.HandleCommand(ctx, command.ID, command.ClaimToken, s.cfg.WorkerID, now)
	return err
}

func (s *Scheduler) applyCancelTask(ctx context.Context, command *Command, now time.Time) error {
	task, err := s.tasks.GetTask(ctx, command.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil
		}
		return err
	}
	switch task.Status {
	case TaskPending:
		_, err = s.tasks.CancelTask(ctx, task.ID, "", command.Reason, now)
		if errors.Is(err, ErrTaskNotCancelable) {
			return nil
		}
		return err
	case TaskRunning:
		lease, err := s.leases.GetLease(ctx, task.ID)
		if err != nil {
			return err
		}
		cause := &TaskCancelCause{Reason: command.Reason}
		if active, ok := s.localActiveRun(task); ok {
			if _, err := s.tasks.CancelTask(ctx, task.ID, lease.Token, command.Reason, now); err != nil && !errors.Is(err, ErrTaskNotCancelable) {
				return err
			}
			active.cancel(cause)
			return nil
		}
		if err := s.cancelRemoteRun(ctx, task, task.Run, cause); err != nil {
			return err
		}
		if _, err := s.tasks.CancelTask(ctx, task.ID, lease.Token, command.Reason, now); err != nil && !errors.Is(err, ErrTaskNotCancelable) {
			return err
		}
		return nil
	case TaskCompleted, TaskFailed, TaskCanceled:
		return nil
	default:
		return nil
	}
}

func (s *Scheduler) applyAbortRun(ctx context.Context, command *Command) error {
	task, err := s.tasks.GetTask(ctx, command.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil
		}
		return err
	}
	if task.Status != TaskRunning || task.Run == nil {
		return nil
	}
	if command.RunID != "" && task.Run.ID != command.RunID {
		return nil
	}
	cause := &RunAbortCause{Reason: command.Reason}
	if active, ok := s.localActiveRun(task); ok {
		active.cancel(cause)
		return nil
	}
	lease, err := s.leases.GetLease(ctx, task.ID)
	if err != nil {
		return err
	}
	if err := s.cancelRemoteRun(ctx, task, task.Run, cause); err != nil {
		return err
	}
	_, err = s.tasks.FailTask(ctx, task.ID, lease.Token, cause, s.cfg.Now())
	if isLeaseLoss(err) {
		return nil
	}
	return err
}

func (s *Scheduler) applyRetryTask(ctx context.Context, command *Command, now time.Time) error {
	task, err := s.tasks.GetTask(ctx, command.TaskID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil
		}
		return err
	}
	switch task.Status {
	case TaskFailed, TaskCanceled:
		_, err = s.tasks.RetryTask(ctx, task.ID, command.Reason, now)
		if errors.Is(err, ErrTaskNotRetryable) {
			return nil
		}
		return err
	case TaskPending, TaskRunning, TaskCompleted:
		return nil
	default:
		return nil
	}
}

func (s *Scheduler) registerActiveClaim(claim *ClaimedTask, cancel context.CancelCauseFunc) {
	if claim == nil || claim.Task == nil || cancel == nil {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	s.active[claim.Task.ID] = activeTaskRun{claim: claim, cancel: cancel}
}

func (s *Scheduler) unregisterActiveClaim(claim *ClaimedTask) {
	if claim == nil || claim.Task == nil {
		return
	}
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	delete(s.active, claim.Task.ID)
}

func (s *Scheduler) activeRun(taskID string) (activeTaskRun, bool) {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	active, ok := s.active[taskID]
	return active, ok
}

func (s *Scheduler) activeTaskIDs() []string {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	if len(s.active) == 0 {
		return nil
	}
	ids := make([]string, 0, len(s.active))
	for taskID := range s.active {
		ids = append(ids, taskID)
	}
	return ids
}

func (s *Scheduler) localActiveRun(task *Task) (activeTaskRun, bool) {
	if task == nil || task.Run == nil {
		return activeTaskRun{}, false
	}
	active, ok := s.activeRun(task.ID)
	if !ok || active.claim == nil || active.claim.Run == nil {
		return activeTaskRun{}, false
	}
	if active.claim.Run.ID != task.Run.ID {
		return activeTaskRun{}, false
	}
	return active, true
}

func (s *Scheduler) hasConflictingLocalRun(claim *ClaimedTask) bool {
	if claim == nil || claim.Task == nil || claim.Run == nil {
		return false
	}
	active, ok := s.activeRun(claim.Task.ID)
	if !ok || active.claim == nil || active.claim.Run == nil {
		return false
	}
	return active.claim.Run.ID != "" && active.claim.Run.ID != claim.Run.ID
}

func (s *Scheduler) cancelRemoteRun(ctx context.Context, task *Task, run *RunRef, cause error) error {
	if run == nil {
		return ErrRunNotFound
	}
	controller, ok := s.runner.(RunController)
	if !ok {
		return ErrRunControlUnavailable
	}
	return controller.CancelRun(ctx, task, run, cause)
}

func inferCommandStore(tasks TaskStore) CommandStore {
	if commands, ok := tasks.(CommandStore); ok {
		return commands
	}
	return nil
}
