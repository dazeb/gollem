package toolproxy

import (
	"sort"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// discoveredState holds the set of deferred tool names that the model has
// loaded via tool_search in the current run. It implements core.StatefulTool
// so the set is captured in checkpoints and restored on resume.
//
// ExportState returns a deterministic sorted slice (stable for tests and
// golden-file comparisons) instead of a map, and RestoreState accepts any
// of: []string, []any (post-JSON roundtrip), map[string]any with a
// "discovered" key.
//
// It also carries a transient "last seen tool pool" — the full tool list
// that the most recent PrepareFunc invocation recorded, including both
// deferred and non-deferred entries. Keeping non-deferred tools in the
// pool lets the tool_search handler fall back to the full list for
// exact-match / select: lookups (matching Claude Code's harmless-no-op
// behavior when the model asks for a tool that is already loaded). The
// pool is process-local runtime state and is NOT persisted by
// ExportState: a restored checkpoint starts with an empty pool that
// refills on the next PrepareFunc call.
type discoveredState struct {
	mu    sync.Mutex
	names map[string]struct{}
	pool  []core.Tool
}

func newDiscoveredState() *discoveredState {
	return &discoveredState{names: make(map[string]struct{})}
}

// setPool records the current pool of tools (deferred + non-deferred)
// for later lookup by the tool_search handler.
func (s *discoveredState) setPool(pool []core.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pool = pool
}

// getPool returns the most recently recorded pool, or nil if none.
func (s *discoveredState) getPool() []core.Tool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pool
}

// add records a tool name as discovered. Returns true if it was new.
func (s *discoveredState) add(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.names[name]; ok {
		return false
	}
	s.names[name] = struct{}{}
	return true
}

// contains reports whether the name has been discovered.
func (s *discoveredState) contains(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.names[name]
	return ok
}

// snapshot returns a sorted copy of the discovered set.
func (s *discoveredState) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.names))
	for n := range s.names {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// reset clears the discovered set. Used by tests and when a new run starts
// with a clean slate.
func (s *discoveredState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = make(map[string]struct{})
}

// ExportState implements core.StatefulTool. Returns a map form so the shape
// is forward-compatible with future state additions.
func (s *discoveredState) ExportState() (any, error) {
	return map[string]any{"discovered": s.snapshot()}, nil
}

// RestoreState implements core.StatefulTool. Accepts the shapes ExportState
// can produce before and after a JSON roundtrip through checkpoints.
func (s *discoveredState) RestoreState(state any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = make(map[string]struct{})

	switch v := state.(type) {
	case nil:
		return nil
	case []string:
		for _, n := range v {
			s.names[n] = struct{}{}
		}
	case []any:
		for _, item := range v {
			if name, ok := item.(string); ok {
				s.names[name] = struct{}{}
			}
		}
	case map[string]any:
		raw, ok := v["discovered"]
		if !ok {
			return nil
		}
		switch d := raw.(type) {
		case []string:
			for _, n := range d {
				s.names[n] = struct{}{}
			}
		case []any:
			for _, item := range d {
				if name, ok := item.(string); ok {
					s.names[name] = struct{}{}
				}
			}
		}
	}
	return nil
}
