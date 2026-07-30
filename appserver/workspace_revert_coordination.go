package appserver

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/fugue-labs/gollem/appserver/store"
)

var (
	ErrWorkspaceRevertInProgress = errors.New("appserver: workspace file-change revert is in progress")
	ErrWorkspaceTurnActive       = errors.New("appserver: workspace turn is active")
)

type workspaceRevertReservation struct {
	root string
}

// WorkspaceMutationCoordinator owns the daemon-wide exclusion state between
// exact reverts and operations that can start or rewrite workspace history.
type WorkspaceMutationCoordinator struct {
	mu     sync.Mutex
	revert *workspaceRevertReservation
}

func NewWorkspaceMutationCoordinator() *WorkspaceMutationCoordinator {
	return &WorkspaceMutationCoordinator{}
}

type workspaceMutationLeaseContextKey struct{}

func withWorkspaceMutationLease(ctx context.Context) context.Context {
	return context.WithValue(ctx, workspaceMutationLeaseContextKey{}, true)
}

func workspaceMutationLeaseHeld(ctx context.Context) bool {
	held, _ := ctx.Value(workspaceMutationLeaseContextKey{}).(bool)
	return held
}

// acquireWorkspaceMutationLease closes the race between checking active turns
// and reserving the filesystem for an exact revert.
func (s *Server) acquireWorkspaceMutationLease() (func(), error) {
	if s == nil || s.workspaceCoordinator == nil {
		return func() {}, errors.New("appserver: nil server")
	}
	coordinator := s.workspaceCoordinator
	coordinator.mu.Lock()
	if coordinator.revert != nil {
		coordinator.mu.Unlock()
		return func() {}, ErrWorkspaceRevertInProgress
	}
	var once sync.Once
	return func() {
		once.Do(coordinator.mu.Unlock)
	}, nil
}

// acquireTurnStartLease is implemented separately for the runtime coordinator
// interface. Callers hold it only through durable transition to running; the
// active turn then guards itself.
func (s *Server) acquireTurnStartLease() (func(), error) {
	return s.acquireWorkspaceMutationLease()
}

func (s *Server) reserveWorkspaceRevert(
	ctx context.Context,
	st store.Store,
	root string,
) (func(), error) {
	if s == nil || s.workspaceCoordinator == nil || st == nil {
		return func() {}, errors.New("appserver: workspace revert coordination is unavailable")
	}
	canonicalRoot, err := canonicalExistingPath(root)
	if err != nil {
		return func() {}, fmt.Errorf("resolve revert workspace: %w", err)
	}

	coordinator := s.workspaceCoordinator
	coordinator.mu.Lock()
	if coordinator.revert != nil {
		coordinator.mu.Unlock()
		return func() {}, ErrWorkspaceRevertInProgress
	}
	reservation := &workspaceRevertReservation{root: canonicalRoot}
	coordinator.revert = reservation
	activeTurns, err := st.ListTurns(ctx, store.TurnFilter{
		Statuses: []store.TurnStatus{store.TurnQueued, store.TurnRunning},
	})
	if err != nil || len(activeTurns) > 0 {
		coordinator.revert = nil
		coordinator.mu.Unlock()
		if err != nil {
			return func() {}, err
		}
		return func() {}, ErrWorkspaceTurnActive
	}
	coordinator.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			coordinator.mu.Lock()
			if coordinator.revert == reservation {
				coordinator.revert = nil
			}
			coordinator.mu.Unlock()
		})
	}, nil
}
