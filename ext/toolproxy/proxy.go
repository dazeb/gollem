package toolproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fugue-labs/gollem/core"
)

// Mode controls how aggressively the proxy defers tools.
type Mode int

const (
	// ModeAlways defers every tool marked ShouldDefer=true and is the
	// default (zero value). The rationale: if a caller stamps
	// ShouldDefer=true on a tool, they want it deferred — threshold logic
	// is a second-order convenience.
	ModeAlways Mode = iota
	// ModeAuto only defers when the combined size of deferred-eligible
	// tools exceeds AutoTokenRatio of the configured context window.
	// Below that threshold the proxy is a pass-through. Useful when a
	// caller stamps ShouldDefer optimistically and wants deferral to
	// kick in only once the catalog grows large enough to matter.
	ModeAuto
	// ModeOff disables deferral entirely. The proxy's PrepareFunc returns
	// the tool list unchanged and tool_search still works for the model
	// but has nothing to hide.
	ModeOff
)

// DefaultToolName is the tool name used when Config.ToolName is empty.
const DefaultToolName = "tool_search"

// DefaultMaxResults is the cap on matches returned per tool_search call.
const DefaultMaxResults = 5

// DefaultAutoTokenRatio is the fraction of the context window above which
// ModeAuto starts deferring. Matches Claude Code's 10% default.
const DefaultAutoTokenRatio = 0.10

// DefaultContextWindow is used when no Config.ContextWindow is provided.
// 200k mirrors Claude Sonnet/Opus 4+. The value is only used for ModeAuto
// threshold calculations; it does not affect runtime behavior otherwise.
const DefaultContextWindow = 200_000

// defaultCharsPerToken approximates tokens from characters when no token
// counter is supplied. Claude Code uses 2.5 for MCP tool definitions.
const defaultCharsPerToken = 2.5

// TokenCounter measures the effective token cost of a set of deferred
// tool definitions. Callers can plug in a provider-specific implementation
// (e.g. Anthropic's count_tokens endpoint). When nil the proxy falls back
// to a character-count heuristic.
type TokenCounter func(tools []core.Tool) int

// Config configures a Proxy instance.
type Config struct {
	// Mode selects the deferral strategy. Zero value is ModeAuto.
	Mode Mode
	// ToolName is the user-visible name of the tool_search tool. If empty,
	// DefaultToolName is used.
	ToolName string
	// MaxResults caps the number of matches returned per keyword search.
	// If zero, DefaultMaxResults is used.
	MaxResults int
	// AutoTokenRatio is the fraction of ContextWindow (0–1) above which
	// ModeAuto starts deferring. If zero, DefaultAutoTokenRatio is used.
	// Only consulted in ModeAuto.
	AutoTokenRatio float64
	// ContextWindow is the size of the target model's context window in
	// tokens. If zero, DefaultContextWindow is used. Only consulted in
	// ModeAuto to compute the deferral threshold.
	ContextWindow int
	// TokenCounter optionally provides an exact token count for deferred
	// tools. Falls back to a character heuristic if nil.
	TokenCounter TokenCounter
}

// Proxy is a stateful tool deferral helper. One Proxy instance owns a
// single tool_search tool and the set of discovered tool names for a run.
// Proxy is safe for concurrent use: all mutable state lives behind a mutex
// on the underlying discoveredState.
type Proxy struct {
	cfg   Config
	state *discoveredState
}

// New constructs a Proxy from the given Config.
func New(cfg Config) *Proxy {
	if cfg.ToolName == "" {
		cfg.ToolName = DefaultToolName
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = DefaultMaxResults
	}
	if cfg.AutoTokenRatio <= 0 {
		cfg.AutoTokenRatio = DefaultAutoTokenRatio
	}
	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = DefaultContextWindow
	}
	return &Proxy{
		cfg:   cfg,
		state: newDiscoveredState(),
	}
}

// ToolName returns the configured name for the tool_search tool.
func (p *Proxy) ToolName() string { return p.cfg.ToolName }

// Discovered returns a snapshot of tool names that have been loaded via
// tool_search in this run. Primarily used by tests and observability.
func (p *Proxy) Discovered() []string { return p.state.snapshot() }

// Reset clears the discovered set. Useful for tests and for callers that
// want to reuse a Proxy across multiple runs without carrying state over.
func (p *Proxy) Reset() { p.state.reset() }

// searchParams is the JSON schema argument for the tool_search tool.
type searchParams struct {
	Query      string `json:"query" jsonschema:"description=Query string. Use 'select:Name1,Name2' for direct lookup or keywords for scored search."`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=Maximum number of matches to return. Defaults to the proxy's configured value."`
}

// searchResultJSON is the structured payload returned to the model.
type searchResultJSON struct {
	Matches            []string `json:"matches"`
	Query              string   `json:"query"`
	TotalDeferredTools int      `json:"total_deferred_tools"`
	Text               string   `json:"text,omitempty"`
}

// Tool returns the tool_search Tool that the model calls to load deferred
// definitions. It is stateful — its Stateful interface persists the
// discovered set through checkpoints.
func (p *Proxy) Tool() core.Tool {
	schema := core.SchemaFor[searchParams]()
	return core.Tool{
		Definition: core.ToolDefinition{
			Name:             p.cfg.ToolName,
			Description:      toolSearchDescription,
			ParametersSchema: schema,
			Kind:             core.ToolKindFunction,
		},
		Handler:  p.handleSearch,
		Stateful: p.state,
	}
}

// handleSearch is the Tool handler. It does not have direct access to the
// full tool list — it relies on state set by PrepareFunc, which records
// the most recent snapshot of the complete tool pool on the Proxy just
// before each request.
//
// This is required because StatefulTool handlers don't receive the agent
// tool catalog, only the call args. We work around it by snapshotting the
// pool whenever PrepareFunc runs, just ahead of the model call that
// might invoke tool_search. The pool includes both deferred and
// non-deferred tools so exact-match / select: lookups can fall back to
// already-loaded tools (harmless no-op).
func (p *Proxy) handleSearch(_ context.Context, _ *core.RunContext, argsJSON string) (any, error) {
	var params searchParams
	if argsJSON != "" && argsJSON != "{}" {
		if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
			return nil, fmt.Errorf("tool_search: failed to parse args: %w", err)
		}
	}
	if params.Query == "" {
		return nil, errors.New("tool_search: 'query' is required")
	}

	maxResults := params.MaxResults
	if maxResults <= 0 {
		maxResults = p.cfg.MaxResults
	}

	pool := p.loadPool()
	matches := searchTools(params.Query, pool, maxResults)

	// Track newly-discovered tools so subsequent PrepareFunc calls will
	// include them. Adding a non-deferred tool name is a harmless no-op:
	// the filter check only drops undiscovered deferred tools.
	for _, name := range matches {
		p.state.add(name)
	}

	// Count deferred tools for the status field so the model sees the
	// size of the hidden catalog.
	deferredCount := 0
	for _, t := range pool {
		if t.ShouldDefer {
			deferredCount++
		}
	}

	return searchResultJSON{
		Matches:            matches,
		Query:              params.Query,
		TotalDeferredTools: deferredCount,
		Text:               buildToolResultText(matches, pool),
	}, nil
}

// SystemPromptFragment returns a markdown block listing the names of all
// deferred tools in the given slice. Append this to your agent's system
// prompt (directly or via WithDynamicSystemPrompt) so the model knows what
// it can ask tool_search to load.
//
// Returns the empty string when no tools are deferred — always safe to
// concatenate.
func (p *Proxy) SystemPromptFragment(tools []core.Tool) string {
	if p.cfg.Mode == ModeOff {
		return ""
	}
	return buildSystemPromptFragment(p.cfg.ToolName, tools)
}

// PrepareFuncFor binds a Proxy to a concrete tool list and returns the
// AgentToolsPrepareFunc to wire into the agent via core.WithToolsPrepare.
// Callers pass the *same* slice they wire into the agent via core.WithTools
// so the returned func can consult Tool.ShouldDefer / Tool.SearchHint
// when filtering on every request.
//
// The returned function, on each request:
//
//  1. Records the current pool of tools so the tool_search handler can
//     later search over it (including non-deferred tools for exact-match
//     / select: fallbacks).
//  2. In ModeAuto, returns the full tool list unchanged when combined
//     deferred-tool size is below the threshold.
//  3. Otherwise, returns non-deferred tools + the tool_search tool itself
//     + any deferred tool already discovered via tool_search.
//
// Why there is no argumentless PrepareFunc(): gollem's
// AgentToolsPrepareFunc signature only receives []ToolDefinition, which
// does not carry ShouldDefer / SearchHint. Binding the full Tool slice
// at wiring time is therefore mandatory for correctness; a no-arg shim
// would silently disable deferral, which is a footgun.
func (p *Proxy) PrepareFuncFor(tools []core.Tool) core.AgentToolsPrepareFunc {
	// Index by name for O(1) lookup during filter.
	index := make(map[string]core.Tool, len(tools))
	for _, t := range tools {
		index[t.Definition.Name] = t
	}

	return func(_ context.Context, _ *core.RunContext, defs []core.ToolDefinition) []core.ToolDefinition {
		if p.cfg.Mode == ModeOff {
			p.recordPool(tools)
			return defs
		}

		// Update the "last seen deferred pool" so handleSearch can search it.
		p.recordPool(tools)

		// ModeAuto: short-circuit if below the deferral threshold.
		if p.cfg.Mode == ModeAuto && !p.shouldDefer(tools) {
			return defs
		}

		// ModeAlways (or ModeAuto past threshold): filter deferred tools
		// whose names are not yet in the discovered set.
		discovered := p.state.snapshot()
		discoveredSet := make(map[string]struct{}, len(discovered))
		for _, n := range discovered {
			discoveredSet[n] = struct{}{}
		}

		out := defs[:0:0] // new slice, do not alias defs
		for _, d := range defs {
			t, ok := index[d.Name]
			if !ok || !t.ShouldDefer {
				// Non-deferred: always include.
				out = append(out, d)
				continue
			}
			if _, loaded := discoveredSet[d.Name]; loaded {
				// Deferred but discovered: include.
				out = append(out, d)
				continue
			}
			// Deferred + not yet discovered: drop.
		}
		return out
	}
}

// recordPool updates the "last observed tool pool" used by the
// tool_search handler. The full tool list is recorded (deferred +
// non-deferred) so that exact-match / select: lookups can fall back to
// tools that are already loaded. We take a shallow copy so later
// mutations to the caller's slice don't bleed into search results.
func (p *Proxy) recordPool(tools []core.Tool) {
	pool := make([]core.Tool, len(tools))
	copy(pool, tools)
	p.state.setPool(pool)
}

// loadPool returns the pool recorded by the most recent recordPool call,
// or nil if recordPool has not been called yet (e.g. the caller forgot
// to wire PrepareFuncFor).
func (p *Proxy) loadPool() []core.Tool {
	return p.state.getPool()
}

// shouldDefer reports whether the combined size of deferred tools exceeds
// the auto-mode threshold.
func (p *Proxy) shouldDefer(tools []core.Tool) bool {
	var deferred []core.Tool
	for _, t := range tools {
		if t.ShouldDefer {
			deferred = append(deferred, t)
		}
	}
	if len(deferred) == 0 {
		return false
	}

	threshold := int(float64(p.cfg.ContextWindow) * p.cfg.AutoTokenRatio)
	if threshold <= 0 {
		return true
	}

	var cost int
	if p.cfg.TokenCounter != nil {
		cost = p.cfg.TokenCounter(deferred)
	} else {
		cost = estimateTokens(deferred)
	}
	return cost >= threshold
}

// estimateTokens approximates the token cost of a slice of tool
// definitions using a character-count heuristic. Only used when the
// caller has not supplied a TokenCounter.
func estimateTokens(tools []core.Tool) int {
	var chars int
	for _, t := range tools {
		chars += len(t.Definition.Name)
		chars += len(t.Definition.Description)
		if t.Definition.ParametersSchema != nil {
			if b, err := json.Marshal(t.Definition.ParametersSchema); err == nil {
				chars += len(b)
			}
		}
		chars += len(t.SearchHint)
	}
	return int(float64(chars) / defaultCharsPerToken)
}
