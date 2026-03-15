package orchestrator

import (
	"context"
	"errors"
	"time"
)

// RecoverySweep summarizes one orchestrator recovery pass.
type RecoverySweep struct {
	RecoveredAt       time.Time
	LeaseRecoveries   []*LeaseRecovery
	CommandRecoveries []*CommandRecovery
	LocalRunCancels   int
	RemoteRunCancels  int
}

// RecoveryOption customizes a RecoveryManager.
type RecoveryOption func(*RecoveryManager)

// RecoveryManager runs lease/command recovery independently of Scheduler.
// It reclaims store state, optionally cancels local runs, and optionally
// propagates recovery cancels to durable runners through RunController.
type RecoveryManager struct {
	tasks               TaskStore
	commands            CommandStore
	controller          RunController
	commandClaimTimeout time.Duration
	cancelLocalRun      func(task *Task, cause error) bool
	onError             func(error)
}

// NewRecoveryManager builds a reusable recovery runner for orchestrator state.
func NewRecoveryManager(tasks TaskStore, commands CommandStore, opts ...RecoveryOption) *RecoveryManager {
	if commands == nil {
		commands = inferCommandStore(tasks)
	}
	manager := &RecoveryManager{
		tasks:    tasks,
		commands: commands,
		onError:  func(error) {},
	}
	for _, opt := range opts {
		opt(manager)
	}
	return manager
}

// WithRecoveryController enables out-of-band run cancellation during recovery.
func WithRecoveryController(controller RunController) RecoveryOption {
	return func(m *RecoveryManager) {
		m.controller = controller
	}
}

// WithRecoveryCommandClaimTimeout configures stale claimed-command recovery.
func WithRecoveryCommandClaimTimeout(timeout time.Duration) RecoveryOption {
	return func(m *RecoveryManager) {
		m.commandClaimTimeout = timeout
	}
}

// WithRecoveryLocalCanceler lets recovery stop a still-running local task attempt.
func WithRecoveryLocalCanceler(fn func(task *Task, cause error) bool) RecoveryOption {
	return func(m *RecoveryManager) {
		m.cancelLocalRun = fn
	}
}

// WithRecoveryErrorHandler receives non-fatal remote run control failures.
func WithRecoveryErrorHandler(fn func(error)) RecoveryOption {
	return func(m *RecoveryManager) {
		if fn == nil {
			m.onError = func(error) {}
			return
		}
		m.onError = fn
	}
}

// Sweep runs one lease/command recovery pass and returns a summary.
func (m *RecoveryManager) Sweep(ctx context.Context, now time.Time) (*RecoverySweep, error) {
	now = normalizeRecoveryNow(now)
	sweep := &RecoverySweep{RecoveredAt: now}

	if recoverer, ok := m.tasks.(LeaseRecoveryStore); ok {
		recovered, err := recoverer.RecoverExpiredLeases(ctx, now)
		if err != nil {
			return nil, err
		}
		sweep.LeaseRecoveries = recovered
		for _, leaseRecovery := range recovered {
			if leaseRecovery == nil || leaseRecovery.Task == nil || leaseRecovery.Task.Run == nil {
				continue
			}
			if m.cancelLocalRun != nil && m.cancelLocalRun(leaseRecovery.Task, ErrLeaseExpired) {
				sweep.LocalRunCancels++
				continue
			}
			if m.controller == nil {
				continue
			}
			if err := m.controller.CancelRun(ctx, leaseRecovery.Task, leaseRecovery.Task.Run, ErrLeaseExpired); err != nil {
				if errors.Is(err, ErrRunControlUnavailable) || errors.Is(err, ErrRunNotFound) {
					continue
				}
				m.onError(err)
				continue
			}
			sweep.RemoteRunCancels++
		}
	}

	if m.commandClaimTimeout > 0 {
		if recoverer, ok := m.commands.(CommandRecoveryStore); ok {
			recovered, err := recoverer.RecoverClaimedCommands(ctx, now.Add(-m.commandClaimTimeout), now)
			if err != nil {
				return nil, err
			}
			sweep.CommandRecoveries = recovered
		}
	}

	return sweep, nil
}

func normalizeRecoveryNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
