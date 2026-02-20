# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

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
