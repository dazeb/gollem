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
feature has a runnable example, most of which work without API keys. The complete feature
catalog with extended examples lives in [docs/REFERENCE.md](docs/REFERENCE.md).

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

MIT — see [LICENSE](LICENSE).
