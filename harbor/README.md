# gollem-harbor

Harbor adapter for running the [gollem](https://github.com/fugue-labs/gollem) coding agent on [Terminal-Bench 2.0](https://www.tbench.ai/).

## Quick Start

```bash
# Install dependencies
cd harbor
uv pip install -e .

# Run on Terminal-Bench 2.0
harbor run \
  -d terminal-bench@2.0 \
  --agent-import-path gollem_harbor:GollemAgent \
  -m anthropic/claude-sonnet-4-5 \
  --env docker \
  -n 4

# With extended thinking
harbor run \
  -d terminal-bench@2.0 \
  --agent-import-path gollem_harbor:GollemAgent \
  -m anthropic/claude-sonnet-4-5 \
  --env docker
```

## How It Works

1. Harbor creates a Docker container for each benchmark task
2. The `install.sh.j2` template installs Go and builds the `gollem` binary inside the container
3. `gollem run` executes with the task instruction, using the full coding agent toolset (bash, view, edit, grep, glob, ls, write)
4. The verification checkpoint forces the agent to run tests/builds before declaring completion
5. Results are collected and scored by Harbor

## Supported Providers

- `anthropic/claude-*` — Anthropic models
- `openai/gpt-*` — OpenAI models
- `google/gemini-*` — Google Vertex AI models

## Environment Variables

Set your API key before running:

```bash
export ANTHROPIC_API_KEY="sk-..."
# or
export OPENAI_API_KEY="sk-..."
```
