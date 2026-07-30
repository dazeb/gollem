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
	if s == nil {
		return func() {}, errors.New("appserver: nil server")
	}
	s.workspaceMutationMu.Lock()
	if s.workspaceRevert != nil {
		s.workspaceMutationMu.Unlock()
		return func() {}, ErrWorkspaceRevertInProgress
	}
	var once sync.Once
	return func() {
		once.Do(s.workspaceMutationMu.Unlock)
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
	if s == nil || st == nil {
		return func() {}, errors.New("appserver: workspace revert coordination is unavailable")
	}
	canonicalRoot, err := canonicalExistingPath(root)
	if err != nil {
		return func() {}, fmt.Errorf("resolve revert workspace: %w", err)
	}

	s.workspaceMutationMu.Lock()
	if s.workspaceRevert != nil {
		s.workspaceMutationMu.Unlock()
		return func() {}, ErrWorkspaceRevertInProgress
	}
	reservation := &workspaceRevertReservation{root: canonicalRoot}
	s.workspaceRevert = reservation
	activeTurns, err := st.ListTurns(ctx, store.TurnFilter{
		Statuses: []store.TurnStatus{store.TurnQueued, store.TurnRunning},
	})
	if err != nil || len(activeTurns) > 0 {
		s.workspaceRevert = nil
		s.workspaceMutationMu.Unlock()
		if err != nil {
			return func() {}, err
		}
		return func() {}, ErrWorkspaceTurnActive
	}
	s.workspaceMutationMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.workspaceMutationMu.Lock()
			if s.workspaceRevert == reservation {
				s.workspaceRevert = nil
			}
			s.workspaceMutationMu.Unlock()
		})
	}, nil
}
