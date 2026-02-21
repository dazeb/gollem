# Go Weekly Submission

## Title

gollem: A Production Agent Framework for Go

## URL

https://github.com/fugue-labs/gollem

## Description

gollem is a type-safe, zero-dependency agent framework for building LLM-powered applications in Go. It provides a generic `Agent[T]` with compile-time output typing, reflection-based tool schema generation from Go structs, multi-provider streaming (Anthropic, OpenAI, Vertex AI), guardrails, cost tracking, agent middleware, and multi-agent orchestration — all without external dependencies in core.

## Key Highlights

- **Generic `Agent[T]`** — output type checked at compile time, not runtime
- **5 LLM providers** with a unified `Model` interface (Anthropic, OpenAI, Vertex AI, Vertex AI Anthropic, Ollama)
- **`FuncTool[P]`** creates tools from typed Go functions with auto-generated JSON Schema via struct tags
- **Go 1.23 `iter.Seq2` streaming** — range-over-function iterators for real-time tokens
- **50+ composable primitives**: guardrails, middleware, interceptors, cost tracking, usage quotas, retry, caching, rate limiting
- **Multi-agent**: AgentTool delegation, Handoff pipelines, typed event bus, composable pipelines with fan-out
- **Code mode**: LLM writes Python, WASM sandbox executes it — N tool calls in 1 model round-trip (via monty-go)
- **Zero core dependencies**, 561+ tests, MIT licensed
- **`TestModel`** for testing agent logic without real LLM calls

## Link

https://github.com/fugue-labs/gollem
