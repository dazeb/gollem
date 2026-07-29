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

// acquireTurnStartLease closes the race between checking active turns and
// reserving the filesystem for an exact revert. Callers hold the lease only
// through durable transition to running; the active turn then guards itself.
func (s *Server) acquireTurnStartLease() (func(), error) {
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
