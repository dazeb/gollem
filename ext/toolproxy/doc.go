// Package toolproxy provides a portable tool-deferral / auto-discovery
// capability for gollem agents.
//
// The problem: an agent with a large tool catalog (especially projects
// with many MCP servers) pays token cost on every request to declare
// tools that most turns never use. Gollem supports 100+ tools across
// providers, and sending 50k+ tokens of tool definitions per turn is
// wasteful when the typical turn only exercises 1–2 tools.
//
// The solution:
//
//  1. Mark eligible tools with Tool.ShouldDefer = true.
//  2. Construct a Proxy via toolproxy.New(...).
//  3. Build the agent's tool list including Proxy.Tool() (the
//     tool_search tool) and wire Proxy.PrepareFuncFor(tools) into the
//     agent with core.WithToolsPrepare.
//  4. Append Proxy.SystemPromptFragment(tools) to the agent's system
//     prompt so the model knows which deferred tools exist.
//
// Minimal wiring:
//
//	proxy := toolproxy.New(toolproxy.Config{}) // zero value is ModeAuto
//	tools := append(baseTools, proxy.Tool())
//	agent := core.NewAgent[string](model,
//	    core.WithTools[string](tools...),
//	    core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
//	    core.WithSystemPrompt[string]("You are..." + proxy.SystemPromptFragment(tools)),
//	)
//
// On every model request, PrepareFuncFor returns: all non-deferred
// tools + tool_search + any deferred tool the model has already
// discovered via tool_search in this run. Discovery state lives on
// the tool_search tool itself as a StatefulTool, so it
// checkpoint-round-trips cleanly.
//
// # Cache Safety — please read
//
// Tool deferral is a net token win in most real-world conditions but
// it is NOT a free lunch under prompt caching (Anthropic and similar).
// The cache hierarchy is tools → system → messages, and any change to
// a higher level invalidates everything below. A cache write costs
// 1.25× normal input tokens; a cache read costs 0.10×. Three hazards:
//
// Hazard 1 — dynamic tool admission busts the tool-definition cache.
// Every time the model discovers a new tool via tool_search, the next
// request's tools[] array differs from the previous request's. That
// is a tool-section cache miss.
//
// Worked example with 10 deferred tools × 500 tokens each, 10-turn run:
//
//   - Baseline (no deferral):  ~10,750 tokens (1 write, 9 reads).
//   - ModeAlways, adversarial (1 new tool discovered every turn):
//     ~34,375 tokens — a 3.2× net LOSS.
//   - ModeAlways, realistic (3 discoveries over 10 turns):
//     ~7,150 tokens — a 7% net WIN.
//
// For pools of ~40+ deferred tools, even the adversarial case still
// wins because the unused tools in baseline dominate the cache-miss
// cost of progressive admission. The break-even threshold lives
// around 40 tools / 20k tokens at a 200k-token context window.
//
// Hazard 2 — SystemPromptFuncFor busts the system-prompt cache.
// The delta form emits the full fragment on turn 1 and empty on
// turn 2+. That means the system-prompt CONTENT differs between
// turns and the system-prompt cache misses on turn 2. Under prompt
// caching, this is worse than SystemPromptFragment (the static full
// listing) for any conversation under ~11 turns. See
// SystemPromptFuncFor's doc comment for the detailed math.
//
// Hazard 3 — MarkDeferred on small pools. MarkDeferred stamps every
// tool in the slice with ShouldDefer=true. If the caller then runs
// under ModeAlways with a small pool, they hit Hazard 1. Mitigated
// by ModeAuto's pass-through threshold.
//
// # Recommended defaults
//
//   - Mode: ModeAuto (the zero value). Pass-through on small pools,
//     unconditional deferral on pools large enough to guarantee net
//     savings. This is the cache-safe default.
//   - Fragment delivery: SystemPromptFragment or WrapPrompt. Both
//     ship the fragment once and then cache it on subsequent turns.
//   - Opt into ModeAlways only when you've measured the pool and
//     know deferral dominates cache cost (~40+ deferred tools at
//     typical description sizes).
//   - Use SystemPromptFuncFor only with providers that don't cache
//     system prompts, or for very long conversations where the
//     per-turn savings of empty deltas add up.
//
// # Difference from Claude Code
//
// Claude Code relies on provider-specific Anthropic beta API features
// (defer_loading on tool definitions + tool_reference content blocks)
// that expand tools server-side, keeping the client-sent tool array
// stable. That sidesteps Hazard 1 entirely. This package is portable
// across every provider gollem targets, so it does NOT have that
// escape hatch — the safe default is ModeAuto rather than Claude
// Code's tst, which is their equivalent of ModeAlways.
package toolproxy
