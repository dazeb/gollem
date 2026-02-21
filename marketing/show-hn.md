# Show HN: Gollem – Production Go agent framework (zero deps, type-safe, multi-provider)

I've been building AI agents in production for the past year. Started with Python (LangChain, then pydantic-ai), and kept hitting the same problems: runtime type errors at 3am, dependency hell in containers, slow cold starts, and the nagging feeling that my deployment pipeline was more complex than my actual agent logic.

So I built Gollem — a production agent framework in Go.

Here's a typed agent with tools in ~15 lines:

```go
type SearchParams struct {
    Query string `json:"query" jsonschema:"description=Search query"`
}

searchTool := gollem.FuncTool[SearchParams](
    "search", "Search the web", doSearch,
)

agent := gollem.NewAgent[Summary](anthropic.New(),
    gollem.WithTools[Summary](searchTool),
    gollem.WithSystemPrompt[Summary]("You are a research assistant."),
)

result, err := agent.Run(ctx, "Find recent papers on RLHF")
fmt.Println(result.Output.Title) // compile-time field access
```

**Why Go for AI agents?**

- **Type safety that actually matters.** `Agent[T]` is generic over your output type. Tool parameters use struct tags for JSON Schema generation. If your tool expects an `int` and the schema says `string`, the compiler tells you — not your pager.

- **Goroutines are a natural fit for streaming.** Go 1.23 `iter.Seq2` iterators make token streaming feel native: `for text, err := range stream.StreamText(true) { ... }`. No async/await coloring. No callback pyramids.

- **Single-binary deployment.** `go build` and `scp`. That's the deployment story. No virtualenvs, no pip, no Docker layers for Python runtimes. Our CI produces a 15MB static binary that runs anywhere.

- **Zero core dependencies.** The gollem core imports only the standard library. Provider packages pull in minimal SDKs. Your dependency tree stays small and auditable.

**What's included (50+ composable primitives):**

- Generic `Agent[T]` with structured output via "final_result" tool pattern
- 5 LLM providers: Anthropic, OpenAI, Google Vertex AI, Vertex AI Anthropic, Ollama (via OpenAI compat)
- `FuncTool[P]` with reflection-based JSON Schema from Go structs
- Guardrails: input validation, turn limits, output repair, content filtering
- Agent middleware chain (like HTTP middleware for model calls)
- Message interceptors with built-in PII redaction
- Cost tracking with per-model pricing and usage quotas
- Retry, rate limiting, response caching, model routing
- Multi-agent: AgentTool delegation, Handoff pipelines, typed event bus, composable pipelines
- Graph workflow engine with conditional branching and Mermaid export
- Code mode via monty-go: LLM writes Python, executes in WASM sandbox, N tool calls in 1 round-trip
- MCP integration (stdio + SSE)
- Evaluation framework, conversation memory, deep context management
- `TestModel` for testing agents without LLM calls
- 561+ tests across all packages

**Compared to Python frameworks:**

I have genuine respect for pydantic-ai — it's well-designed and the closest analog to what gollem does. But the language-level differences matter in production:

| | gollem (Go) | pydantic-ai (Python) |
|---|---|---|
| Type checking | Compile-time (generics) | Runtime (Pydantic) |
| Deployment | `go build` → single binary | pip + venv + Docker |
| Startup | ~5ms | ~500ms-2s |
| Memory | ~15MB RSS | ~80-150MB RSS |
| Streaming | `iter.Seq2` (native) | async generators |
| Concurrency | Goroutines | asyncio |
| Testing | `TestModel` (no mocks needed) | Requires mocking |
| Dependencies | 0 in core | pydantic, httpx, etc. |

If your team is already in Go, or you want production agent deployments that are simple and fast, gollem might be worth a look.

MIT licensed. Feedback and contributions welcome.

https://github.com/fugue-labs/gollem
