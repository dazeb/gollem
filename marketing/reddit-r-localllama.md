# gollem + Ollama + monty-go: A single-binary local agent stack

If you're running local models with Ollama, you might be interested in this: I built a Go agent framework that lets you run a fully-featured AI agent stack as a single binary — no Docker, no pip, no node_modules, no virtual environments. Just one Go binary + Ollama.

**GitHub**: https://github.com/fugue-labs/gollem
**WASM sandbox**: https://github.com/fugue-labs/monty-go

## The setup

Install Ollama, pull a model, build your Go binary. That's it.

```bash
ollama pull llama3
go build -o my-agent ./cmd/agent
./my-agent
```

Your entire agent — LLM client, tool execution, guardrails, cost tracking, structured output — is one static binary. No runtime dependencies.

## Connecting to Ollama

gollem has a built-in Ollama provider via the OpenAI-compatible API:

```go
import "github.com/fugue-labs/gollem/provider/openai"

// Connect to local Ollama — no API key needed
model := openai.NewOllama(openai.WithModel("llama3"))

// Or point to a remote Ollama instance
model := openai.NewOllama(
    openai.WithBaseURL("http://gpu-box:11434"),
    openai.WithModel("mistral"),
)
```

Now build a typed agent with tools:

```go
type ResearchResult struct {
    Topic   string   `json:"topic"`
    Summary string   `json:"summary"`
    Sources []string `json:"sources"`
}

type SearchParams struct {
    Query string `json:"query" jsonschema:"description=Search query"`
}

searchTool := gollem.FuncTool[SearchParams](
    "search", "Search the local knowledge base",
    func(ctx context.Context, p SearchParams) (string, error) {
        return searchLocalDB(p.Query)
    },
)

agent := gollem.NewAgent[ResearchResult](model,
    gollem.WithTools[ResearchResult](searchTool),
    gollem.WithSystemPrompt[ResearchResult]("You are a local research assistant."),
)

result, err := agent.Run(ctx, "Summarize recent papers on RLHF")
fmt.Println(result.Output.Summary) // typed, compile-time safe
```

## Code mode: N tool calls in 1 round-trip

This is where it gets interesting. With [monty-go](https://github.com/fugue-labs/monty-go), the LLM writes Python code that calls your tools as functions. monty-go executes it in a WASM sandbox — no containers, no subprocess, no `exec()`. Just a 2.9MB WASM binary embedded in your Go binary.

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
```

Instead of making 3 separate tool calls (3 model round-trips), the LLM writes:

```python
q3 = search(query="Q3 revenue")
q4 = search(query="Q4 revenue")
growth = calculate(expression=f"({q4['revenue']} - {q3['revenue']}) / {q3['revenue']} * 100")
{"q3": q3, "q4": q4, "growth_rate": growth}
```

One model call, three tool executions. This matters a lot with local models where each round-trip has latency.

## Why this matters for local-first

| | Traditional Python stack | gollem + Ollama |
|---|---|---|
| **Install** | python, pip, venv, langchain, ... | `go build` (one binary) |
| **Runtime deps** | Python interpreter, packages | None (static binary) |
| **Code execution** | Docker/subprocess/E2B | WASM sandbox (embedded) |
| **Memory overhead** | ~150MB+ (Python + deps) | ~15MB (Go binary) |
| **Startup** | 1-3 seconds | ~5ms |
| **Security** | Container isolation | WASM sandbox per execution |
| **Cross-compile** | Painful | `GOOS=linux go build` |

The WASM sandbox from monty-go is particularly relevant for local setups. You get safe code execution without needing Docker or any container runtime. Each execution gets a fresh, isolated WASM instance with configurable resource limits:

```go
montygo.WithLimits(montygo.Limits{
    MaxDuration:       5 * time.Second,
    MaxMemoryBytes:    10 * 1024 * 1024, // 10 MB
    MaxAllocations:    100000,
    MaxRecursionDepth: 100,
})
```

No filesystem access, no network access, no state leaks between calls. The LLM can write whatever Python it wants — it can't escape the sandbox.

## Full feature list

Even though you're running locally, you get the full production framework:

- **Structured output** — typed results via generics, not string parsing
- **Guardrails** — input validation, turn limits, content filtering, output repair
- **Agent middleware** — logging, timing, custom middleware chains
- **Cost tracking** — monitor token usage even with local models
- **Multi-agent** — agent delegation, handoff pipelines, event coordination
- **Graph workflows** — state machines with conditional branching
- **Streaming** — Go 1.23 `iter.Seq2` iterators for real-time tokens
- **Testing** — `TestModel` for unit testing without any LLM
- **MCP integration** — connect to MCP servers for external tools
- **561+ tests**, MIT licensed

## The vision

One Go binary that contains your entire agent: LLM client, tools, guardrails, WASM sandbox for code execution. Point it at your Ollama instance and you have a production-grade local agent stack. No Docker compose. No dependency management. No "works on my machine."

Would love to hear from others running local agent setups. What does your stack look like? What tools are you using for agent orchestration with Ollama?

https://github.com/fugue-labs/gollem
https://github.com/fugue-labs/monty-go
