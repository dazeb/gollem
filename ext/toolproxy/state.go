package toolproxy

import (
	"sort"
	"strings"
	"sync"

	"github.com/fugue-labs/gollem/core"
)

// scoringEntry caches the precomputed per-tool strings the scorer needs
// on every search: the parsed name (CamelCase / snake_case / MCP
// tokenisation), the lowercased description, and the lowercased search
// hint. Parsing a name and lowercasing a description are cheap
// individually, but at hundreds of deferred tools times multiple search
// terms per query the allocations add up — the cache reduces the hot
// loop to map lookups.
type scoringEntry struct {
	parsed    parsedName
	descLower string
	hintLower string
}

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
//
// Alongside the pool we keep a scoring cache, also transient and
// invalidated whenever the pool "signature" (sorted joined tool names)
// changes.
type discoveredState struct {
	mu           sync.Mutex
	names        map[string]struct{}
	pool         []core.Tool
	poolKey      string
	scoringCache map[string]scoringEntry

	// announcedInSystemPrompt is a transient flag set by
	// SystemPromptFuncFor after it emits the "Deferred Tools" fragment
	// for the first time. Subsequent calls skip the fragment to achieve
	// a delta announcement. It is NOT persisted by ExportState — a
	// restored checkpoint re-announces, which is safe: the model may
	// not have seen the pre-restore system prompt in its context.
	announcedInSystemPrompt bool
}

func newDiscoveredState() *discoveredState {
	return &discoveredState{names: make(map[string]struct{})}
}

// setPool records the current pool of tools (deferred + non-deferred)
// for later lookup by the tool_search handler. If the pool's signature
// (sorted, joined names) changed since the last call, the scoring cache
// is dropped so stale entries can't leak, and the announcement flag is
// cleared so the delta announcer re-emits the updated list.
func (s *discoveredState) setPool(pool []core.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := poolSignature(pool)
	if key != s.poolKey {
		s.scoringCache = nil
		s.announcedInSystemPrompt = false
		s.poolKey = key
	}
	s.pool = pool
}

// claimSystemPromptAnnouncement reports whether the caller should emit
// the full "Deferred Tools" fragment this turn. On the first call it
// returns true and marks the flag so subsequent calls return false.
// Safe for concurrent use.
func (s *discoveredState) claimSystemPromptAnnouncement() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.announcedInSystemPrompt {
		return false
	}
	s.announcedInSystemPrompt = true
	return true
}

// getPool returns the most recently recorded pool, or nil if none.
func (s *discoveredState) getPool() []core.Tool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pool
}

// lookupScoring returns a cached scoringEntry for the given tool,
// building it lazily on first access. Safe for concurrent use.
func (s *discoveredState) lookupScoring(tool core.Tool) scoringEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := tool.Definition.Name
	if entry, ok := s.scoringCache[name]; ok {
		return entry
	}
	entry := scoringEntry{
		parsed:    parseToolName(name),
		descLower: strings.ToLower(tool.Definition.Description),
		hintLower: strings.ToLower(tool.SearchHint),
	}
	if s.scoringCache == nil {
		s.scoringCache = make(map[string]scoringEntry)
	}
	s.scoringCache[name] = entry
	return entry
}

// poolSignature produces a stable key for a pool based on the sorted
// set of tool names. Used to invalidate the scoring cache when the pool
// actually changes, rather than on every setPool call.
func poolSignature(pool []core.Tool) string {
	if len(pool) == 0 {
		return ""
	}
	names := make([]string, len(pool))
	for i, t := range pool {
		names[i] = t.Definition.Name
	}
	sort.Strings(names)
	return strings.Join(names, "\x00")
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

// reset clears the discovered set and the system-prompt announcement
// flag. Used by tests and when a new run starts with a clean slate.
func (s *discoveredState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = make(map[string]struct{})
	s.announcedInSystemPrompt = false
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
