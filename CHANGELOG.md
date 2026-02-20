# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Phase 11: Ten Innovations

#### Innovation 1: KnowledgeBase Interface
- `KnowledgeBase` interface for pluggable RAG, graph databases, and memory services
- Agent transparently calls `Retrieve()` before each request and `Store()` after successful runs
- `StaticKnowledgeBase` for testing and simple use cases
- `WithKnowledgeBase` and `WithKnowledgeBaseAutoStore` agent options

#### Innovation 2: Message Serialization API
- `MarshalMessages` / `UnmarshalMessages` for JSON round-trip of conversations
- Envelope pattern with `kind` and `type` discriminators for all part types
- `RunResult.AllMessagesJSON()` and `NewMessagesJSON()` helpers

#### Innovation 3: Multimodal Message Parts
- `ImagePart`, `AudioPart`, `DocumentPart` types implementing `ModelRequestPart`
- `BinaryContent()` helper for base64 data: URI generation
- Full serialization support for multimodal parts

#### Innovation 4: Tool Prepare Functions
- Per-tool `PrepareFunc` for dynamic include/exclude/modify at each agent step
- Agent-wide `WithToolsPrepare` for bulk tool filtering
- Context-based tool availability (e.g., hide tools based on run state)

#### Innovation 5: Deferred Tool Calls
- `CallDeferred` error type for tools that pause the agent for external resolution
- `RunResultDeferred` / `ErrDeferred` for clean deferred signaling
- `WithDeferredResults` to resume runs with externally-resolved tool results
- Mixed deferred and normal tool calls in the same step

#### Innovation 6: Graph Fan-Out / Map-Reduce
- `FanOutNode` for parallel branch execution via goroutines
- `Send[S]` directives and `ReduceFunc` for state merging
- Error propagation from parallel branches
- Mermaid diagram support for fan-out nodes

#### Innovation 7: Checkpoint Replay, Fork, and Tool State
- `GetHistory()` for browsing checkpoint history
- `ReplayFrom()` to resume from any checkpoint step
- `ForkFrom()` to branch with modified state
- `StatefulTool` interface for tool state persistence across checkpoints
- `ExportToolStates` / `RestoreToolStates` for checkpoint-aware tools

#### Innovation 8: Persistent Memory Store
- `Store` interface with namespace-scoped CRUD and search
- `MemoryStore` (in-memory, thread-safe) implementation
- `SQLiteStore` (persistent, pure-Go via modernc.org/sqlite)
- `StoreKnowledgeBase` adapter bridging Store to KnowledgeBase interface
- `MemoryTool` for agent-accessible memory operations

#### Innovation 9: Step-by-Step Evaluation
- `StepEvaluator` interface for per-step scoring
- Built-in evaluators: `MaxStepsEvaluator`, `NoRetryEvaluator`
- Step scores in `CaseResult` and aggregated reports

#### Innovation 10: TUI Agent Debugger
- Terminal UI using bubbletea with color-coded message display
- Step mode (press 's') and auto mode (press 'a') for agent execution
- Tool call formatting, usage stats, and scroll navigation
- `cmd/gollem` CLI entry point for interactive debugging

### Phase 10: Innovations
- Provider fallback chains — FallbackModel tries multiple models in order until one succeeds
- Rate limiting middleware — token bucket rate limiter with configurable rps and burst
- Retry middleware with exponential backoff — configurable max retries, delay caps, RetryIf predicates
- Request/response caching middleware — SHA-256 hash-based cache with TTL expiration and stats
- Reflection/self-correction pattern — RunWithReflection loops output through a validator with configurable iterations

### Phase 9: Documentation, README & Examples
- Comprehensive README.md with quick start, architecture diagram, and feature documentation
- CONTRIBUTING.md with development setup, code style, and PR process
- CHANGELOG.md documenting all phases
- New examples: temporal, evaluation, multi-agent delegation, deep context management, graph workflows

### Phase 8: Extended MCP & Observability
- SSE transport for MCP servers
- Multi-server Manager with namespaced tool aggregation
- ToolSource interface for unified client usage
- OpenTelemetry tracing and metrics middleware
- Streaming middleware support

### Phase 7: Evaluation Framework
- Dataset and Case types for structured evaluation
- Built-in evaluators: ExactMatch, Contains, JSONMatch, Custom, LLMJudge
- Runner with multi-evaluator support
- Report aggregation with pass/fail scoring

### Phase 6: Multi-Agent Framework
- Agent delegation via AgentTool
- Sequential handoff pipelines
- Typed graph engine with conditional branching and cycle detection
- Mermaid diagram generation

### Phase 5: Temporal Durable Execution
- TemporalModel wrapping model requests as activities
- Tool call wrapping as activities
- TemporalAgent orchestrator
- Activity collection for worker registration

### Phase 4: Deep Package -- Planning & Checkpointing
- Planning tool for multi-step task coherence
- Checkpoint save/load/resume system
- Custom JSON serialization for ModelMessage interfaces
- LongRunAgent wrapper combining all deep features

### Phase 3: Deep Package -- Context Management
- Three-tier context compression (offload large results, offload inputs, LLM summarization)
- Token estimation utility
- Filesystem-backed context store
- ContextManager as HistoryProcessor

### Phase 2: Core Framework Enhancements
- Dynamic system prompts (WithDynamicSystemPrompt)
- History processors (WithHistoryProcessor)
- Human-in-the-loop tool approval (WithToolApproval)
- Node-by-node agent iteration (Agent.Iter)
- Concurrency and tool call limits
- Toolsets for grouped tool management

### Phase 1: Go Best Practices & Infrastructure
- Makefile with comprehensive targets
- golangci-lint v2 configuration
- GitHub Actions CI/CD workflows
- MIT License, .gitignore, codecov config
- Testable examples
