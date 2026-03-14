package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fugue-labs/gollem/core"
	"github.com/fugue-labs/gollem/ext/orchestrator"
)

// CreateCommand implements orchestrator.CommandStore.
func (s *Store) CreateCommand(_ context.Context, req orchestrator.CreateCommandRequest) (*orchestrator.Command, error) {
	if req.TaskID == "" {
		return nil, orchestrator.ErrTaskNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasks[req.TaskID]
	if !ok {
		return nil, orchestrator.ErrTaskNotFound
	}

	runID, targetWorkerID, err := s.validateCommandTargetLocked(task, req)
	if err != nil {
		return nil, err
	}

	s.nextCommand++
	now := time.Now()
	command := &orchestrator.Command{
		ID:             fmt.Sprintf("command-%d", s.nextCommand),
		Kind:           req.Kind,
		TaskID:         req.TaskID,
		RunID:          runID,
		TargetWorkerID: targetWorkerID,
		Reason:         req.Reason,
		Metadata:       cloneAnyMap(req.Metadata),
		Status:         orchestrator.CommandPending,
		CreatedAt:      now,
	}
	s.commands[command.ID] = command
	s.commandOrder = append(s.commandOrder, command.ID)
	s.publishCommandCreated(command)
	return cloneCommand(command), nil
}

// GetCommand implements orchestrator.CommandStore.
func (s *Store) GetCommand(_ context.Context, id string) (*orchestrator.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	command, ok := s.commands[id]
	if !ok {
		return nil, orchestrator.ErrCommandNotFound
	}
	return cloneCommand(command), nil
}

// ListCommands implements orchestrator.CommandStore.
func (s *Store) ListCommands(_ context.Context, filter orchestrator.CommandFilter) ([]*orchestrator.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var commands []*orchestrator.Command
	for _, id := range s.commandOrder {
		command, ok := s.commands[id]
		if !ok || !matchesCommandFilter(command, filter) {
			continue
		}
		commands = append(commands, cloneCommand(command))
	}
	return commands, nil
}

// ClaimPendingCommand implements orchestrator.CommandStore.
func (s *Store) ClaimPendingCommand(_ context.Context, req orchestrator.ClaimCommandRequest) (*orchestrator.Command, error) {
	if req.WorkerID == "" {
		return nil, errors.New("orchestrator/memory: command claim worker id must not be empty")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, id := range s.commandOrder {
		command, ok := s.commands[id]
		if !ok || command.Status != orchestrator.CommandPending {
			continue
		}
		s.refreshCommandTargetLocked(command)
		if command.TargetWorkerID != "" && command.TargetWorkerID != req.WorkerID {
			continue
		}
		command.Status = orchestrator.CommandClaimed
		command.ClaimedBy = req.WorkerID
		command.ClaimToken = fmt.Sprintf("%s-claim-%d", command.ID, now.UnixNano())
		command.ClaimedAt = now
		s.publishCommandClaimed(command)
		return cloneCommand(command), nil
	}
	return nil, orchestrator.ErrNoPendingCommand
}

// HandleCommand implements orchestrator.CommandStore.
func (s *Store) HandleCommand(_ context.Context, id, claimToken, handledBy string, now time.Time) (*orchestrator.Command, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	command, ok := s.commands[id]
	if !ok {
		return nil, orchestrator.ErrCommandNotFound
	}
	if command.Status != orchestrator.CommandClaimed || command.ClaimToken != claimToken {
		return nil, orchestrator.ErrCommandClaimMismatch
	}
	if now.IsZero() {
		now = time.Now()
	}
	command.Status = orchestrator.CommandHandled
	command.HandledBy = handledBy
	command.HandledAt = now
	command.ClaimToken = ""
	s.publishCommandHandled(command)
	return cloneCommand(command), nil
}

// ReleaseCommand implements orchestrator.CommandStore.
func (s *Store) ReleaseCommand(_ context.Context, id, claimToken string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	command, ok := s.commands[id]
	if !ok {
		return orchestrator.ErrCommandNotFound
	}
	if command.Status != orchestrator.CommandClaimed || command.ClaimToken != claimToken {
		return orchestrator.ErrCommandClaimMismatch
	}
	releasedBy := command.ClaimedBy
	releasedAt := time.Now().UTC()
	command.Status = orchestrator.CommandPending
	command.ClaimedBy = ""
	command.ClaimToken = ""
	command.ClaimedAt = time.Time{}
	s.publishCommandReleased(command, releasedBy, releasedAt)
	return nil
}

func (s *Store) validateCommandTargetLocked(task *orchestrator.Task, req orchestrator.CreateCommandRequest) (runID, targetWorkerID string, err error) {
	if task == nil {
		return "", "", orchestrator.ErrTaskNotFound
	}
	runID = req.RunID
	if runID == "" && task.Run != nil {
		runID = task.Run.ID
	}

	switch req.Kind {
	case orchestrator.CommandCancelTask:
		switch task.Status {
		case orchestrator.TaskPending:
			return runID, "", nil
		case orchestrator.TaskRunning:
			if task.Run == nil || task.Run.WorkerID == "" {
				return "", "", orchestrator.ErrInvalidCommand
			}
			return runID, task.Run.WorkerID, nil
		default:
			return "", "", orchestrator.ErrTaskNotCancelable
		}
	case orchestrator.CommandRetryTask:
		switch task.Status {
		case orchestrator.TaskFailed, orchestrator.TaskCanceled:
			return runID, "", nil
		default:
			return "", "", orchestrator.ErrTaskNotRetryable
		}
	default:
		return "", "", orchestrator.ErrInvalidCommand
	}
}

func (s *Store) refreshCommandTargetLocked(command *orchestrator.Command) {
	if command == nil {
		return
	}
	task, ok := s.tasks[command.TaskID]
	if !ok {
		command.TargetWorkerID = ""
		command.RunID = ""
		return
	}

	switch command.Kind {
	case orchestrator.CommandCancelTask:
		switch task.Status {
		case orchestrator.TaskRunning:
			if task.Run != nil {
				command.RunID = task.Run.ID
				command.TargetWorkerID = task.Run.WorkerID
				return
			}
		case orchestrator.TaskPending:
			command.RunID = ""
			command.TargetWorkerID = ""
			return
		}
	case orchestrator.CommandRetryTask:
		command.TargetWorkerID = ""
		if task.Run == nil {
			command.RunID = ""
		}
		return
	}
}

func matchesCommandFilter(command *orchestrator.Command, filter orchestrator.CommandFilter) bool {
	if command == nil {
		return false
	}
	if filter.TaskID != "" && command.TaskID != filter.TaskID {
		return false
	}
	if filter.RunID != "" && command.RunID != filter.RunID {
		return false
	}
	if filter.TargetWorkerID != "" && command.TargetWorkerID != filter.TargetWorkerID {
		return false
	}
	if len(filter.Kinds) > 0 {
		matched := false
		for _, kind := range filter.Kinds {
			if command.Kind == kind {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(filter.Statuses) > 0 {
		matched := false
		for _, status := range filter.Statuses {
			if command.Status == status {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func cloneCommand(src *orchestrator.Command) *orchestrator.Command {
	if src == nil {
		return nil
	}
	return &orchestrator.Command{
		ID:             src.ID,
		Kind:           src.Kind,
		TaskID:         src.TaskID,
		RunID:          src.RunID,
		TargetWorkerID: src.TargetWorkerID,
		Reason:         src.Reason,
		Metadata:       cloneAnyMap(src.Metadata),
		Status:         src.Status,
		ClaimToken:     src.ClaimToken,
		ClaimedBy:      src.ClaimedBy,
		HandledBy:      src.HandledBy,
		CreatedAt:      src.CreatedAt,
		ClaimedAt:      src.ClaimedAt,
		HandledAt:      src.HandledAt,
	}
}

func (s *Store) publishCommandCreated(command *orchestrator.Command) {
	if s.eventBus == nil || command == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.CommandCreatedEvent{
		CommandID:      command.ID,
		Kind:           command.Kind,
		TaskID:         command.TaskID,
		RunID:          command.RunID,
		TargetWorkerID: command.TargetWorkerID,
		CreatedAt:      command.CreatedAt,
	})
}

func (s *Store) publishCommandClaimed(command *orchestrator.Command) {
	if s.eventBus == nil || command == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.CommandClaimedEvent{
		CommandID: command.ID,
		Kind:      command.Kind,
		TaskID:    command.TaskID,
		RunID:     command.RunID,
		ClaimedBy: command.ClaimedBy,
		ClaimedAt: command.ClaimedAt,
	})
}

func (s *Store) publishCommandReleased(command *orchestrator.Command, releasedBy string, releasedAt time.Time) {
	if s.eventBus == nil || command == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.CommandReleasedEvent{
		CommandID:  command.ID,
		Kind:       command.Kind,
		TaskID:     command.TaskID,
		RunID:      command.RunID,
		ReleasedBy: releasedBy,
		ReleasedAt: releasedAt,
	})
}

func (s *Store) publishCommandHandled(command *orchestrator.Command) {
	if s.eventBus == nil || command == nil {
		return
	}
	core.PublishAsync(s.eventBus, orchestrator.CommandHandledEvent{
		CommandID: command.ID,
		Kind:      command.Kind,
		TaskID:    command.TaskID,
		RunID:     command.RunID,
		HandledBy: command.HandledBy,
		HandledAt: command.HandledAt,
	})
}
