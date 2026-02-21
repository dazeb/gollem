# I built a production Go agent framework as an alternative to Python AI frameworks

I've spent the last year building AI agents in production and kept running into the same friction with Python frameworks. Runtime type errors showing up in production, complex deployment pipelines, slow startup times, and dependency graphs that made me nervous every time I ran `pip install`.

So I built **gollem** — a production agent framework written entirely in Go.

**GitHub**: https://github.com/fugue-labs/gollem

## Why Go for AI agents?

The AI agent ecosystem is dominated by Python, but Go's strengths align surprisingly well with what production agents actually need:

1. **Compile-time type safety.** `Agent[T]` is generic over your output type. Tool parameters are Go structs with JSON Schema generated via reflection. If your structured output schema and your tool params don't match, you know at compile time — not when a user hits the right edge case at 3am.

2. **Goroutines for streaming.** Token streaming is inherently concurrent. Go 1.23's `iter.Seq2` iterators make it feel native — no async/await function coloring, no callback pyramids.

3. **Single binary deployment.** `go build`, `scp`, done. No virtualenv, no pip, no multi-stage Docker builds to trim your Python image from 1.2GB to 400MB.

4. **Zero core dependencies.** The gollem core package imports only the Go standard library. Your `go.sum` stays clean.

## What does it look like?

Here's a complete typed agent with a tool:

```go
type WeatherParams struct {
    City string `json:"city" jsonschema:"description=City name"`
}

type WeatherReport struct {
    City    string  `json:"city"`
    TempC   float64 `json:"temp_c"`
    Summary string  `json:"summary"`
}

weatherTool := gollem.FuncTool[WeatherParams](
    "get_weather", "Get current weather for a city",
    func(ctx context.Context, p WeatherParams) (string, error) {
        // your real weather API call here
        return fmt.Sprintf(`{"city":"%s","temp":22}`, p.City), nil
    },
)

agent := gollem.NewAgent[WeatherReport](anthropic.New(),
    gollem.WithTools[WeatherReport](weatherTool),
    gollem.WithSystemPrompt[WeatherReport]("You are a weather assistant."),
)

result, err := agent.Run(ctx, "What's the weather in Tokyo?")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("%s: %.1f°C - %s\n", result.Output.City, result.Output.TempC, result.Output.Summary)
```

## Production features

The framework ships 50+ composable primitives:

**Safety & Control:**
```go
agent := gollem.NewAgent[Analysis](model,
    // Input validation
    gollem.WithInputGuardrail[Analysis]("length", gollem.MaxPromptLength(10000)),
    gollem.WithInputGuardrail[Analysis]("content", gollem.ContentFilter("ignore previous")),
    gollem.WithTurnGuardrail[Analysis]("turns", gollem.MaxTurns(20)),

    // Cost tracking with per-model pricing
    gollem.WithCostTracker[Analysis](tracker),
    gollem.WithUsageQuota[Analysis](gollem.UsageQuota{
        MaxRequests: 100, MaxTotalTokens: 500000,
    }),

    // PII redaction before data reaches the model
    gollem.WithMessageInterceptor[Analysis](gollem.RedactPII(
        `\b\d{3}-\d{2}-\d{4}\b`, "[SSN REDACTED]",
    )),
)
```

**Multi-agent orchestration:**
```go
// One agent delegates to another as a tool
researcher := gollem.NewAgent[Research](model, ...)
writer := gollem.NewAgent[Article](model,
    gollem.WithTools[Article](
        orchestration.AgentTool("research", "Delegate research tasks", researcher),
    ),
)

// Or chain agents in a pipeline
pipeline := gollem.NewPipeline(
    gollem.AgentStep(researcher),
    gollem.TransformStep(formatForWriter),
    gollem.AgentStep(writer),
)
result, _ := pipeline.Run(ctx, "Write about quantum computing")
```

**Streaming with Go 1.23 iterators:**
```go
stream, _ := agent.RunStream(ctx, "Write a story")
for text, err := range stream.StreamText(true) {
    if err != nil {
        log.Fatal(err)
    }
    fmt.Print(text) // tokens as they arrive
}
```

**Testing without LLM calls:**
```go
func TestMyAgent(t *testing.T) {
    model := gollem.NewTestModel(
        gollem.ToolCallResponse("final_result", `{"status":"ok"}`),
    )
    agent := gollem.NewAgent[MyOutput](model)
    result, err := agent.Run(context.Background(), "test prompt")
    require.NoError(t, err)
    assert.Equal(t, "ok", result.Output.Status)
}
```

## What's in the box

- **5 LLM providers**: Anthropic, OpenAI, Vertex AI (Gemini), Vertex AI (Claude), Ollama
- **Agent middleware** (like HTTP middleware for model calls): timing, logging, max tokens, custom
- **Model wrappers**: retry with backoff, rate limiting, response caching, capability-based routing
- **Conversation memory**: sliding window, token budget, summary, auto context compression
- **Code mode**: LLM writes Python, executes in WASM sandbox via [monty-go](https://github.com/fugue-labs/monty-go) — N tool calls in 1 model round-trip
- **Graph workflow engine** with conditional branching and Mermaid export
- **MCP integration** (stdio + SSE transports)
- **Evaluation framework** with datasets and LLM-as-judge scoring
- **561+ tests**, MIT licensed

## Honest comparison with pydantic-ai

I have a lot of respect for pydantic-ai — it's well-designed and the closest Python analog. Here's where the approaches differ:

| Aspect | gollem (Go) | pydantic-ai (Python) |
|--------|-------------|---------------------|
| Type safety | Compile-time (Go generics) | Runtime (Pydantic validators) |
| Tool schemas | Struct tags + reflection | Decorator + docstring parsing |
| Deployment | `go build` → static binary | pip + venv + Docker |
| Startup time | ~5ms | ~500ms-2s |
| Memory footprint | ~15MB RSS | ~80-150MB RSS |
| Streaming | `iter.Seq2` (native iterators) | async generators |
| Concurrency | Goroutines (native) | asyncio (colored functions) |
| Testing | `TestModel` (built-in) | Requires external mocking |
| Core deps | 0 | pydantic, httpx, etc. |

## Questions for the community

- For those using Go in ML/AI adjacent work — what's your current approach for LLM integration?
- Is there interest in additional provider support (Bedrock, Mistral, etc.)?
- What would make you consider Go over Python for an agent project?

I'd love feedback. The framework is MIT licensed and contributions are welcome.
