# Proposal: Add LLM/AI Agent Instrumentation Package

**Title:** Proposal: Add LLM/AI agent instrumentation package

**Labels:** enhancement, proposal

---

## Summary

I'd like to propose adding an OpenTelemetry instrumentation package for LLM/AI agent frameworks in Go. As LLM-powered applications become more prevalent in production, there's a growing need for standardized observability that follows OpenTelemetry semantic conventions.

## Context

LLM agent frameworks orchestrate multi-step interactions with language models, including tool calls, streaming responses, and token-intensive workloads. Today, each Go framework implements its own ad-hoc metrics and tracing, leading to inconsistent dashboards and alert configurations across organizations.

The OpenTelemetry ecosystem already has emerging [semantic conventions for generative AI](https://opentelemetry.io/docs/specs/semconv/gen-ai/), but there is no reference instrumentation package for Go agent frameworks.

## Existing Implementation

We have a production-ready implementation in [gollem](https://github.com/fugue-labs/gollem), a Go LLM agent framework. The middleware provides both tracing and metrics instrumentation:

**Source:** https://github.com/fugue-labs/gollem/blob/main/ext/middleware/otel.go

### Metric Instruments (6 total)

| Instrument | Type | Description |
|---|---|---|
| `core.requests` | Int64Counter | Total number of model requests |
| `core.errors` | Int64Counter | Total number of failed model requests |
| `core.tokens.input` | Int64Counter | Total input tokens consumed |
| `core.tokens.output` | Int64Counter | Total output tokens consumed |
| `core.request.duration` | Float64Histogram | Duration of model requests in seconds |
| `core.tool_calls` | Int64Counter | Total number of tool calls in responses |

### Span Tracing

- **`core.request`** span (SpanKindClient) for synchronous requests
  - Attributes: `core.message_count`, `core.model`, `core.input_tokens`, `core.output_tokens`, `core.finish_reason`, `core.tool_calls`
- **`core.stream_request`** span (SpanKindClient) for streaming requests
  - Same attributes, finalized when the stream closes
  - Tracks token usage and tool calls across the full stream lifecycle

### Design Highlights

- Pluggable `TracerProvider` and `MeterProvider` via functional options
- Implements both `Middleware` (sync) and `StreamMiddleware` (streaming) interfaces
- Stream instrumentation wraps the response to finalize spans on close, ensuring accurate duration and token counts
- Zero-allocation on the hot path for metric recording

## Proposal

Extract and generalize this implementation into an instrumentation package under `instrumentation/`:

```
instrumentation/github.com/fugue-labs/gollem/otelgollem
```

This would:

1. Provide a reference implementation for LLM agent instrumentation in Go
2. Align metric names with emerging GenAI semantic conventions (renaming `core.*` to `gen_ai.*` where appropriate)
3. Serve as a template for instrumenting other Go LLM frameworks (e.g., langchaingo)
4. Include comprehensive tests, examples, and documentation

## Alignment with Semantic Conventions

The proposed metric names would map to the [GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/):

| Current Name | Proposed Name | Semantic Convention |
|---|---|---|
| `core.requests` | `gen_ai.client.operation.duration` | Aligned |
| `core.tokens.input` | `gen_ai.client.token.usage` (input) | Aligned |
| `core.tokens.output` | `gen_ai.client.token.usage` (output) | Aligned |
| `core.tool_calls` | `gen_ai.client.tool_calls` | New (proposed) |
| `core.errors` | `gen_ai.client.errors` | New (proposed) |

## Willingness to Contribute

I am willing to:
- Submit the implementation PR with full test coverage
- Sign the CNCF CLA
- Iterate on API design based on maintainer feedback
- Align with any in-progress GenAI semantic convention work

## Related Work

- [OpenTelemetry GenAI Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/)
- [opentelemetry-python-contrib GenAI instrumentation](https://github.com/open-telemetry/opentelemetry-python-contrib/tree/main/instrumentation-genai)
- [gollem OTel middleware source](https://github.com/fugue-labs/gollem/blob/main/ext/middleware/otel.go)
