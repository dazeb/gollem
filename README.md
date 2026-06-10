<p align="center">
  <h1 align="center">gollem</h1>
  <p align="center"><strong>Typed LLM agents for Go</strong></p>
  <p align="center">
    Define your agent's output as a Go struct. gollem generates the JSON Schema, runs the agent loop,
    validates every response, repairs malformed output, and hands you back a typed value — not a <code>map[string]any</code>.
  </p>
</p>

<p align="center">
  <a href="https://github.com/fugue-labs/gollem/actions/workflows/ci.yml"><img src="https://github.com/fugue-labs/gollem/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/fugue-labs/gollem"><img src="https://pkg.go.dev/badge/github.com/fugue-labs/gollem.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/fugue-labs/gollem"><img src="https://goreportcard.com/badge/github.com/fugue-labs/gollem" alt="Go Report Card"></a>
  <a href="https://codecov.io/gh/fugue-labs/gollem"><img src="https://codecov.io/gh/fugue-labs/gollem/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
</p>

---

```bash
go get github.com/fugue-labs/gollem
```

## Why gollem

Most agent frameworks validate at runtime and return loosely typed data. gollem leans on Go's
type system instead: output schemas, tool parameters, guardrails, and middleware are all
expressed as ordinary Go types, so the compiler catches a whole class of wiring mistakes
before anything runs — and your editor can autocomplete the rest.

The core ideas:

- **`Agent[T]`** — declare the output type once. Schema generation (via reflection over
  struct tags), validation, deserialization, and retry-on-parse-failure are automatic.
- **`FuncTool[P]`** — turn any typed Go function into a tool. Parameter schemas come from
  struct tags; no hand-written JSON Schema.
- **Structured output via a `final_result` tool** — the model returns its answer through a
  forced tool call, which is the most reliable extraction pattern across providers.
- **Output repair** — if the model returns malformed JSON, an optional repair model gets one
  shot at fixing it before the normal retry path kicks in.
- **Single binary** — your agent compiles into one static binary. No interpreter, no
  virtualenv, no "works on my machine."

gollem is a young project (v0.5.x, pre-1.0). The core agent loop, structured output,
guardrails, and providers are stable and heavily tested; the wider extension surface is
usable but still evolving. See [Stability](#stability) below for what that means in practice.

## Quick start

This example runs entirely offline using the built-in `TestModel` mock — no API key required.

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/fugue-labs/gollem"
)

type CityInfo struct {
    Name       string `json:"name" jsonschema:"description=City name"`
    Country    string `json:"country" jsonschema:"description=Country"`
    Population int    `json:"population" jsonschema:"description=Approximate population"`
}

func main() {
    model := gollem.NewTestModel(
        gollem.ToolCallResponse("final_result", `{"name":"Tokyo","country":"Japan","population":14000000}`),
    )

    agent := gollem.NewAgent[CityInfo](model,
        gollem.WithSystemPrompt[CityInfo]("You are a geography expert."),
    )

    result, err := agent.Run(context.Background(), "Tell me about Tokyo")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Output.Name)       // Tokyo — a string, not an any
    fmt.Println(result.Output.Population) // 14000000 — an int, not a float64 in disguise
}
```

Swap in a real provider with one line:

```go
import "github.com/fugue-labs/gollem/provider/anthropic"

model := anthropic.New() // reads ANTHROPIC_API_KEY
```

## Providers

| Provider | Package |
|---|---|
| Anthropic (Claude) | `provider/anthropic` |
| OpenAI (GPT, o-series) | `provider/openai` |
| Google Gemini (Vertex AI) | `provider/vertexai` |
| Claude via Vertex AI | `provider/vertexai_anthropic` |

All providers support streaming, tool calls, and structured output. Providers implement a
small `Model` interface, so wrapping or adding one is straightforward.

## A production-shaped example

Everything below is composed from independent options — use what you need, ignore the rest.

```go
model := gollem.NewRetryModel(anthropic.New(), gollem.DefaultRetryConfig())

tracker := gollem.NewCostTracker(map[string]gollem.ModelPricing{
    "claude-sonnet-4-6": {InputTokenCost: 0.003, OutputTokenCost: 0.015},
})

agent := gollem.NewAgent[Analysis](model,
    // Guardrails: validate input before the loop, and state before each turn.
    gollem.WithInputGuardrail[Analysis]("length", gollem.MaxPromptLength(10000)),
    gollem.WithTurnGuardrail[Analysis]("turns", gollem.MaxTurns(20)),

    // Hard limits: stop the run when budgets are exhausted.
    gollem.WithCostTracker[Analysis](tracker),
    gollem.WithUsageQuota[Analysis](gollem.UsageQuota{MaxRequests: 50, MaxTotalTokens: 100000}),
    gollem.WithRunCondition[Analysis](gollem.Or(
        gollem.MaxRunDuration(2*time.Minute),
        gollem.ToolCallCount(50),
    )),

    // Cross-cutting concerns as middleware and interceptors.
    gollem.WithAgentMiddleware[Analysis](gollem.LoggingMiddleware(log.Printf)),
    gollem.WithMessageInterceptor[Analysis](gollem.RedactPII(
        `\b\d{3}-\d{2}-\d{4}\b`, "[SSN REDACTED]",
    )),

    // Observability: structured traces, exportable as JSON or via OpenTelemetry.
    gollem.WithTracing[Analysis](),
    gollem.WithTraceExporter[Analysis](gollem.NewTraceFileExporter("./traces")),

    // Repair malformed output with a model before failing the run.
    gollem.WithOutputRepair[Analysis](gollem.ModelRepair[Analysis](repairModel)),
)

result, err := agent.Run(ctx, "Analyze the Q4 earnings report")
// result.Output     — typed Analysis struct
// result.Cost       — cost of this run
// result.Trace      — full execution trace
```

## What's in the box

**Core** (`github.com/fugue-labs/gollem`) — the part you should bet on:

- Typed agents, tools, and toolsets with reflection-based schema generation
- Streaming via Go 1.23+ `iter.Seq2` iterators (delta, accumulated, or debounced)
- Guardrails (input, per-turn, tool-result), output validators, output repair
- Retry, rate limiting, and response caching as composable `Model` wrappers
- Cost tracking, usage quotas, tool timeouts, run conditions
- Middleware, message/response interceptors, lifecycle hooks
- Structured run traces with pluggable exporters; canonical `gollem.trace.v1`
  artifacts with CLI inspect, replay, fork, and diff workflows
- Conversation snapshots for resume and time-travel debugging
- Memory strategies (sliding window, token budget, summarization)
- Orchestration primitives: chaining, pipelines, agent-as-tool delegation, handoffs
- `TestModel` for testing agents with zero network calls

**Extensions** (`ext/`, `contrib/`) — useful, less settled:

- `ext/mcp` — Model Context Protocol client (stdio and SSE, multi-server, namespaced tools)
- `ext/monty` — *code mode*: the model writes one Python script that calls your tools as
  functions, executed in a WASM sandbox ([monty-go](https://github.com/fugue-labs/monty-go)).
  N tool calls, one model round-trip.
- `ext/graph` — typed graph workflows with branching, fan-out, cycle detection, Mermaid export
- `ext/orchestrator`, `ext/team` — durable task stores and concurrent multi-agent teams
- `ext/eval` — datasets, built-in evaluators, LLM-as-judge scoring
- `ext/otel` — OpenTelemetry tracing and metrics
- `ext/tui` — terminal debugger with step-mode execution
- `contrib/` — HTTP handler adapters for Gin, Fiber, Echo, and Chi

Run `go doc` on any package, or browse the [examples/](examples/) directory — every major
feature has a runnable example, most of which work without API keys.

## A note on dependencies

The **core package** imports only the standard library. The **module** currently includes
the extensions, so your `go.sum` will list their dependencies (Temporal, OTel, web
frameworks, SQLite) even if your binary doesn't link them — Go only compiles what you
import, so binary size and attack surface are unaffected, but lockfile noise is real.
Splitting `ext/` and `contrib/` into separately versioned modules is planned before 1.0.

## Stability

- **Versioning:** pre-1.0. Core APIs (`Agent`, `Tool`, `Model`, guardrails, streaming) are
  treated as stable and changes are documented in [CHANGELOG.md](CHANGELOG.md). Extension
  APIs may still change between minor versions.
- **Testing:** 560+ tests across the repository; the core package has no external test
  dependencies and runs in under a second. CI runs on every commit.
- **Maintenance:** gollem is currently maintained by one person. If that gives you pause
  for a production dependency — fair. The codebase is small enough to read in an afternoon,
  vendoring is painless, and contributions are actively wanted (see
  [CONTRIBUTING.md](CONTRIBUTING.md)).

## When not to use gollem

- You want a batteries-included Python ecosystem (notebooks, LangSmith-style tooling,
  community integrations). Use LangChain, PydanticAI, or the OpenAI Agents SDK.
- You need providers gollem doesn't ship (Bedrock, Mistral, local llama.cpp). Implementing
  the `Model` interface is easy, but it's work you'd be signing up for.
- You need a 1.0 stability guarantee today.

## License

MIT — see [LICENSE](LICENSE).- **Built-in middleware** — `LoggingMiddleware`, `TimingMiddleware`, `MaxTokensMiddleware`
- **Message interceptors** — Intercept, modify, or drop outgoing model requests before they leave your system
- **Response interceptors** — Intercept incoming model responses for filtering or transformation
- **PII redaction** — Built-in `RedactPII` interceptor with regex-based pattern matching
- **Audit logging** — Built-in `AuditLog` interceptor for compliance and debugging

### Cost & Usage Control
- **Cost tracking** — `CostTracker` with per-model pricing, per-run cost breakdowns, and cumulative totals
- **Usage quotas** — Hard limits on requests, input tokens, output tokens, and total tokens with auto-termination
- **Tool choice control** — `Auto`, `Required`, `None`, `Force(toolName)` with optional auto-reset to prevent infinite loops
- **Auto context window management** — Transparent token overflow handling with configurable threshold and model-based summarization

### Resilience & Performance
- **Retry with exponential backoff** — `RetryModel` wrapper with jitter, configurable retries, and custom retryable predicates
- **Rate limiting** — Token-bucket `RateLimitedModel` for API throttling with burst capacity
- **Response caching** — `CachedModel` with SHA-256 key derivation and optional TTL
- **Tool execution timeouts** — Per-tool and agent-level deadlines via `context.WithTimeout`
- **Composable run conditions** — `MaxRunDuration`, `ToolCallCount`, `TextContains` with `And`/`Or` combinators
- **Batch execution** — `RunBatch` for concurrent multi-prompt runs with ordered results

### Multi-Agent Team Swarms
- **Durable task orchestration** — Task stores, lease-based claiming, schedulers, runner adapters, task-scoped artifacts, and durable event history for work coordination (`ext/orchestrator`)
- **Team orchestration** — Spawn concurrent teammate agents as goroutines with orchestrator-backed task claiming, teammate lifecycle control, and automatic task execution (`ext/team`)
- **Dynamic personality generation** — LLM generates task-specific system prompts for each subagent and teammate before they start, dramatically improving agent effectiveness (`modelutil`)
- **Cached personality generation** — SHA256-keyed cache prevents redundant LLM calls when identical tasks are delegated multiple times
- **Shared team tasks** — Team tasks live in the orchestrator store with assignees, lease-backed claiming, results, and artifacts
- **Teammate lifecycle** — Starting, running, idle, shutting down, and stopped states over orchestrator-backed task execution
- **Thin team sugar** — `ext/team` is a convenience layer over orchestrator primitives, not a second coordination model

### Composition & Multi-Agent
- **Agent cloning** — `Clone()` creates independent copies with additional options
- **Agent chaining** — `orchestration.ChainRun` pipes one agent's output as the next agent's input with usage aggregation
- **Composable pipelines** — `Pipeline` chains `PipelineStep` functions sequentially with `Then`, `ParallelSteps`, and `ConditionalStep` (`core/orchestration`)
- **`AgentTool` delegation** — One agent calls another as a tool (`core/orchestration`)
- **`Handoff` pipelines** — Sequential agent chains with context filters at boundaries (`core/orchestration`)
- **Handoff context filters** — `StripSystemPrompts`, `KeepLastN`, `SummarizeHistory`, composable with `ChainFilters` (`core/orchestration`)
- **Typed event bus** — Publish-subscribe coordination with `Subscribe[E]`, `Publish[E]`, and async variants; built-in runtime events carry run IDs, parent run IDs, and timestamps

### Intelligence & Routing
- **Model router** — Route prompts to different models based on content, length, or custom logic
- **Capability-based routing** — `NewCapabilityRouter` selects models matching required capabilities (vision, tool calls, context window)
- **Model capability profiles** — `ModelProfile` describes what a model supports; `Profiled` interface for self-declaration
- **Typed dependency injection** — `GetDeps[D]` and `TryGetDeps[D]` for compile-time safe dependency access from tools
- **Prompt templates** — Go `text/template` syntax with `Partial()` pre-filling and `TemplateVars` interface
- **Conversation memory strategies** — `SlidingWindowMemory`, `TokenBudgetMemory`, `SummaryMemory` (`core/memory`)
- **Dynamic system prompts** — Generate system prompts at runtime using `RunContext`

### Streaming Options
- **Delta streaming** — `StreamTextDelta` for raw incremental text chunks as they arrive
- **Accumulated streaming** — `StreamTextAccumulated` for growing accumulated text at each step
- **Debounced streaming** — `StreamTextDebounced` for grouped event delivery with configurable window
- **Unified `StreamText`** — Single function with `StreamTextOptions` for all modes

### Extensions
- **Multi-agent team swarms** — Concurrent teammate agents with orchestrator-backed task claiming, dynamic personality generation, and automatic lifecycle management (`ext/team`)
- **Dynamic personality generation** — LLM-generated task-specific system prompts for subagents and teammates with SHA256-keyed caching (`modelutil`)
- **Code mode (monty)** — LLM writes a single Python script that calls N tools as functions; executes in a WASM sandbox via [monty-go](https://github.com/fugue-labs/monty-go) — N tool calls in 1 model round-trip
- **Graph workflow engine** — Typed state machines with conditional branching, fan-out/map-reduce, cycle detection, and Mermaid export
- **Deep context management** — Three-tier compression, planning tools, and checkpointing for long-running agents
- **Temporal activity scaffolding (preview)** — Export named model and tool activities for custom Temporal workflows
- **MCP integration** — Stdio and SSE transports with multi-server management and namespaced tools
- **Evaluation framework** — Datasets, built-in evaluators (`ExactMatch`, `Contains`, `JSONMatch`, `Custom`), LLM-as-judge scoring
- **Persistent memory store** — Namespace-scoped CRUD and search with in-memory and SQLite backends
- **TUI debugger** — Terminal UI with step-mode execution, tool call formatting, and color-coded messages

### Testing
- **`TestModel` mock** — Test agents without real LLM calls using canned responses and call recording
- **`Override` / `WithTestModel`** — Swap models in tests without modifying the original agent
- **561+ tests** across all packages with zero external test dependencies in core

## Quick Start

### Minimal Example (No API Key Required)

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/fugue-labs/gollem"
)

type CityInfo struct {
    Name       string `json:"name" jsonschema:"description=City name"`
    Country    string `json:"country" jsonschema:"description=Country"`
    Population int    `json:"population" jsonschema:"description=Approximate population"`
}

func main() {
    model := gollem.NewTestModel(
        gollem.ToolCallResponse("final_result", `{"name":"Tokyo","country":"Japan","population":14000000}`),
    )

    agent := gollem.NewAgent[CityInfo](model,
        gollem.WithSystemPrompt[CityInfo]("You are a geography expert."),
    )

    result, err := agent.Run(context.Background(), "Tell me about Tokyo")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("City: %s\n", result.Output.Name)       // Tokyo
    fmt.Printf("Country: %s\n", result.Output.Country)  // Japan
    fmt.Printf("Population: %d\n", result.Output.Population) // 14000000
}
```

### Production Agent with Middleware, Cost Tracking, and Guardrails

```go
import (
    "github.com/fugue-labs/gollem"
    "github.com/fugue-labs/gollem/provider/anthropic"
)

model := gollem.NewRetryModel(anthropic.New(), gollem.DefaultRetryConfig())

// Track costs across all runs.
tracker := gollem.NewCostTracker(map[string]gollem.ModelPricing{
    "claude-opus-4-7":            {InputTokenCost: 0.005, OutputTokenCost: 0.025},
    "claude-sonnet-4-6":          {InputTokenCost: 0.003, OutputTokenCost: 0.015},
})

agent := gollem.NewAgent[Analysis](model,
    // Safety
    gollem.WithInputGuardrail[Analysis]("length", gollem.MaxPromptLength(10000)),
    gollem.WithInputGuardrail[Analysis]("content", gollem.ContentFilter("ignore previous instructions")),
    gollem.WithTurnGuardrail[Analysis]("turns", gollem.MaxTurns(20)),

    // Cost & Usage Control
    gollem.WithCostTracker[Analysis](tracker),
    gollem.WithUsageQuota[Analysis](gollem.UsageQuota{MaxRequests: 50, MaxTotalTokens: 100000}),

    // Middleware
    gollem.WithAgentMiddleware[Analysis](gollem.TimingMiddleware(func(d time.Duration) {
        log.Printf("model call took %v", d)
    })),
    gollem.WithAgentMiddleware[Analysis](gollem.LoggingMiddleware(log.Printf)),

    // Intercept PII before it reaches the model
    gollem.WithMessageInterceptor[Analysis](gollem.RedactPII(
        `\b\d{3}-\d{2}-\d{4}\b`, "[SSN REDACTED]",
    )),

    // Observability
    gollem.WithTracing[Analysis](),
    gollem.WithTraceExporter[Analysis](gollem.NewTraceFileExporter("./traces")),
    gollem.WithHooks[Analysis](gollem.Hook{
        OnToolStart: func(ctx context.Context, rc *gollem.RunContext, callID, name, args string) {
            log.Printf("tool: %s(%s)", name, args)
        },
    }),

    // Control
    gollem.WithRunCondition[Analysis](gollem.Or(
        gollem.MaxRunDuration(2 * time.Minute),
        gollem.ToolCallCount(50),
    )),
    gollem.WithDefaultToolTimeout[Analysis](30 * time.Second),
)

result, err := agent.Run(ctx, "Analyze Q4 earnings report")
// result.Cost.TotalCost — cost of this run
// result.Trace — full execution trace
// tracker.TotalCost() — cumulative cost across all runs
```

### Coding Agent Background Processes

The `ext/codetool` package can start long-running commands in the background, surface status through a companion tool, and adopt already-started processes into the same tracking pool.

Use `codetool.AgentOptions(...)` for the recommended automatic lifecycle. It installs `bash` and `bash_status`, cleans up non-`keep_alive` processes at run end, injects completion notifications back into the agent loop, and in team mode creates isolated background-process managers for each worker and delegated subagent.

```go
import (
    "context"

    "github.com/fugue-labs/gollem"
    "github.com/fugue-labs/gollem/ext/codetool"
)

agent := gollem.NewAgent[string](model,
    codetool.AgentOptions("/repo")...,
)

// The model can now call:
//   bash({"command":"npm run dev","background":true,"keep_alive":true})
//   bash_status({"id":"all"})
```

`codetool.Toolset(...)` is now stateless. If you use `Toolset(...)` or `AllTools(...)` directly, pass an explicit `BackgroundProcessManager` and wire lifecycle manually:

```go
mgr := codetool.NewBackgroundProcessManager()
ts := codetool.Toolset(
    codetool.WithWorkDir("/repo"),
    codetool.WithBackgroundProcessManager(mgr),
)

agent := gollem.NewAgent[string](model,
    gollem.WithToolsets[string](ts),
    gollem.WithHooks[string](gollem.Hook{
        OnRunEnd: func(_ context.Context, _ *gollem.RunContext, _ []gollem.ModelMessage, _ error) {
            mgr.Cleanup()
        },
    }),
    gollem.WithDynamicSystemPrompt[string](mgr.CompletionPrompt),
)
```

If you assemble individual tools manually, pass the same manager to both `bash` and `bash_status` so they share the same process pool:

```go
mgr := codetool.NewBackgroundProcessManager()

agent := gollem.NewAgent[string](model,
    gollem.WithTools[string](
        codetool.Bash(
            codetool.WithWorkDir("/repo"),
            codetool.WithBackgroundProcessManager(mgr),
        ),
        codetool.BashStatus(
            codetool.WithBackgroundProcessManager(mgr),
        ),
    ),
)
defer mgr.Cleanup()
```

`BackgroundProcessManager.Adopt(...)` and `AdoptWithWait(...)` are the lower-level APIs for callers that start a process themselves and then want gollem to track it:

```go
cmd := exec.CommandContext(ctx, "bash", "-c", "long-running-command")
stdout, _ := cmd.StdoutPipe()
stderr, _ := cmd.StderrPipe()
if err := cmd.Start(); err != nil {
    return err
}

id, err := mgr.Adopt(cmd, stdout, stderr, "long-running-command")
if err != nil {
    return err
}

fmt.Println("tracking process as", id)
```

Use `AdoptWithWait(...)` instead when your code already wraps `cmd.Wait()` behind a shared `waitFn` and you need the manager to reuse that function instead of calling `cmd.Wait()` directly.

### Multi-Agent Team Swarm

Spawn concurrent teammates that coordinate through orchestrator-backed team tasks. Each teammate gets a dynamically generated personality tailored to its specific task — the LLM itself writes the system prompt.

For durable worker coordination without the team sugar layer, use `ext/orchestrator` directly. It owns tasks, leases, schedulers, runner adapters, task-scoped artifacts, and durable history.

`ext/team` is intentionally thin: teammates claim tasks from an orchestrator-backed store, successful runs complete the claimed task automatically, and blocked work is reported by failing the current task. There is no `TaskBoard` API or mailbox-style note channel in the current model.

If you want durable team state across restarts, pass a dedicated orchestrator backend in `team.TeamConfig.Store` such as `ext/orchestrator/sqlite`. The store should back one team instance so its tasks, commands, recovery sweeps, and history stay scoped together.

```go
import (
    "github.com/fugue-labs/gollem/ext/team"
    "github.com/fugue-labs/gollem/modelutil"
)

// Create a team with dynamic personality generation (enabled by default).
t := team.NewTeam(team.TeamConfig{
    Name:   "code-review",
    Leader: "lead",
    Model:  model,
    Toolset: codingTools, // bash, edit, grep, etc.
    PersonalityGenerator: modelutil.CachedPersonalityGenerator(
        modelutil.GeneratePersonality(model),
    ),
})

leader := gollem.NewAgent[string](model,
    gollem.WithTools[string](team.LeaderTools(t)...),
)

// Teammates run as goroutines with fresh context windows and claim
// orchestrator-backed team tasks assigned to them.
t.SpawnTeammate(ctx, "reviewer", "Review auth module for security vulnerabilities")
t.SpawnTeammate(ctx, "tester", "Write comprehensive tests for the payment flow")
t.SpawnTeammate(ctx, "docs", "Update API documentation for the new endpoints")

// The leader coordinates by creating and inspecting tasks.
result, _ := leader.Run(ctx, "Coordinate the code review across all teammates")

t.Shutdown(ctx)
```

If your teammate toolset contains per-worker state, use `team.TeamConfig.ToolsetFactory` instead of sharing a single `Toolset`. This is the right pattern for stateful helpers such as background-process managers. `codetool.AgentOptions(...)` handles that automatically in team mode.

### Multi-Agent with Event Coordination

```go
import "github.com/fugue-labs/gollem/core/orchestration"

bus := gollem.NewEventBus()

type TaskAssigned struct {
    AgentName string
    Task      string
}

gollem.Subscribe[TaskAssigned](bus, func(e TaskAssigned) {
    log.Printf("Agent %s received: %s", e.AgentName, e.Task)
})

researcher := gollem.NewAgent[ResearchResult](model,
    gollem.WithEventBus[ResearchResult](bus),
    gollem.WithSystemPrompt[ResearchResult]("You are a research specialist."),
)

orchestrator := gollem.NewAgent[FinalReport](model,
    gollem.WithEventBus[FinalReport](bus),
    gollem.WithTools[FinalReport](
        orchestration.AgentTool("research", "Delegate research tasks", researcher),
    ),
)

result, _ := orchestrator.Run(ctx, "Research and summarize recent advances in robotics")
```

Agents attached to an event bus also publish built-in runtime lifecycle events: `RunStartedEvent`, `ToolCalledEvent`, and `RunCompletedEvent`. Those events include `RunID`, `ParentRunID` for nested runs, and timestamps so local orchestration and adapters can trace lineage without scraping transcripts.

### Composable Pipelines

```go
// Build a processing pipeline that chains agents and transforms.
pipeline := gollem.NewPipeline(
    gollem.AgentStep(researcher),
    gollem.TransformStep(func(s string) string {
        return "Summarize the following research:\n" + s
    }),
    gollem.AgentStep(writer),
)

// Or use parallel fan-out with automatic result joining.
pipeline = pipeline.Then(gollem.ParallelSteps(
    gollem.AgentStep(factChecker),
    gollem.AgentStep(editor),
))

// Conditional branching based on content.
pipeline = pipeline.Then(gollem.ConditionalStep(
    func(s string) bool { return len(s) > 5000 },
    gollem.AgentStep(summarizer),  // long content
    gollem.TransformStep(strings.TrimSpace), // short content
))

result, _ := pipeline.Run(ctx, "Research quantum computing advances")
```

### Typed Dependencies in Tools

```go
type AppDeps struct {
    DB     *sql.DB
    Cache  *redis.Client
    APIKey string
}

queryTool := gollem.FuncTool[struct{ SQL string }](
    "query_db", "Execute a database query",
    func(ctx context.Context, rc *gollem.RunContext, p struct{ SQL string }) (string, error) {
        deps := gollem.GetDeps[*AppDeps](rc)  // compile-time type safe
        rows, err := deps.DB.QueryContext(ctx, p.SQL)
        // ...
    },
)

agent := gollem.NewAgent[Report](model,
    gollem.WithTools[Report](queryTool),
    gollem.WithDeps[Report](&AppDeps{DB: db, Cache: cache, APIKey: key}),
)
```

### Batch Processing with Model Routing

```go
// Route simple queries to a fast model, complex ones to a powerful model.
router := gollem.NewRouterModel(gollem.ThresholdRouter(
    fastModel,    // short prompts
    powerModel,   // long prompts
    500,          // character threshold
))

agent := gollem.NewAgent[Summary](router)

results := agent.RunBatch(ctx, []string{
    "Summarize: Go is great.",
    "Analyze the geopolitical implications of semiconductor supply chain disruptions across ASEAN nations...",
}, gollem.WithBatchConcurrency(10))

for _, r := range results {
    if r.Err != nil {
        log.Printf("prompt %d failed: %v", r.Index, r.Err)
        continue
    }
    fmt.Println(r.Result.Output)
}
```

## Core Concepts

### Agents

The `Agent[T]` is the central type. It orchestrates the loop of sending messages to an LLM, processing tool calls, and extracting a typed result. The type parameter `T` determines the output type — a struct for structured data, or `string` for free-form text.

```go
// Structured output agent.
agent := gollem.NewAgent[MyStruct](model, opts...)
result, _ := agent.Run(ctx, "prompt")
fmt.Println(result.Output.SomeField)

// Free-form text agent.
textAgent := gollem.NewAgent[string](model, opts...)
textResult, _ := textAgent.Run(ctx, "prompt")
fmt.Println(textResult.Output)
```

### Tools

Tools give agents the ability to call Go functions. Use `FuncTool` to create type-safe tools:

```go
type SearchParams struct {
    Query string `json:"query" jsonschema:"description=Search query"`
    Limit int    `json:"limit" jsonschema:"description=Max results,default=10"`
}

searchTool := gollem.FuncTool[SearchParams](
    "search",
    "Search the knowledge base",
    func(ctx context.Context, params SearchParams) (string, error) {
        return doSearch(params.Query, params.Limit), nil
    },
)

agent := gollem.NewAgent[string](model,
    gollem.WithTools[string](searchTool),
    gollem.WithToolResultValidator[string](func(_ context.Context, name, result string) error {
        if result == "" {
            return fmt.Errorf("empty result from %s", name)
        }
        return nil
    }),
    gollem.WithDefaultToolTimeout[string](10 * time.Second),
)
```

### Structured Output

Gollem uses a "final_result" tool pattern to extract structured output from LLMs. The framework generates a JSON Schema from `T` and presents it as a tool the model must call. If parsing fails, the optional repair function attempts a fix before retrying:

```go
type Analysis struct {
    Sentiment  string   `json:"sentiment" jsonschema:"enum=positive|negative|neutral"`
    Keywords   []string `json:"keywords" jsonschema:"description=Key topics"`
    Confidence float64  `json:"confidence" jsonschema:"description=Confidence 0-1"`
}

agent := gollem.NewAgent[Analysis](model,
    gollem.WithOutputRepair[Analysis](gollem.ModelRepair[Analysis](repairModel)),
    gollem.WithOutputValidator[Analysis](func(a Analysis) error {
        if a.Confidence < 0 || a.Confidence > 1 {
            return fmt.Errorf("confidence out of range: %f", a.Confidence)
        }
        return nil
    }),
)
```

### Streaming

Use `RunStream` for real-time token streaming with Go 1.23+ iterators, or the new streaming options for fine-grained control:

```go
stream, _ := agent.RunStream(ctx, "Write a story about a robot")

// Standard streaming.
for text, err := range stream.StreamText(true) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(text) // prints tokens as they arrive
}

// Or use streaming options for more control.
for delta, err := range gollem.StreamTextDelta(rawStream) {
    fmt.Print(delta) // raw incremental chunks
}

for accumulated, err := range gollem.StreamTextAccumulated(rawStream) {
    updateUI(accumulated) // growing text at each step
}

for text, err := range gollem.StreamTextDebounced(rawStream, 100*time.Millisecond) {
    sendToClient(text) // grouped delivery for network efficiency
}
```

### Providers

All providers implement the `Model` interface, making them interchangeable. Wrap any provider with resilience:

```go
import (
    "github.com/fugue-labs/gollem/provider/anthropic"
    "github.com/fugue-labs/gollem/provider/openai"
    "github.com/fugue-labs/gollem/provider/vertexai"
    "github.com/fugue-labs/gollem/provider/vertexai_anthropic"
)

// Raw providers — each reads credentials from environment.
claude := anthropic.New()
gpt := openai.New()
gemini := vertexai.New("my-project", "us-central1")
vertexClaude := vertexai_anthropic.New("my-project", "us-east5")

// Wrap with retry, rate limiting, and caching.
resilient := gollem.NewRetryModel(
    gollem.NewRateLimitedModel(
        gollem.NewCachedModel(claude, gollem.NewMemoryCacheWithTTL(5*time.Minute)),
        10, // requests per second
        20, // burst capacity
    ),
    gollem.DefaultRetryConfig(),
)
```

OpenAI provider note:
- `OPENAI_TRANSPORT=websocket` (or `openai.WithTransport("websocket")`) enables
  Responses-API WebSocket continuation optimization for tool-heavy loops.
- Current limitation: this path is non-streaming (`Request()` flow). Streaming
  UI output still relies on provider streaming support via `RequestStream()`.

| Feature | Anthropic | OpenAI | Vertex AI | Vertex AI Anthropic |
|---------|-----------|--------|-----------|---------------------|
| Structured output | Yes | Yes | Yes | Yes |
| Streaming | Yes | Yes | Yes | Yes |
| Tool use | Yes | Yes | Yes | Yes |
| Extended thinking | Yes | -- | -- | Yes |
| Prompt caching | Yes | -- | -- | Yes |
| Native JSON mode | -- | Yes | Yes | -- |
| Auth | API key | API key | OAuth2 (GCP) | OAuth2 (GCP) |

## Advanced Features

### Agent Middleware

Wrap model calls with cross-cutting concerns. Middleware compose like HTTP middleware — first registered is outermost:

```go
agent := gollem.NewAgent[string](model,
    // Outermost: timing wraps everything.
    gollem.WithAgentMiddleware[string](gollem.TimingMiddleware(func(d time.Duration) {
        metrics.RecordLatency("model_call", d)
    })),
    // Middle: logging.
    gollem.WithAgentMiddleware[string](gollem.LoggingMiddleware(log.Printf)),
    // Innermost: token limit enforcement.
    gollem.WithAgentMiddleware[string](gollem.MaxTokensMiddleware(4096)),
    // Custom middleware can skip the model call entirely.
    gollem.WithAgentMiddleware[string](func(ctx context.Context, messages []gollem.ModelMessage, settings *gollem.ModelSettings, params *gollem.ModelRequestParameters, next func(context.Context, []gollem.ModelMessage, *gollem.ModelSettings, *gollem.ModelRequestParameters) (*gollem.ModelResponse, error)) (*gollem.ModelResponse, error) {
        if shouldUseCache(messages) {
            return cachedResponse, nil // skip model call
        }
        return next(ctx, messages, settings, params)
    }),
)
```

### Message Interceptors

Filter, modify, or block messages before they reach the model or after responses return:

```go
agent := gollem.NewAgent[string](model,
    // Redact SSNs before they leave your system.
    gollem.WithMessageInterceptor[string](gollem.RedactPII(
        `\b\d{3}-\d{2}-\d{4}\b`, "[SSN REDACTED]",
    )),
    // Audit log all messages for compliance.
    gollem.WithMessageInterceptor[string](gollem.AuditLog(func(direction string, messages []gollem.ModelMessage) {
        auditDB.Record(direction, messages)
    })),
    // Custom interceptor to strip sensitive headers.
    gollem.WithResponseInterceptor[string](func(ctx context.Context, resp *gollem.ModelResponse) gollem.InterceptResult {
        sanitize(resp)
        return gollem.InterceptResult{Action: gollem.MessageAllow}
    }),
)
```

### Cost Tracking & Usage Quotas

Monitor spend in real-time and enforce hard limits:

```go
tracker := gollem.NewCostTracker(map[string]gollem.ModelPricing{
    "claude-opus-4-7":            {InputTokenCost: 0.005, OutputTokenCost: 0.025},
    "claude-sonnet-4-6":          {InputTokenCost: 0.003, OutputTokenCost: 0.015},
    "gpt-4o":                    {InputTokenCost: 0.005, OutputTokenCost: 0.015},
})

agent := gollem.NewAgent[string](model,
    gollem.WithCostTracker[string](tracker),
    gollem.WithUsageQuota[string](gollem.UsageQuota{
        MaxRequests:     100,
        MaxTotalTokens:  500000,
        MaxOutputTokens: 100000,
    }),
)

result, err := agent.Run(ctx, "prompt")
if err != nil {
    var qe *gollem.QuotaExceededError
    if errors.As(err, &qe) {
        log.Printf("quota exceeded: %s", qe.Message)
    }
}

// Per-run and cumulative cost visibility.
fmt.Printf("Run cost: $%.4f\n", result.Cost.TotalCost)
fmt.Printf("Total spend: $%.4f\n", tracker.TotalCost())
breakdown := tracker.CostBreakdown()
for model, cost := range breakdown {
    fmt.Printf("  %s: $%.4f\n", model, cost)
}
```

### Tool Choice Control

Direct which tools the model can use:

```go
agent := gollem.NewAgent[string](model,
    gollem.WithTools[string](searchTool, calcTool, writeTool),

    // Force the model to use a specific tool on the first call.
    gollem.WithToolChoice[string](gollem.ToolChoiceForce("search")),

    // Auto-reset to "auto" after the first tool call to prevent infinite loops.
    gollem.WithToolChoiceAutoReset[string](),
)
```

### Model Capability Profiles

Query model capabilities and route based on requirements:

```go
// Models can self-declare capabilities.
profile := gollem.GetProfile(model)
fmt.Printf("Supports vision: %v\n", profile.SupportsVision)
fmt.Printf("Max context: %d tokens\n", profile.MaxContextTokens)

// Route to the first model that supports your requirements.
router := gollem.NewCapabilityRouter(
    []gollem.Model{fastModel, powerModel, visionModel},
    gollem.ModelProfile{SupportsVision: true, SupportsToolCalls: true},
)
```

### Prompt Templates

Use Go's `text/template` syntax for dynamic, reusable prompts:

```go
tmpl := gollem.MustTemplate("analyst", `You are a {{.Role}} specializing in {{.Domain}}.
Analyze the following with {{.Depth}} depth.`)

agent := gollem.NewAgent[Analysis](model,
    gollem.WithSystemPromptTemplate[Analysis](tmpl),
)

// Variables resolved from RunContext.Deps
result, _ := agent.Run(ctx, "Analyze Q4 results",
    gollem.WithRunDeps(map[string]string{
        "Role": "senior analyst", "Domain": "fintech", "Depth": "comprehensive",
    }),
)
```

### Conversation Memory Strategies

Manage context windows intelligently across long conversations:

```go
import "github.com/fugue-labs/gollem/core/memory"

// Keep only the last 10 message pairs.
agent := gollem.NewAgent[string](model,
    gollem.WithHistoryProcessor[string](memory.SlidingWindowMemory(10)),
)

// Stay within a token budget.
agent := gollem.NewAgent[string](model,
    gollem.WithHistoryProcessor[string](memory.TokenBudgetMemory(4000)),
)

// Summarize old messages using a model.
agent := gollem.NewAgent[string](model,
    gollem.WithHistoryProcessor[string](memory.SummaryMemory(summaryModel, 20)),
)

// Auto context compression (transparent overflow handling).
agent := gollem.NewAgent[string](model,
    gollem.WithAutoContext[string](gollem.AutoContextConfig{
        MaxTokens:    100000,
        KeepLastN:    10,
        SummaryModel: summaryModel, // optional
    }),
)
```

### Agent Composition

Clone agents for variant configurations, or chain them for multi-stage pipelines:

```go
// Clone with overrides — original is never modified.
verbose := agent.Clone(
    gollem.WithTemperature[Analysis](0.9),
    gollem.WithMaxTokens[Analysis](4000),
)

// Chain agents — first output becomes second input.
summary, _ := orchestration.ChainRun(ctx, researcher, writer, "Topic: AI safety",
    func(research ResearchResult) string {
        return fmt.Sprintf("Write an article based on: %s", research.Summary)
    },
)
```

### Task Orchestration

Use `ext/orchestrator` directly when the source of truth should be durable tasks, leases, runs, artifacts, and control/history instead of freeform teammate notes.

```go
import (
    "github.com/fugue-labs/gollem/core"
    "github.com/fugue-labs/gollem/ext/orchestrator"
    memstore "github.com/fugue-labs/gollem/ext/orchestrator/memory"
)

store := memstore.NewStore()
runner := orchestrator.NewAgentRunner(workerAgent,
    orchestrator.WithTaskArtifacts(func(task *orchestrator.Task, result *core.RunResult[WorkerOutput]) []orchestrator.ArtifactSpec {
        return []orchestrator.ArtifactSpec{{
            Kind:        "report",
            Name:        "handoff.md",
            ContentType: "text/markdown",
            Body:        []byte("# Handoff\n\nScheduler path reviewed."),
        }}
    }),
)
scheduler := orchestrator.NewScheduler(store, store, runner,
    orchestrator.WithWorkerID("worker-1"),
)

task, _ := store.CreateTask(ctx, orchestrator.CreateTaskRequest{
    Kind:    "analysis",
    Subject: "Review scheduler path",
    Input:   "Summarize the scheduler path and capture a handoff artifact.",
})

go scheduler.Run(ctx)

// The scheduler/store persists the task result and emitted artifacts together.
```

See [`examples/orchestrator/main.go`](examples/orchestrator/main.go) for a full runnable in-memory example that drives a task through the scheduler and persists an artifact as part of task completion.

For persistent orchestration state across process restarts, use the SQLite-backed store:

```go
import (
    "time"

    "github.com/fugue-labs/gollem/ext/orchestrator"
    orchestratorsqlite "github.com/fugue-labs/gollem/ext/orchestrator/sqlite"
)

store, _ := orchestratorsqlite.NewStore("orchestrator.db")

task, _ := store.CreateTask(ctx, orchestrator.CreateTaskRequest{
    Kind:  "analysis",
    Input: "Review the scheduler path and persist durable history.",
})
claim, _ := store.ClaimTask(ctx, task.ID, orchestrator.ClaimTaskRequest{
    WorkerID: "worker-1",
    LeaseTTL: time.Minute,
})

events, _ := store.ListEvents(ctx, orchestrator.EventFilter{TaskID: task.ID})
_ = events // append-ordered durable history with monotonically increasing Sequence

timeline, _ := orchestrator.LoadTaskTimeline(ctx, store, task.ID)
_ = timeline // decoded task lifecycle projection over durable history

runTimeline, _ := orchestrator.LoadRunTimeline(ctx, store, claim.Run.ID)
_ = runTimeline // decoded per-run lifecycle projection over durable history

runSummary, _ := orchestrator.GetRun(ctx, store, claim.Run.ID)
_ = runSummary // projected run status, worker, attempt, and terminal kind

runs, _ := orchestrator.ListRuns(ctx, store, orchestrator.RunFilter{TaskID: task.ID})
_ = runs // projected run summaries for this task

workerSummary, _ := orchestrator.GetWorker(ctx, store, "worker-1")
_ = workerSummary // projected worker totals and latest durable run attribution

workers, _ := orchestrator.ListWorkers(ctx, store, orchestrator.WorkerFilter{})
_ = workers // projected worker summaries across the durable store

activeRuns, _ := orchestrator.ListActiveRuns(ctx, store, orchestrator.ActiveRunFilter{WorkerID: "worker-1"})
_ = activeRuns // current running tasks for this worker from task store state

pendingCommands, _ := orchestrator.ListPendingCommandsForWorker(ctx, store, "worker-1")
_ = pendingCommands // currently claimable durable commands for this worker

expiredLeases, _ := orchestrator.ListExpiredLeases(ctx, store, time.Now())
_ = expiredLeases // currently expired leases that recovery would reclaim, oldest first

staleCommands, _ := orchestrator.ListStaleClaimedCommands(ctx, store, time.Now().Add(-time.Minute))
_ = staleCommands // currently claimed commands old enough to release back to pending

leaseRecoveries, _ := orchestrator.ListLeaseRecoveries(ctx, store, orchestrator.RecoveryHistoryFilter{Limit: 20})
_ = leaseRecoveries // durable record of recovered leases and their outcomes

commandRecoveries, _ := orchestrator.ListCommandRecoveries(ctx, store, orchestrator.RecoveryHistoryFilter{Limit: 20})
_ = commandRecoveries // durable record of recovered claimed commands

recovery := orchestrator.NewRecoveryManager(store, store,
    orchestrator.WithRecoveryCommandClaimTimeout(time.Minute),
)
sweep, _ := recovery.Sweep(ctx, time.Now())
_ = sweep // reclaimed leases/commands; add WithRecoveryController(...) for durable remote run cancel
```

When you pass a SQLite-backed store to helpers like `ListActiveRuns`, `GetActiveRun`, `ListPendingCommandsForWorker`, or `ListStaleClaimedCommands`, they use store-native indexed queries instead of scanning the full task or command set.

See [`examples/orchestrator_sqlite/main.go`](examples/orchestrator_sqlite/main.go) for a full runnable SQLite example that reopens the store, inspects durable history, and queries worker/current-state projections.

### Multi-Agent Team Swarms

Spawn teams of concurrent agents that coordinate through orchestrator-backed shared tasks. Each teammate runs as a goroutine with its own context window and tools.

If you want durable work coordination without the team sugar layer, use `ext/orchestrator` directly and treat `ext/team` as convenience sugar.

The source of truth is the underlying orchestrator store. `ext/team` adds teammate lifecycle helpers and task-oriented prompts on top of that store; it does not maintain a parallel task board or note-delivery subsystem.

```go
import "github.com/fugue-labs/gollem/ext/team"

// Create a team. Teammates get coding tools plus thin task-oriented team tools.
t := team.NewTeam(team.TeamConfig{
    Name:    "refactor",
    Leader:  "lead",
    Model:   model,
    Toolset: codingTools,
    // Optional: inject a dedicated durable backend for this team.
    // Store: orchestratorsqlite.NewStore("refactor-team.db"),
})

// Spawn teammates — each runs concurrently in its own goroutine.
t.SpawnTeammate(ctx, "analyzer", "Analyze the codebase for dead code and unused imports")
t.SpawnTeammate(ctx, "migrator", "Migrate database queries from raw SQL to the ORM")

// The leader coordinates through orchestrator-backed tasks.
leader := gollem.NewAgent[string](model,
    gollem.WithTools[string](team.LeaderTools(t)...),
)
result, _ := leader.Run(ctx, "Coordinate the refactoring effort")

// Graceful shutdown — asks teammates to stop after their current task.
t.Shutdown(ctx)
```

**Direct orchestrator access:**

```go
import "github.com/fugue-labs/gollem/ext/orchestrator"

// ext/team is thin sugar over the orchestrator store it owns.
store := t.Store()

// Inspect all current team tasks directly via the orchestrator API.
tasks, _ := store.ListTasks(ctx, orchestrator.TaskFilter{
    Kinds: []string{"team"},
})

// Read artifacts emitted by completed tasks.
artifacts, _ := store.ListArtifacts(ctx, orchestrator.ArtifactFilter{
    TaskID: tasks[0].ID,
})
```

### Dynamic Personality Generation

Instead of static system prompts, let the LLM generate a task-specific personality for each subagent. A "write tests for auth" agent gets a different persona than a "refactor database layer" agent — better focus, better results.

```go
import "github.com/fugue-labs/gollem/modelutil"

// Generate a personality — the model writes a system prompt tailored to the task.
gen := modelutil.GeneratePersonality(model)

prompt, _ := gen(ctx, modelutil.PersonalityRequest{
    Task:        "Review Go code for concurrency bugs and race conditions",
    Role:        "senior concurrency reviewer",
    BasePrompt:  "You are a coding assistant.", // extended, not replaced
    Constraints: []string{"Focus only on goroutine safety", "Ignore style issues"},
})
// prompt is now a rich, task-specific system prompt written by the model itself.

// Wrap with caching to avoid redundant LLM calls for identical tasks.
cached := modelutil.CachedPersonalityGenerator(gen)

// Use with teams — every teammate gets a unique personality.
t := team.NewTeam(team.TeamConfig{
    Name:                 "review-team",
    Leader:               "lead",
    Model:                model,
    PersonalityGenerator: cached,
})
```

Personality generation is **enabled by default** when using the codetool toolset — no configuration needed. Every subagent and teammate automatically gets a tailored system prompt.

### State Snapshots & Time-Travel Debugging

Capture and restore agent state for debugging, branching, or replay:

```go
var checkpoint *gollem.RunSnapshot

agent := gollem.NewAgent[string](model,
    gollem.WithHooks[string](gollem.Hook{
        OnModelResponse: func(ctx context.Context, rc *gollem.RunContext, resp *gollem.ModelResponse) {
            checkpoint = gollem.Snapshot(rc) // capture state
        },
    }),
)

agent.Run(ctx, "original prompt")

// Branch from checkpoint and explore an alternative path.
alt := checkpoint.Branch(func(snap *gollem.RunSnapshot) {
    snap.Prompt = "alternative prompt"
})

// Serialize for storage or debugging.
data, _ := gollem.MarshalSnapshot(checkpoint)
restored, _ := gollem.UnmarshalSnapshot(data)
```

### Trace Artifacts

The `gollem` CLI can write canonical `gollem.trace.v1` artifacts for local
inspection, runtime-boundary replay, and trace diffing:

```bash
gollem run --trace-out run.trace.json "Fix the failing tests"
gollem trace export --temporal workflow-123 --out workflow.trace.json
gollem trace inspect run.trace.json
gollem trace replay run.trace.json
gollem trace fork run.trace.json --from-kind model.responded --system-prompt planner-v2.txt --append-user "try a simpler fix" --out fork.snapshot.json
gollem run --resume-snapshot fork.snapshot.json --trace-out fork.trace.json "continue"
gollem trace diff baseline.trace.json variant.trace.json
gollem trace redact run.trace.json --pattern "$API_KEY" --out redacted.trace.json
gollem trace compact run.trace.json --out compact.trace.json
```

The persisted file format is always `gollem.trace.v1`. `core.RunTrace` is the
in-memory trace struct used to build that artifact. Older raw trace JSON files
from previous Gollem versions can still be converted by `gollem trace export`
for backward compatibility. Forked snapshots use the same snapshot format as
`core.WithSnapshot`, and resumed runs emit a fresh trace segment rather than
prepending or appending the source trace. The restored conversation history may
still appear inside the new segment's outbound model request payload because it
is part of the resumed agent state. CLI fork/resume traces include source
lineage metadata such as the source trace run id and source snapshot id. Strict
replay validates that recorded model/tool/runtime boundaries are internally
paired before rendering. `redact` and `compact` provide local-only sharing and
archival workflows for sensitive or large traces.

SDK users can configure a canonical trace directory in code:

```go
agent := core.NewAgent[string](model,
    core.WithTraceExporter[string](core.NewTraceFileExporter("./traces")),
)
```

When a run needs additional runtime-boundary events from a shared event bus
(delegate/team child runs, approvals, waits, deferred results), use the
`ext/trace` directory exporter. It writes the same core `gollem.trace.v1`
artifact shape with those extra events included:

```go
bus := core.NewEventBus()
defer bus.Close()
recorder := trace.NewRuntimeRecorder(bus)
defer recorder.Close()

agent := core.NewAgent[string](model,
    core.WithEventBus[string](bus),
    core.WithTraceExporter[string](trace.NewDirectoryExporter("./traces",
        trace.WithRuntimeRecorder(recorder),
        trace.WithExporterMetadata(map[string]any{"component": "worker"}),
    )),
)
```

For Kubernetes or other multi-worker deployments, prefer durable storage over
pod-local trace directories. `ext/trace` includes an object-storage exporter
that writes the same canonical artifact to a deterministic key, so a retried
Temporal `trace_export` activity overwrites or deduplicates the same object
rather than creating a new trace form:

```go
type MyObjectStore struct{}

func (MyObjectStore) PutObject(ctx context.Context, object trace.ObjectPut) error {
    // Write object.Body to S3, GCS, R2, MinIO, or another durable store.
    return nil
}

agent := core.NewAgent[string](model,
    core.WithTraceExporter[string](trace.NewObjectStorageExporter(MyObjectStore{},
        trace.WithObjectKeyPrefix("gollem/prod"),
        trace.WithObjectExporterMetadata(map[string]any{"component": "worker"}),
    )),
)
```

Coding agents built with `codetool.AgentOptions` write canonical trace
artifacts to `/tmp/gollem-traces` by default. Override or disable that from
code with:

```go
opts := codetool.AgentOptions(workDir,
    codetool.WithTraceDir("./traces"),
)

// Disable file export while keeping in-memory RunResult.Trace.
opts = codetool.AgentOptions(workDir,
    codetool.WithTraceDir(""),
)
```

When tracing is enabled, delegate subagents and team teammates share the same
runtime event bus, so nested agent runs appear in the parent artifact with
their own run IDs and parent-run causality. Orchestrator-backed task runners can
persist canonical trace artifacts via `trace.OrchestratorArtifactSpec`; team
workers do this automatically for completed tasks.

Dashboard runs started with `gollem serve` record the same artifact in memory
after completion. The run page links to `/runs/<id>/trace` for JSON export and
`/runs/<id>/trace/inspect` for the human-readable trace summary.

### Code Mode (monty)

Instead of N sequential tool calls (N model round-trips), the LLM writes a single Python script that calls tools as functions. The [monty-go](https://github.com/fugue-labs/monty-go) WASM interpreter executes the script in a sandbox, pausing at each function call so the corresponding gollem tool handler runs. Result: N tool calls in 1 round-trip.

```go
import (
    montygo "github.com/fugue-labs/monty-go"
    "github.com/fugue-labs/gollem/ext/monty"
)

runner, _ := montygo.New()
defer runner.Close()

searchTool := gollem.FuncTool[SearchParams]("search", "Search docs", doSearch)
calcTool := gollem.FuncTool[CalcParams]("calculate", "Run calculations", doCalc)

cm := monty.New(runner, []gollem.Tool{searchTool, calcTool})

agent := gollem.NewAgent[string](model,
    gollem.WithSystemPrompt[string](cm.SystemPrompt()),
    gollem.WithTools[string](cm.Tool()),
)

// The LLM now writes Python like:
//   results = search(query="Q4 revenue")
//   total = calculate(a=results["count"], b=10)
//   total
// All tool calls execute in a single model round-trip.
result, _ := agent.Run(ctx, "Search and calculate Q4 metrics")
```

Tools with `RequiresApproval` are automatically excluded (can't pause mid-script for human approval). CodeMode is safe for concurrent use.

### Graph Workflow Engine

Build typed state machines for complex multi-step workflows:

```go
import "github.com/fugue-labs/gollem/ext/graph"

type OrderState struct {
    OrderID string
    Status  string
    Total   float64
}

g := graph.NewGraph[OrderState]()
g.AddNode(graph.Node[OrderState]{
    Name: "validate",
    Run: func(ctx context.Context, s *OrderState) (string, error) {
        if s.Total <= 0 {
            return graph.EndNode, fmt.Errorf("invalid total")
        }
        return "process", nil
    },
})
g.AddNode(graph.Node[OrderState]{
    Name: "process",
    Run: func(ctx context.Context, s *OrderState) (string, error) {
        s.Status = "processed"
        return graph.EndNode, nil
    },
})
g.SetEntryPoint("validate")

finalState, _ := g.Run(ctx, OrderState{OrderID: "123", Total: 99.99})
```

### Deep Context Management

Three-tier context compression for agents that handle massive context windows:

```go
import "github.com/fugue-labs/gollem/ext/deep"

cm := deep.NewContextManager(model,
    deep.WithMaxContextTokens(100000),
    deep.WithOffloadThreshold(20000),
    deep.WithCompressionThreshold(0.85),
)

agent := gollem.NewAgent[string](model,
    gollem.WithHistoryProcessor[string](cm.AsHistoryProcessor()),
)

// Or use the all-in-one LongRunAgent.
lra := deep.NewLongRunAgent[string](model,
    deep.WithContextWindow[string](100000),
    deep.WithPlanningEnabled[string](),
)
result, _ := lra.Run(ctx, "Analyze this large codebase...")
```

### Temporal Durable Execution

Durable Temporal workflow support for gollem agents:

```go
import "github.com/fugue-labs/gollem/ext/temporal"

ta := temporal.NewTemporalAgent(agent,
    temporal.WithName("my-agent"),
    temporal.WithVersion("2026_03"),
    temporal.WithContinueAsNew(temporal.ContinueAsNewConfig{
        MaxTurns:         50,
        MaxHistoryLength: 10000,
        OnSuggested:      true,
    }),
    temporal.WithActivityConfig(temporal.ActivityConfig{
        StartToCloseTimeout: 120 * time.Second,
        MaxRetries:          3,
    }),
)

w := worker.New(client, "my-queue", worker.Options{})
_ = temporal.RegisterAll(w, ta)

run, _ := client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
    TaskQueue: "my-queue",
}, ta.WorkflowName(), temporal.WorkflowInput{
    Prompt: "Summarize the current project status",
})

var output temporal.WorkflowOutput
_ = run.Get(ctx, &output)
result, _ := ta.DecodeWorkflowOutput(&output)
```

`NewTemporalAgent` validates construction-time invariants; `ta.Run(...)` still
executes in-process, while `RunWorkflow`, `Register(...)`, and
`RegisterAll(...)` are the durable entry points. `WithVersion(...)` gives the
workflow/activity names a stable deployment suffix, and
`WithContinueAsNew(...)` rolls long runs into fresh workflow executions while
preserving snapshot state. Use `ta.StatusQueryName()` to query
`WorkflowStatus`, which includes workflow identity, current history metrics,
continue-as-new counters, and readable structured `Messages`, `Snapshot`, and
`Trace` payloads for operator-facing inspection. `WorkflowStatus` and
`WorkflowOutput` also expose the logical Temporal workflow id, the current
Temporal run id, the continue-as-new run chain, and trace export status/errors.
Signal
`ta.ApprovalSignalName()` for tools marked with `WithRequiresApproval()`,
signal `ta.DeferredResultSignalName()` to resolve deferred tool calls, and
signal `ta.AbortSignalName()` to abort a waiting workflow. The current
activity-backed callback surface includes dynamic
system prompts, history processors, input/turn guardrails, lifecycle hooks,
run conditions, tool preparation callbacks, request middleware,
message/response interceptors, output repair/validation, custom
`WithToolApproval(...)` callbacks, knowledge-base retrieval/storage, usage
quota checks, toolsets, tool result validators, tracing, trace exporters, cost
estimates, event bus integration, agent deps, and auto-context compression.
The built-in workflow uses non-streaming model requests; the streaming model
activity is available for custom workflows. JSON-valued workflow/activity
payloads are emitted as nested JSON so Temporal history stays readable, while
the legacy raw `*JSON` fields remain as decode fallbacks for older histories.

In a Kubernetes deployment with multiple workers polling the same task queue,
Temporal, not the worker process, is the trace unifier. Workflow tasks for a
single workflow execution are serialized through Temporal history, and
`RunWorkflow` rebuilds its trace state from that history no matter which worker
pod receives the next workflow task. The cluster-safe export path is to read
`WorkflowStatus.Trace` / `WorkflowOutput.Trace`, or run:

```bash
gollem trace export --temporal <workflow-id> --out workflow.trace.json
```

`core.NewTraceFileExporter(...)` still works with Temporal, but it runs inside
the final `trace_export` activity on whichever worker pod picks up that
activity. `WorkflowOutput.TraceExport` records whether that export was
attempted, how many exporters succeeded or failed, and each non-fatal exporter
error. In Kubernetes, a local directory is pod-local unless it is a shared
volume, so production deployments should prefer `gollem trace export
--temporal`, a shared filesystem, `trace.NewObjectStorageExporter(...)`, or a
custom exporter that writes to durable storage such as a database. The
in-memory `EventBus` and `RuntimeRecorder` are process-local conveniences; they
are not a cluster-wide trace aggregation mechanism.

See [`ext/temporal/README.md`](ext/temporal/README.md) for the full execution
model, payload shapes, status/signal API, dep override flow, continue-as-new
behavior, custom workflow hooks, and current caveats. The runnable example is
[`examples/temporal/main.go`](examples/temporal/main.go), which starts a real
worker, runs a durable workflow, queries waiting status, and signals tool
approval.

### Evaluation Framework

Test agent quality with datasets and composable evaluators:

```go
import "github.com/fugue-labs/gollem/ext/eval"

dataset := eval.Dataset[string]{
    Name: "geography",
    Cases: []eval.Case[string]{
        {Name: "capital-france", Prompt: "What is the capital of France?", Expected: "Paris"},
        {Name: "capital-japan", Prompt: "What is the capital of Japan?", Expected: "Tokyo"},
    },
}

runner := eval.NewRunner(agent, eval.Contains())
report, _ := runner.Run(ctx, dataset)
fmt.Printf("Score: %.0f%% (%d/%d passed)\n",
    report.AvgScore*100, report.PassedCases, report.TotalCases)
```

### MCP Integration

Connect to Model Context Protocol servers for external tool discovery:

```go
import mcpclient "github.com/fugue-labs/gollem/ext/mcp"

client, _ := mcpclient.NewStdioClient(ctx, "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp")
defer client.Close()

// Multi-server manager with namespaced tools.
mgr := mcpclient.NewManager()
mgr.AddClient("fs", client)
mgr.AddClient("db", sseClient)
allTools, _ := mgr.Tools(ctx) // "fs__read", "db__query", etc.
```

### Middleware

Compose cross-cutting concerns around model requests at the provider level:

```go
import "github.com/fugue-labs/gollem/ext/middleware"

wrapped := middleware.Wrap(model,
    middleware.NewLogging(logger),
    middleware.NewOTel("my-service"),
)

agent := gollem.NewAgent[string](wrapped)
```

## Examples

| Example | Description |
|---------|-------------|
| [`examples/simple`](examples/simple) | Basic `Agent[CityInfo]` with structured output |
| [`examples/tools`](examples/tools) | Tool use with `FuncTool` |
| [`examples/streaming`](examples/streaming) | Real-time streaming with `iter.Seq2` |
| [`examples/multi-provider`](examples/multi-provider) | Same agent across different providers |
| [`examples/mcp`](examples/mcp) | MCP server integration |
| [`examples/temporal`](examples/temporal) | Durable Temporal workflow with query + approval signal |
| [`examples/evaluation`](examples/evaluation) | Evaluation framework with datasets |
| [`examples/multi-agent/delegation`](examples/multi-agent/delegation) | Agent-as-tool delegation |
| [`examples/deep/context_management`](examples/deep/context_management) | Three-tier context compression |
| [`examples/graph`](examples/graph) | Graph workflow state machine |
| [`ext/team`](ext/team) | Multi-agent team swarms as thin sugar over orchestrator tasks |

## Testing

Gollem provides `TestModel` and test helpers for verifying agent logic without real LLM calls:

```go
func TestMyAgent(t *testing.T) {
    model := gollem.NewTestModel(
        gollem.ToolCallResponse("final_result", `{"status":"ok"}`),
    )

    agent := gollem.NewAgent[MyOutput](model)
    result, err := agent.Run(context.Background(), "test prompt")

    require.NoError(t, err)
    assert.Equal(t, "ok", result.Output.Status)

    // Inspect what was sent to the model.
    calls := model.Calls()
    assert.Len(t, calls, 1)
}

func TestWithOverride(t *testing.T) {
    // Swap model in production agent without modifying original.
    testAgent, testModel := gollem.WithTestModel[MyOutput](productionAgent,
        gollem.ToolCallResponse("final_result", `{"status":"ok"}`),
    )
    result, _ := testAgent.Run(ctx, "test")
    assert.Equal(t, 1, len(testModel.Calls()))
}
```

## Terminal-Bench Submissions

Before opening a Terminal-Bench 2.0 leaderboard PR, validate your submission folder locally:

```bash
make tbench-validate-submission SUBMISSION_DIR=submissions/terminal-bench/2.0/<agent>__<model>
```

For full requirements and common failure modes, see:

- [AGENTS.md](AGENTS.md)
- [contrib/tbench_submission_checklist.md](contrib/tbench_submission_checklist.md)

## Contributing

Contributions are welcome. Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, code style, testing requirements, and the pull request process.

## License

MIT License — Copyright (c) 2026 [Trevor Prater](https://github.com/trevorprater)

See [LICENSE](LICENSE) for the full text.
