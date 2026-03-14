package orchestrator

import (
	"context"
	"time"
)

// LeaseRecovery describes a lease recovered after its owner stopped renewing it.
// Task is the pre-recovery task snapshot, including the abandoned Run ref.
type LeaseRecovery struct {
	Task         *Task
	Lease        *Lease
	RecoveredAt  time.Time
	ResultStatus TaskStatus
	Requeued     bool
	Reason       string
}

// CommandRecovery describes a claimed command returned to pending after recovery.
type CommandRecovery struct {
	Command     *Command
	RecoveredAt time.Time
	ReleasedBy  string
}

// LeaseRecoveryStore reclaims expired task leases after worker loss or restart.
type LeaseRecoveryStore interface {
	RecoverExpiredLeases(ctx context.Context, now time.Time) ([]*LeaseRecovery, error)
}

// CommandRecoveryStore reclaims stranded claimed commands after worker loss or restart.
type CommandRecoveryStore interface {
	RecoverClaimedCommands(ctx context.Context, claimedBefore, now time.Time) ([]*CommandRecovery, error)
}
