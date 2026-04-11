package toolproxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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
func (p *Proxy) handleSearch(_ context.Context, rc *core.RunContext, argsJSON string) (any, error) {
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
	matches := searchTools(params.Query, pool, maxResults, p.state.lookupScoring)

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

	// Telemetry: report what just happened. Nil-safe — no-op if the
	// agent has no EventBus configured.
	publishSearchOutcome(rc, SearchOutcomeEvent{
		ToolName:           p.cfg.ToolName,
		Query:              params.Query,
		QueryKind:          classifyQueryKind(params.Query),
		MatchCount:         len(matches),
		HasMatches:         len(matches) > 0,
		TotalDeferredTools: deferredCount,
		MaxResults:         maxResults,
	})

	return searchResultJSON{
		Matches:            matches,
		Query:              params.Query,
		TotalDeferredTools: deferredCount,
		Text:               buildToolResultText(matches, pool),
	}, nil
}

// classifyQueryKind returns SearchQueryKindSelect for `select:...`
// queries and SearchQueryKindKeyword for everything else. Used only
// by the telemetry path.
func classifyQueryKind(query string) SearchQueryKind {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(query)), "select:") {
		return SearchQueryKindSelect
	}
	return SearchQueryKindKeyword
}

// MarkDeferred returns a new slice in which every tool has
// ShouldDefer=true set. The input slice is not mutated. Existing
// ShouldDefer / SearchHint fields on each tool are preserved; only
// the deferred flag is forced on.
//
// This is the idiomatic way to stamp all tools from a source that
// should be hidden until the model explicitly loads them — most
// commonly MCP servers. Matches Claude Code's auto-defer behaviour
// for MCP tools (isMcp=true), except the gollem port is explicit so
// callers can opt in per-source rather than relying on magic.
//
// Example with an MCP Manager:
//
//	mcpTools, _ := manager.Tools(ctx)
//	all := append(baseTools, toolproxy.MarkDeferred(mcpTools)...)
//	all = append(all, proxy.Tool())
//	agent := core.NewAgent[string](model,
//	    core.WithTools[string](all...),
//	    core.WithToolsPrepare[string](proxy.PrepareFuncFor(all)),
//	    core.WithSystemPrompt[string]("..."+proxy.SystemPromptFragment(all)),
//	)
func MarkDeferred(tools []core.Tool) []core.Tool {
	return MarkDeferredIf(tools, func(core.Tool) bool { return true })
}

// MarkDeferredIf is like MarkDeferred but only stamps ShouldDefer on
// tools for which predicate returns true. Useful when importing a
// mixed set and wanting to defer only some subset — for example,
// deferring MCP tools whose names match a prefix while leaving the
// rest inline.
//
// Tools with AlwaysLoad=true are skipped unconditionally: AlwaysLoad
// is the canonical opt-out and winning it automatically means the
// stamper can never accidentally override a caller's intent to keep
// a specific tool in the base toolset.
//
// Returns a new slice; the input is not mutated.
func MarkDeferredIf(tools []core.Tool, predicate func(core.Tool) bool) []core.Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]core.Tool, len(tools))
	for i, t := range tools {
		out[i] = t
		if t.AlwaysLoad {
			// Never stamp AlwaysLoad tools.
			continue
		}
		if predicate(t) {
			out[i].ShouldDefer = true
		}
	}
	return out
}

// SystemPromptFragment returns a markdown block listing the names of all
// deferred tools in the given slice. Append this to your agent's system
// prompt (directly, via WithSystemPrompt) so the model knows what it
// can ask tool_search to load.
//
// This is the static full-list form: every request's system prompt
// contains the complete list of deferred tool names. It is
// cache-friendly — provider prompt caching (Anthropic, OpenAI, etc.)
// amortises the transmission cost to once per conversation.
//
// For callers who want first-turn-only delivery via the user prompt
// (e.g. to work well with providers that don't cache system prompts),
// use WrapPrompt. For stateful delta via a dynamic system prompt, use
// SystemPromptFuncFor.
//
// Returns the empty string when no tools are deferred — always safe to
// concatenate.
func (p *Proxy) SystemPromptFragment(tools []core.Tool) string {
	if p.cfg.Mode == ModeOff {
		return ""
	}
	return buildSystemPromptFragment(p.cfg.ToolName, tools)
}

// WrapPrompt prepends the deferred-tools fragment to the given user
// prompt, returning a combined string suitable for passing directly
// to agent.Run. The fragment is delivered exactly once — the first
// turn's user message — and persists through the rest of the run via
// message history, so no subsequent turn re-sends it.
//
// This is the delta-efficient delivery mechanism: it works on every
// provider (no cache dependency) and keeps the per-turn token cost
// flat across the run, equivalent to Claude Code's one-shot
// <available-deferred-tools> attachment on the first user message.
//
// If the Proxy is in ModeOff, or there are no deferred tools in the
// list, WrapPrompt returns the prompt unchanged.
func (p *Proxy) WrapPrompt(prompt string, tools []core.Tool) string {
	if p.cfg.Mode == ModeOff {
		return prompt
	}
	fragment := buildSystemPromptFragment(p.cfg.ToolName, tools)
	if fragment == "" {
		return prompt
	}
	// Trim leading newlines on the fragment — SystemPromptFragment
	// emits them as a separator from prior system prompt content, but
	// here the fragment is the start of a user message.
	fragment = strings.TrimLeft(fragment, "\n")
	if prompt == "" {
		return fragment
	}
	return fragment + "\n" + prompt
}

// SystemPromptFuncFor returns a dynamic system-prompt function that
// emits a proper delta announcement: on the first invocation it lists
// all deferred tools; on subsequent calls it lists only tools added
// since the previous call (or empty when nothing is new). Wire the
// result into the agent with core.WithDynamicSystemPrompt.
//
// This mirrors Claude Code's deferred_tools_delta semantics:
//
//   - Turn 1 — fresh Proxy, deferred pool is {A, B, C}. The returned
//     prompt lists A, B, C.
//   - Turn 2 — pool unchanged. The returned prompt is empty.
//   - Turn 3 — a new deferred tool D appeared (e.g. an MCP server
//     just reconnected and PrepareFuncFor saw the change). The
//     returned prompt lists only D.
//
// When tools are REMOVED from the pool, the delta is still emitted as
// an empty-added, non-empty-removed PoolChangeEvent so subscribers can
// log the churn. The system prompt itself reports only additions —
// dropping a tool doesn't need to be explained to the model, it just
// stops existing in the next tools array.
//
// This is a delta-efficient form for providers that support prompt
// caching (Anthropic, OpenAI): the first-turn full list is cached,
// then empty subsequent turns cost ~0 tokens.
//
// For provider-independent one-shot delivery via the user prompt,
// prefer WrapPrompt. For a static full listing in every turn's
// system prompt, use SystemPromptFragment directly.
//
// Announcement state lives on the Proxy instance and is mutex-guarded.
// Calling Proxy.Reset clears the announced set, so the next call
// re-announces everything.
func (p *Proxy) SystemPromptFuncFor(tools []core.Tool) core.SystemPromptFunc {
	return func(_ context.Context, rc *core.RunContext) (string, error) {
		if p.cfg.Mode == ModeOff {
			return "", nil
		}

		// Compute current deferred set for the pool closed over by this
		// function. A tool with AlwaysLoad=true overrides ShouldDefer.
		var currentDeferred []string
		for _, t := range tools {
			if t.ShouldDefer && !t.AlwaysLoad {
				currentDeferred = append(currentDeferred, t.Definition.Name)
			}
		}

		added, removed, initial := p.state.diffSystemPromptAnnouncement(currentDeferred)
		if len(added) == 0 && len(removed) == 0 {
			return "", nil
		}

		// Emit a pool-change event for observability even when the
		// fragment itself won't include anything (removals only). The
		// Initial flag lets subscribers distinguish the first full
		// announcement from an incremental delta.
		publishPoolChange(rc, PoolChangeEvent{
			ToolName: p.cfg.ToolName,
			Added:    added,
			Removed:  removed,
			Initial:  initial,
		})

		if len(added) == 0 {
			return "", nil
		}
		return buildDeferredListFragment(p.cfg.ToolName, added, initial), nil
	}
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

	return func(_ context.Context, rc *core.RunContext, defs []core.ToolDefinition) []core.ToolDefinition {
		// Pre-count deferred/inline in the incoming pool so the decision
		// event carries a consistent snapshot regardless of branch.
		// AlwaysLoad wins over ShouldDefer, so tools with both flags
		// count as inline.
		totalDeferred := 0
		for _, t := range tools {
			if t.ShouldDefer && !t.AlwaysLoad {
				totalDeferred++
			}
		}
		inline := len(tools) - totalDeferred

		if p.cfg.Mode == ModeOff {
			p.recordPool(tools)
			publishModeDecision(rc, ModeDecisionEvent{
				Mode:              ModeOff,
				Reason:            ReasonModeOff,
				Deferred:          false,
				DeferredToolCount: totalDeferred,
				InlineToolCount:   inline,
			})
			return defs
		}

		// Update the "last seen deferred pool" so handleSearch can search it.
		p.recordPool(tools)

		// ModeAuto: short-circuit if below the deferral threshold.
		if p.cfg.Mode == ModeAuto {
			cost := p.estimateDeferredCost(tools)
			threshold := int(float64(p.cfg.ContextWindow) * p.cfg.AutoTokenRatio)
			if cost < threshold {
				publishModeDecision(rc, ModeDecisionEvent{
					Mode:              ModeAuto,
					Reason:            ReasonAutoBelowThreshold,
					Deferred:          false,
					DeferredToolCount: totalDeferred,
					InlineToolCount:   inline,
					EstimatedTokens:   cost,
					Threshold:         threshold,
				})
				return defs
			}
			// Fall through to filtering, but emit the decision event first
			// with Deferred=true.
			publishModeDecision(rc, ModeDecisionEvent{
				Mode:              ModeAuto,
				Reason:            ReasonAutoAboveThreshold,
				Deferred:          true,
				DeferredToolCount: totalDeferred,
				InlineToolCount:   inline,
				EstimatedTokens:   cost,
				Threshold:         threshold,
			})
		} else {
			// ModeAlways: always defer.
			publishModeDecision(rc, ModeDecisionEvent{
				Mode:              ModeAlways,
				Reason:            ReasonAlwaysDefer,
				Deferred:          true,
				DeferredToolCount: totalDeferred,
				InlineToolCount:   inline,
			})
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
			if !ok || !t.ShouldDefer || t.AlwaysLoad {
				// Non-deferred, or AlwaysLoad opt-out: always include.
				// AlwaysLoad wins over ShouldDefer, matching Claude
				// Code's tool.alwaysLoad priority.
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

// estimateDeferredCost returns the token-cost estimate for the tools
// in `tools` that have ShouldDefer=true and AlwaysLoad=false. Uses the
// caller-supplied TokenCounter if present, else falls back to a
// character heuristic. Returns 0 when no tools are eligible for
// deferral.
func (p *Proxy) estimateDeferredCost(tools []core.Tool) int {
	var deferred []core.Tool
	for _, t := range tools {
		if t.ShouldDefer && !t.AlwaysLoad {
			deferred = append(deferred, t)
		}
	}
	if len(deferred) == 0 {
		return 0
	}
	if p.cfg.TokenCounter != nil {
		return p.cfg.TokenCounter(deferred)
	}
	return estimateTokens(deferred)
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
