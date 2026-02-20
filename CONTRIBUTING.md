# Contributing to gollem

Thank you for your interest in contributing to gollem. This guide covers the development setup, code style, testing requirements, and pull request process.

## Development Setup

### Prerequisites

- **Go 1.23+** (uses `iter.Seq2` range-over-function iterators)
- **golangci-lint v2** for linting
- **goimports** for formatting
- **govulncheck** for vulnerability scanning (optional but recommended)

### Getting Started

```bash
# Clone the repository.
git clone https://github.com/trevorprater/gollem.git
cd gollem

# Install dependencies.
go mod download

# Run the full CI pipeline locally.
make ci
```

### Available Make Targets

| Target | Description |
|--------|-------------|
| `make help` | Show all available targets |
| `make test` | Run tests with race detector |
| `make test-verbose` | Run tests with verbose output |
| `make coverage` | Generate HTML coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Run goimports formatting |
| `make vet` | Run go vet |
| `make vulncheck` | Run govulncheck |
| `make tidy` | Run go mod tidy and verify |
| `make clean` | Remove build artifacts |
| `make ci` | Run full CI pipeline (lint + vet + test + vulncheck) |
| `make doc` | Start local pkgsite documentation server |

## Code Style

### Formatting

All code must be formatted with `goimports`:

```bash
make fmt
```

Import groups should be ordered as:
1. Standard library
2. External dependencies
3. Internal packages (`github.com/trevorprater/gollem/...`)

### Linting

All code must pass `golangci-lint`:

```bash
make lint
```

### Conventions

- **Functional options pattern** for configuration: `WithXxx` functions returning option types.
- **Zero external dependencies** in the root `gollem` package. Only sub-packages may import external libraries.
- **Import cycle prevention**: The root `gollem` package must never import sub-packages. All core interfaces and types live in the root package. Provider sub-packages import `gollem` (one-way dependency).
- **Generic types** use `[T any]` type parameters where appropriate.
- **Error handling**: Return `error` values rather than panicking. Use sentinel error types for expected failure modes.
- Use `nolint` directives sparingly and always include a justification comment.

### Commit Messages

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add SSE transport for MCP servers
fix: prevent race condition in concurrent tool execution
docs: update README with evaluation framework examples
test: add integration tests for Vertex AI provider
refactor: extract history processing into separate package
```

## Testing

### Requirements

- All new code must include tests.
- Tests must pass with the race detector enabled: `go test -race ./...`
- Use `TestModel` for unit testing agent logic without real LLM calls.
- Use `httptest.NewServer` for provider integration tests.
- Use `rewriteTransport` and `staticTokenSource` patterns for testing GCP-authenticated providers.

### Running Tests

```bash
# Run all tests.
make test

# Run tests for a specific package.
go test -race ./deep/...

# Run a specific test.
go test -race -run TestContextManager ./deep/...

# Generate coverage report.
make coverage
```

### Test Patterns

**Agent unit test using TestModel:**

```go
func TestMyFeature(t *testing.T) {
    model := gollem.NewTestModel(
        gollem.ToolCallResponse("final_result", `{"result":"ok"}`),
    )
    agent := gollem.NewAgent[MyOutput](model)
    result, err := agent.Run(context.Background(), "test")
    require.NoError(t, err)
    assert.Equal(t, "ok", result.Output.Result)
}
```

**Provider integration test:**

```go
func TestProviderRequest(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Return mock API response.
    }))
    defer server.Close()
    // Configure provider to use test server URL.
}
```

## Pull Request Process

1. **Fork and branch**: Create a feature branch from `main`. Use a descriptive name like `feat/sse-transport` or `fix/race-condition`.

2. **Make your changes**: Follow the code style and conventions above.

3. **Write tests**: Ensure your changes are tested. Aim for coverage of the happy path and key error cases.

4. **Run CI locally**: Before pushing, run `make ci` to verify everything passes.

5. **Open a PR**: Write a clear description of what changed and why. Reference any related issues.

6. **Review**: Address any feedback from reviewers. Keep commits clean; squash fixup commits before merge.

### PR Checklist

- [ ] Code compiles without errors (`go build ./...`)
- [ ] All tests pass with race detector (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] New code has test coverage
- [ ] Public APIs have godoc comments
- [ ] Commit messages follow Conventional Commits
- [ ] No import cycles introduced

## Architecture Notes

The codebase is organized to prevent import cycles:

```
gollem/              # Root package: all core types and interfaces
  anthropic/         # Anthropic provider (imports gollem)
  openai/            # OpenAI provider (imports gollem)
  vertexai/          # Vertex AI provider (imports gollem)
  vertexai_anthropic/# Vertex AI Anthropic provider (imports gollem)
  middleware/        # Middleware chain (imports gollem)
  mcp/               # MCP client (imports gollem)
  memory/            # Conversation memory (imports gollem)
  deep/              # Context management, planning, checkpoints (imports gollem)
  temporal/          # Temporal durable execution (imports gollem)
  graph/             # Workflow graph engine (imports gollem)
  eval/              # Evaluation framework (imports gollem)
  examples/          # Runnable examples
```

The root `gollem` package defines all interfaces (`Model`, `StreamedResponse`, `ModelMessage`, etc.) and core types (`Agent[T]`, `Tool`, `FuncTool`, `TestModel`). Sub-packages import `gollem` but never the reverse. This one-way dependency prevents import cycles and keeps the core package free of external dependencies.
