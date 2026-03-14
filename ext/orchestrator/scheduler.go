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
	WorkerID           string
	PollInterval       time.Duration
	LeaseTTL           time.Duration
	LeaseRenewInterval time.Duration
	MaxConcurrentRuns  int
	Now                func() time.Time
	OnError            func(error)
}

// DefaultSchedulerConfig returns sane defaults for in-process orchestration.
func DefaultSchedulerConfig() SchedulerConfig {
	leaseTTL := 30 * time.Second
	return SchedulerConfig{
		WorkerID:           fmt.Sprintf("worker-%d", time.Now().UnixNano()),
		PollInterval:       100 * time.Millisecond,
		LeaseTTL:           leaseTTL,
		LeaseRenewInterval: leaseTTL / 2,
		MaxConcurrentRuns:  1,
		Now:                time.Now,
		OnError:            func(error) {},
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
	tasks  TaskStore
	leases LeaseStore
	runner Runner
	cfg    SchedulerConfig

	slots chan struct{}
	wg    sync.WaitGroup
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
		tasks:  tasks,
		leases: leases,
		runner: runner,
		cfg:    cfg,
		slots:  make(chan struct{}, cfg.MaxConcurrentRuns),
	}
}

// Run blocks until ctx is canceled or the scheduler encounters a fatal polling error.
func (s *Scheduler) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollInterval)
	defer ticker.Stop()
	defer s.wg.Wait()

	for {
		if err := s.dispatch(ctx); err != nil {
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
			WorkerID: s.cfg.WorkerID,
			LeaseTTL: s.cfg.LeaseTTL,
			Now:      s.cfg.Now(),
		})
		if err != nil {
			s.releaseSlot()
			if errors.Is(err, ErrNoReadyTask) {
				return nil
			}
			return err
		}

		s.wg.Add(1)
		go s.runClaim(ctx, claim)
	}
	return nil
}

func (s *Scheduler) runClaim(ctx context.Context, claim *ClaimedTask) {
	defer s.wg.Done()
	defer s.releaseSlot()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	updateCtx := context.WithoutCancel(runCtx)
	done := make(chan struct{})
	renewDone := make(chan struct{})
	renewErrCh := make(chan error, 1)
	go func() {
		defer close(renewDone)
		s.renewLoop(runCtx, claim, done, cancel, renewErrCh)
	}()

	result, runErr := s.runner.RunTask(runCtx, claim)
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

	now := s.cfg.Now()
	if runErr != nil {
		if _, err := s.tasks.FailTask(updateCtx, claim.Task.ID, claim.Lease.Token, runErr, now); err != nil && !isLeaseLoss(err) {
			s.cfg.OnError(err)
		}
		return
	}

	if result != nil && result.CompletedAt.IsZero() {
		result.CompletedAt = now
	}
	if _, err := s.tasks.CompleteTask(updateCtx, claim.Task.ID, claim.Lease.Token, result, now); err != nil && !isLeaseLoss(err) {
		s.cfg.OnError(err)
	}
}

func (s *Scheduler) renewLoop(ctx context.Context, claim *ClaimedTask, done <-chan struct{}, cancel context.CancelFunc, renewErrCh chan<- error) {
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
				cancel()
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
