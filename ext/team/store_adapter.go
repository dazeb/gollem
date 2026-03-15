package team

import (
	"context"
	"errors"
	"time"

	"github.com/fugue-labs/gollem/ext/orchestrator"
)

// teamWorkerStore scopes orchestrator claims to team tasks visible to one teammate.
type teamWorkerStore struct {
	team     *Team
	workerID string
}

var (
	_ orchestrator.TaskStore            = (*teamWorkerStore)(nil)
	_ orchestrator.LeaseStore           = (*teamWorkerStore)(nil)
	_ orchestrator.CommandStore         = (*teamWorkerStore)(nil)
	_ orchestrator.ArtifactStore        = (*teamWorkerStore)(nil)
	_ orchestrator.LeaseRecoveryStore   = (*teamWorkerStore)(nil)
	_ orchestrator.CommandRecoveryStore = (*teamWorkerStore)(nil)
)

func newTeamWorkerStore(team *Team, workerID string) *teamWorkerStore {
	return &teamWorkerStore{team: team, workerID: workerID}
}

func (s *teamWorkerStore) CreateTask(ctx context.Context, req orchestrator.CreateTaskRequest) (*orchestrator.Task, error) {
	return s.team.store.CreateTask(ctx, req)
}

func (s *teamWorkerStore) GetTask(ctx context.Context, id string) (*orchestrator.Task, error) {
	return s.team.getTeamTask(ctx, id)
}

func (s *teamWorkerStore) ListTasks(ctx context.Context, filter orchestrator.TaskFilter) ([]*orchestrator.Task, error) {
	return s.team.listTeamTasks(ctx, filter)
}

func (s *teamWorkerStore) ClaimReadyTask(ctx context.Context, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	tasks, err := s.team.listTeamTasks(ctx, orchestrator.TaskFilter{
		Statuses: []orchestrator.TaskStatus{orchestrator.TaskPending},
	})
	if err != nil {
		return nil, err
	}

	tryClaim := func(task *orchestrator.Task) (*orchestrator.ClaimedTask, error) {
		if task == nil || containsTaskID(req.ExcludeTaskIDs, task.ID) {
			return nil, nil
		}
		claim, claimErr := s.team.store.ClaimTask(ctx, task.ID, req)
		if claimErr == nil {
			return claim, nil
		}
		if errors.Is(claimErr, orchestrator.ErrNoReadyTask) ||
			errors.Is(claimErr, orchestrator.ErrTaskBlocked) ||
			errors.Is(claimErr, orchestrator.ErrTaskNotFound) {
			return nil, nil
		}
		return nil, claimErr
	}

	for _, task := range tasks {
		if teamTaskAssignee(task) != s.workerID {
			continue
		}
		claim, claimErr := tryClaim(task)
		if claimErr != nil {
			return nil, claimErr
		}
		if claim != nil {
			return claim, nil
		}
	}

	for _, task := range tasks {
		if assignee := teamTaskAssignee(task); assignee != "" {
			continue
		}
		claim, claimErr := tryClaim(task)
		if claimErr != nil {
			return nil, claimErr
		}
		if claim != nil {
			return claim, nil
		}
	}

	return nil, orchestrator.ErrNoReadyTask
}

func (s *teamWorkerStore) ClaimTask(ctx context.Context, taskID string, req orchestrator.ClaimTaskRequest) (*orchestrator.ClaimedTask, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.team.store.ClaimTask(ctx, taskID, req)
}

func (s *teamWorkerStore) UpdateTask(ctx context.Context, req orchestrator.UpdateTaskRequest) (*orchestrator.Task, error) {
	if _, err := s.team.getTeamTask(ctx, req.ID); err != nil {
		return nil, err
	}
	return s.team.store.UpdateTask(ctx, req)
}

func (s *teamWorkerStore) DeleteTask(ctx context.Context, id string) error {
	if _, err := s.team.getTeamTask(ctx, id); err != nil {
		return err
	}
	return s.team.store.DeleteTask(ctx, id)
}

func (s *teamWorkerStore) CompleteTask(ctx context.Context, taskID, leaseToken string, outcome *orchestrator.TaskOutcome, now time.Time) (*orchestrator.Task, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	task, err := s.team.store.CompleteTask(ctx, taskID, leaseToken, outcome, now)
	if err == nil {
		s.markTaskSettled(taskID)
	}
	return task, err
}

func (s *teamWorkerStore) FailTask(ctx context.Context, taskID, leaseToken string, runErr error, now time.Time) (*orchestrator.Task, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	task, err := s.team.store.FailTask(ctx, taskID, leaseToken, runErr, now)
	if err == nil {
		s.markTaskSettled(taskID)
	}
	return task, err
}

func (s *teamWorkerStore) CancelTask(ctx context.Context, taskID, leaseToken, reason string, now time.Time) (*orchestrator.Task, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	task, err := s.team.store.CancelTask(ctx, taskID, leaseToken, reason, now)
	if err == nil {
		s.markTaskSettled(taskID)
	}
	return task, err
}

func (s *teamWorkerStore) RetryTask(ctx context.Context, taskID, reason string, now time.Time) (*orchestrator.Task, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.team.store.RetryTask(ctx, taskID, reason, now)
}

func (s *teamWorkerStore) GetLease(ctx context.Context, taskID string) (*orchestrator.Lease, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.team.store.GetLease(ctx, taskID)
}

func (s *teamWorkerStore) RenewLease(ctx context.Context, taskID, leaseToken string, ttl time.Duration, now time.Time) (*orchestrator.Lease, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.team.store.RenewLease(ctx, taskID, leaseToken, ttl, now)
}

func (s *teamWorkerStore) ReleaseLease(ctx context.Context, taskID, leaseToken string) error {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return err
	}
	err := s.team.store.ReleaseLease(ctx, taskID, leaseToken)
	if err == nil {
		s.markTaskSettled(taskID)
	}
	return err
}

func (s *teamWorkerStore) CreateCommand(ctx context.Context, req orchestrator.CreateCommandRequest) (*orchestrator.Command, error) {
	return s.team.store.CreateCommand(ctx, req)
}

func (s *teamWorkerStore) GetCommand(ctx context.Context, id string) (*orchestrator.Command, error) {
	command, err := s.team.store.GetCommand(ctx, id)
	if err != nil {
		return nil, err
	}
	if !s.commandVisible(ctx, command) {
		return nil, orchestrator.ErrCommandNotFound
	}
	return command, nil
}

func (s *teamWorkerStore) ListCommands(ctx context.Context, filter orchestrator.CommandFilter) ([]*orchestrator.Command, error) {
	commands, err := s.team.store.ListCommands(ctx, filter)
	if err != nil {
		return nil, err
	}
	filtered := make([]*orchestrator.Command, 0, len(commands))
	for _, command := range commands {
		if s.commandVisible(ctx, command) {
			filtered = append(filtered, command)
		}
	}
	return filtered, nil
}

func (s *teamWorkerStore) ClaimPendingCommand(ctx context.Context, req orchestrator.ClaimCommandRequest) (*orchestrator.Command, error) {
	commands, err := orchestrator.ListPendingCommandsForWorker(ctx, s.team.store, req.WorkerID)
	if err != nil {
		return nil, err
	}
	for _, command := range commands {
		if !s.commandVisible(ctx, command) {
			continue
		}
		claimed, claimErr := s.team.store.ClaimCommand(ctx, command.ID, req)
		if claimErr == nil {
			return claimed, nil
		}
		if errors.Is(claimErr, orchestrator.ErrNoPendingCommand) ||
			errors.Is(claimErr, orchestrator.ErrCommandNotFound) {
			continue
		}
		return nil, claimErr
	}
	return nil, orchestrator.ErrNoPendingCommand
}

func (s *teamWorkerStore) ClaimCommand(ctx context.Context, id string, req orchestrator.ClaimCommandRequest) (*orchestrator.Command, error) {
	if _, err := s.GetCommand(ctx, id); err != nil {
		return nil, err
	}
	return s.team.store.ClaimCommand(ctx, id, req)
}

func (s *teamWorkerStore) HandleCommand(ctx context.Context, id, claimToken, handledBy string, now time.Time) (*orchestrator.Command, error) {
	if _, err := s.GetCommand(ctx, id); err != nil {
		return nil, err
	}
	return s.team.store.HandleCommand(ctx, id, claimToken, handledBy, now)
}

func (s *teamWorkerStore) ReleaseCommand(ctx context.Context, id, claimToken string) error {
	if _, err := s.GetCommand(ctx, id); err != nil {
		return err
	}
	return s.team.store.ReleaseCommand(ctx, id, claimToken)
}

func (s *teamWorkerStore) RecoverExpiredLeases(ctx context.Context, now time.Time) ([]*orchestrator.LeaseRecovery, error) {
	now = normalizeRecoveryTime(now)
	tasks, err := s.team.listTeamTasks(ctx, orchestrator.TaskFilter{})
	if err != nil {
		return nil, err
	}
	recovered := make([]*orchestrator.LeaseRecovery, 0)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		lease, leaseErr := s.team.store.GetLease(ctx, task.ID)
		if leaseErr != nil {
			if errors.Is(leaseErr, orchestrator.ErrLeaseNotFound) {
				continue
			}
			return nil, leaseErr
		}
		if lease.ExpiresAt.After(now) {
			continue
		}
		recovery, recoveryErr := s.team.store.RecoverExpiredLease(ctx, task.ID, now)
		if recoveryErr != nil {
			return nil, recoveryErr
		}
		if recovery != nil {
			recovered = append(recovered, recovery)
		}
	}
	return recovered, nil
}

func (s *teamWorkerStore) RecoverExpiredLease(ctx context.Context, taskID string, now time.Time) (*orchestrator.LeaseRecovery, error) {
	return s.recoverExpiredLease(ctx, taskID, now)
}

func (s *teamWorkerStore) RecoverClaimedCommands(ctx context.Context, claimedBefore, now time.Time) ([]*orchestrator.CommandRecovery, error) {
	now = normalizeRecoveryTime(now)
	if claimedBefore.IsZero() {
		claimedBefore = now
	}
	commands, err := s.ListCommands(ctx, orchestrator.CommandFilter{
		Statuses: []orchestrator.CommandStatus{orchestrator.CommandClaimed},
	})
	if err != nil {
		return nil, err
	}
	recovered := make([]*orchestrator.CommandRecovery, 0)
	for _, command := range commands {
		if command == nil || command.ClaimedAt.IsZero() || command.ClaimedAt.After(claimedBefore) {
			continue
		}
		recovery, recoveryErr := s.team.store.RecoverClaimedCommand(ctx, command.ID, claimedBefore, now)
		if recoveryErr != nil {
			return nil, recoveryErr
		}
		if recovery != nil {
			recovered = append(recovered, recovery)
		}
	}
	return recovered, nil
}

func (s *teamWorkerStore) RecoverClaimedCommand(ctx context.Context, id string, claimedBefore, now time.Time) (*orchestrator.CommandRecovery, error) {
	return s.recoverClaimedCommand(ctx, id, claimedBefore, now)
}

func (s *teamWorkerStore) CreateArtifact(ctx context.Context, req orchestrator.CreateArtifactRequest) (*orchestrator.Artifact, error) {
	return s.team.store.CreateArtifact(ctx, req)
}

func (s *teamWorkerStore) GetArtifact(ctx context.Context, id string) (*orchestrator.Artifact, error) {
	return s.team.store.GetArtifact(ctx, id)
}

func (s *teamWorkerStore) ListArtifacts(ctx context.Context, filter orchestrator.ArtifactFilter) ([]*orchestrator.Artifact, error) {
	return s.team.store.ListArtifacts(ctx, filter)
}

func containsTaskID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func (s *teamWorkerStore) markTaskSettled(taskID string) {
	if taskID == "" {
		return
	}
	tm := s.team.GetTeammate(s.workerID)
	if tm == nil {
		return
	}
	tm.markTaskSettled(taskID)
}

func (s *teamWorkerStore) commandVisible(ctx context.Context, command *orchestrator.Command) bool {
	if command == nil || command.TaskID == "" {
		return false
	}
	_, err := s.team.getTeamTask(ctx, command.TaskID)
	return err == nil
}

func (s *teamWorkerStore) recoverExpiredLease(ctx context.Context, taskID string, now time.Time) (*orchestrator.LeaseRecovery, error) {
	if _, err := s.team.getTeamTask(ctx, taskID); err != nil {
		return nil, err
	}
	return s.team.store.RecoverExpiredLease(ctx, taskID, now)
}

func (s *teamWorkerStore) recoverClaimedCommand(ctx context.Context, id string, claimedBefore, now time.Time) (*orchestrator.CommandRecovery, error) {
	if _, err := s.GetCommand(ctx, id); err != nil {
		return nil, err
	}
	return s.team.store.RecoverClaimedCommand(ctx, id, claimedBefore, now)
}

func normalizeRecoveryTime(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
