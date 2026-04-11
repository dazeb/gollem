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

	// announcedSystemPromptNames tracks every deferred tool name that
	// SystemPromptFuncFor has emitted so far in this Proxy's lifetime.
	// On each call the announcer diffs the current deferred pool
	// against this set and emits only the additions — a proper delta
	// announcement that stays efficient even when the pool changes
	// mid-run (e.g. MCP server reconnects).
	//
	// NOT persisted by ExportState: if a run is restored with a fresh
	// Proxy, the announcer re-sends everything on its first call,
	// which is the safe default since the model may not have the old
	// system prompts in its context.
	announcedSystemPromptNames map[string]bool
}

func newDiscoveredState() *discoveredState {
	return &discoveredState{names: make(map[string]struct{})}
}

// setPool records the current pool of tools (deferred + non-deferred)
// for later lookup by the tool_search handler. If the pool's signature
// (sorted joined names of the DEFERRED subset) changed since the last
// call, the scoring cache is dropped so stale entries can't leak.
//
// The announcedSystemPromptNames set is NOT cleared here — the delta
// announcer handles additions and removals on its own, so a pool
// change simply surfaces as a non-empty delta on the next call.
func (s *discoveredState) setPool(pool []core.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := poolSignature(pool)
	if key != s.poolKey {
		s.scoringCache = nil
		s.poolKey = key
	}
	s.pool = pool
}

// diffSystemPromptAnnouncement diffs `currentDeferred` (the set of
// deferred tool names active on the most recent pool) against the
// previously-announced set. Returns `added` and `removed` (both
// sorted for deterministic output) plus `initial`, which is true if
// this call is the very first announcement on this state (i.e. the
// announced set was empty before the diff).
//
// As a side effect the call commits the delta — after it returns, the
// announced set reflects the current pool. A caller that decides NOT
// to emit the delta (e.g. a mode short-circuit) must not call this
// method, or bookkeeping will drift.
//
// Safe for concurrent use.
func (s *discoveredState) diffSystemPromptAnnouncement(currentDeferred []string) (added, removed []string, initial bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	initial = len(s.announcedSystemPromptNames) == 0
	if s.announcedSystemPromptNames == nil {
		s.announcedSystemPromptNames = make(map[string]bool)
	}

	currentSet := make(map[string]bool, len(currentDeferred))
	for _, n := range currentDeferred {
		currentSet[n] = true
		if !s.announcedSystemPromptNames[n] {
			added = append(added, n)
		}
	}
	for n := range s.announcedSystemPromptNames {
		if !currentSet[n] {
			removed = append(removed, n)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)

	// Commit: announced set now mirrors the current pool.
	s.announcedSystemPromptNames = make(map[string]bool, len(currentSet))
	for n := range currentSet {
		s.announcedSystemPromptNames[n] = true
	}
	return added, removed, initial
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

// poolSignature produces a stable key for the *effectively* deferred
// subset of a pool. Only tools with ShouldDefer=true AND AlwaysLoad=false
// participate, because those are the only tools the scoring cache
// actually holds entries for. This matches Claude Code's
// getDeferredToolsCacheKey: adding or removing a non-deferred (or
// AlwaysLoad) tool keeps the cache warm, so common scenarios like
// "caller rotates an inline base toolset while the MCP catalog stays
// put" don't thrash the cache.
func poolSignature(pool []core.Tool) string {
	var names []string
	for _, t := range pool {
		if t.ShouldDefer && !t.AlwaysLoad {
			names = append(names, t.Definition.Name)
		}
	}
	if len(names) == 0 {
		return ""
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

// reset clears the discovered set and the announced-names bookkeeping.
// Used by tests and when a new run starts with a clean slate.
func (s *discoveredState) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = make(map[string]struct{})
	s.announcedSystemPromptNames = nil
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
