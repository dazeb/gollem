# Context Trace Plan

This plan splits the feature into two layers:

- `gollem` owns the authoritative request/context trace.
- `gollem.nvim` visualizes that trace and diffs it between turns.

## Phase 1: Core Trace Capture In `gollem`

Status: in progress

Goal:

- capture the actual outbound model request after all request shaping and middleware
- attach it to `RunTrace`
- include enough structure for later Neovim inspection
- rely on provider-reported usage totals, not heuristic token estimates

Files:

- `core/trace.go`
- `core/request_trace.go`
- `core/run_state.go`
- `core/run_engine.go`
- `core/stream.go`
- `core/agent.go`
- `core/trace_test.go`

Checklist:

- add `RequestTrace` to `RunTrace`
- add a JSON-safe request payload snapshot
- capture final `messages`, `settings`, and `params`
- capture response metadata and usage
- capture compaction stats that affected the request
- support both `Run` and `RunStream`
- verify middleware-mutated requests are what gets traced

## Phase 2: Sidecar Persistence And RPC In `gollem.nvim`

Status: pending

Goal:

- persist per-session context traces
- expose trace history over JSON-RPC
- keep raw trace data separate from transcript rendering

Files:

- `internal/sidecar/protocol.go`
- `internal/sidecar/server.go`
- `internal/sidecar/store.go`
- `internal/sidecar/server_test.go`
- `PROTOCOL.md`

Checklist:

- add `list_context_traces` RPC
- add `get_context_trace` RPC
- persist trace history per session on disk
- include Neovim-originated context blocks alongside the request trace
- keep resumed sessions able to read older traces after restart
- add tests for trace restore and RPC reads

## Phase 3: Neovim Context Inspector UI In `gollem.nvim`

Status: pending

Goal:

- inspect the current request context
- browse prior requests and turns
- diff two requests to see context growth or compaction impact

Files:

- `lua/gollem/client.lua`
- `lua/gollem/init.lua`
- `lua/gollem/ui.lua`
- `lua/gollem/config.lua`
- `plugin/gollem.lua`
- `README.md`

Checklist:

- add `:GollemContext`
- add a picker for request history in the current session
- open a summary buffer for one request trace
- open a raw JSON buffer for exact payload inspection
- add a side-by-side diff view using Neovim diff mode
- show request summaries: message count, provider input/output/cache-read/cache-write usage, tool counts, compactions
- add keymaps for context inspector and previous/next request navigation

## Phase 4: Higher-Signal Token Debugging

Status: pending

Goal:

- make the inspector useful for token-efficiency debugging rather than just payload dumping

Possible follow-ups:

- surface request-to-request usage deltas from provider-reported input/output/cache-read/cache-write totals
- add duplicate-context heuristics in `gollem.nvim`
- surface “what changed since the previous request” in a compact summary panel
- highlight compaction wins and regressions turn-over-turn
- optionally integrate with `telescope.nvim`, `fzf-lua`, or `diffview.nvim` when present
