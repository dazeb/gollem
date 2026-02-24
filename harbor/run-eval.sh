#!/bin/bash
# Terminal-Bench 2.0 evaluation runner for gollem
# Usage: ./run-eval.sh [model] [concurrency] [tasks]
#   model: provider/model (default: google/gemini-3-flash-preview)
#   concurrency: number of parallel tasks (default: 2)
#   tasks: comma-separated task names (default: all non-excluded tasks)
#
# Examples:
#   ./run-eval.sh                                        # all tasks, gemini flash
#   ./run-eval.sh google/gemini-3-flash-preview 4        # 4 concurrent
#   ./run-eval.sh anthropic/claude-sonnet-4-20250514 2   # claude sonnet
#   ./run-eval.sh google/gemini-3-flash-preview 1 gpt2-codegolf,chess-best-move

MODEL="${1:-google/gemini-3-flash-preview}"
CONCURRENCY="${2:-2}"
TASKS="${3:-}"  # Optional: comma-separated task names, or "all" for everything

# Tasks that require Selenium/Chromium (won't work under QEMU on ARM Mac)
SELENIUM_TASKS="break-filter-js-from-html,filter-js-from-html"

# Load API keys
if [[ -f ~/.anthropic-key ]]; then
  export ANTHROPIC_API_KEY=$(sed -n 's/^ANTHROPIC_API_KEY=//p' ~/.anthropic-key | tr -d '[:space:]')
fi

# Auto-detect xAI models and set base URL
if [[ "$OPENAI_API_KEY" == xai-* ]] && [[ -z "$OPENAI_BASE_URL" ]]; then
  export OPENAI_BASE_URL="https://api.x.ai"
fi

# Verify we have the right API key for the provider
PROVIDER="${MODEL%%/*}"
case "$PROVIDER" in
  anthropic)
    echo "Provider: anthropic | Key: ${ANTHROPIC_API_KEY:+${#ANTHROPIC_API_KEY} chars}"
    ;;
  openai)
    echo "Provider: openai | Key: ${OPENAI_API_KEY:+${#OPENAI_API_KEY} chars} | Base: ${OPENAI_BASE_URL:-default}"
    ;;
  google|vertexai|vertex)
    echo "Provider: vertexai | Project: ${GOOGLE_CLOUD_PROJECT:-not set}"
    ;;
  *)
    echo "Provider: $PROVIDER"
    ;;
esac

echo "Model: $MODEL | Concurrency: $CONCURRENCY"

# Build Linux binary with latest changes
echo "Building gollem-linux-amd64..."
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "$SCRIPT_DIR/gollem-linux-amd64" "$REPO_ROOT/cmd/gollem/" || exit 1
echo "Build complete."

# Construct extra agent kwargs for Google/Vertex AI
EXTRA_ARGS=()
if [[ "$PROVIDER" == "google" || "$PROVIDER" == "vertexai" || "$PROVIDER" == "vertex" ]]; then
  EXTRA_ARGS+=(--ak "location=global")
fi

# Build task selection args
TASK_ARGS=()
if [[ -n "$TASKS" ]]; then
  IFS=',' read -ra TASK_LIST <<< "$TASKS"
  for task in "${TASK_LIST[@]}"; do
    TASK_ARGS+=(-t "$task")
  done
fi

# Exclude Selenium-based tasks only on ARM Mac (Chromium doesn't work under QEMU)
EXCLUDE_ARGS=()
if [[ "$(uname -m)" == "arm64" ]]; then
  echo "WARNING: ARM Mac detected — skipping Selenium tasks: $SELENIUM_TASKS"
  IFS=',' read -ra SKIP_LIST <<< "$SELENIUM_TASKS"
  for task in "${SKIP_LIST[@]}"; do
    EXCLUDE_ARGS+=(-x "$task")
  done
fi

cd "$SCRIPT_DIR"
exec uv run harbor run \
  -d terminal-bench@2.0 \
  --agent-import-path gollem_harbor:GollemAgent \
  -m "$MODEL" \
  --env docker \
  -n "$CONCURRENCY" \
  "${TASK_ARGS[@]}" \
  "${EXCLUDE_ARGS[@]}" \
  "${EXTRA_ARGS[@]}"
