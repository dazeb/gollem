// Package toolproxy provides a portable tool-deferral / auto-discovery
// capability for gollem agents.
//
// The problem: an agent with a large tool catalog (especially projects with
// many MCP servers) pays token cost on every request to declare tools that
// most turns never use. Gollem supports 100+ tools across providers, and
// sending 50k+ tokens of tool definitions per turn is wasteful when the
// typical turn only exercises 1–2 tools.
//
// The solution in this package:
//
//  1. Mark eligible tools with Tool.ShouldDefer = true.
//  2. Construct a Proxy via toolproxy.New(...).
//  3. Build the agent's tool list, including Proxy.Tool() (the tool_search
//     tool), and wire Proxy.PrepareFuncFor(tools) into the agent with
//     core.WithToolsPrepare.
//  4. Append Proxy.SystemPromptFragment(tools) to the agent's system prompt
//     so the model knows which deferred tools exist.
//
// Minimal wiring:
//
//	proxy := toolproxy.New(toolproxy.Config{}) // zero value is ModeAlways
//	tools := append(baseTools, proxy.Tool())
//	agent := core.NewAgent[string](model,
//	    core.WithTools[string](tools...),
//	    core.WithToolsPrepare[string](proxy.PrepareFuncFor(tools)),
//	    core.WithSystemPrompt[string]("You are..." + proxy.SystemPromptFragment(tools)),
//	)
//
// On every model request PrepareFuncFor returns: all non-deferred tools +
// tool_search + any deferred tool the model has already discovered via
// tool_search in this run. Discovery state lives on the tool_search tool
// itself as a StatefulTool, so it checkpoint-round-trips cleanly.
//
// # Difference from Claude Code
//
// Claude Code / Codex rely on provider-specific API features (defer_loading on tool
// definitions + tool_reference content blocks) for server-side expansion.
// That feature isn't portable across the providers gollem targets, so this
// package implements the same user-visible behavior entirely client-side by
// filtering the tool list on each request.
package toolproxy
