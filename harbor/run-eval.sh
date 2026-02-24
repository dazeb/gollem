#!/bin/bash
# Terminal-Bench 2.0 evaluation runner for gollem
# Usage: ./run-eval.sh [model] [concurrency] [tasks]
#   model: provider/model (default: openai/gpt-5.2-codex)
#   concurrency: number of parallel tasks (default: 2)
#   tasks: comma-separated task names (default: all non-excluded tasks)
#
# Examples:
#   ./run-eval.sh                                        # all tasks, gpt-5.2-codex
#   ./run-eval.sh openai/gpt-5.2-codex 4                 # 4 concurrent
#   ./run-eval.sh anthropic/claude-sonnet-4-20250514 2   # claude sonnet
#   ./run-eval.sh google/gemini-3-flash-preview 1 gpt2-codegolf,chess-best-move

MODEL="${1:-openai/gpt-5.2-codex}"
CONCURRENCY="${2:-2}"
TASKS="${3:-}"  # Optional: comma-separated task names, or "all" for everything

# Tasks that require Selenium/Chromium (won't work under QEMU on ARM Mac)
SELENIUM_TASKS="break-filter-js-from-html,filter-js-from-html"

# Load API keys from ~/.envrc (provider-specific keys)
if [[ -f ~/.envrc ]]; then
  source ~/.envrc
fi
if [[ -f ~/.anthropic-key ]]; then
  export ANTHROPIC_API_KEY=$(sed -n 's/^ANTHROPIC_API_KEY=//p' ~/.anthropic-key | tr -d '[:space:]')
fi

# Auto-detect xAI models and set base URL (only for non-OpenAI providers)
PROVIDER="${MODEL%%/*}"
if [[ "$PROVIDER" != "openai" ]] && [[ "$OPENAI_API_KEY" == xai-* ]] && [[ -z "$OPENAI_BASE_URL" ]]; then
  export OPENAI_BASE_URL="https://api.x.ai"
fi

# Verify we have the right API key for the provider
case "$PROVIDER" in
  anthropic)
    echo "Provider: anthropic | Key: ${ANTHROPIC_API_KEY:+${#ANTHROPIC_API_KEY} chars}"
    if [[ -z "$ANTHROPIC_API_KEY" ]]; then
      echo "ERROR: ANTHROPIC_API_KEY is required for anthropic models."
      exit 1
    fi
    ;;
  openai)
    # Ensure we're hitting real OpenAI, not xAI
    unset OPENAI_BASE_URL
    echo "Provider: openai | Key: ${OPENAI_API_KEY:+${#OPENAI_API_KEY} chars}"
    if [[ -z "$OPENAI_API_KEY" ]]; then
      echo "ERROR: OPENAI_API_KEY is required for openai models."
      exit 1
    fi
    # OpenAI prompt caching tuning (applies only to OpenAI models).
    # Keep stable defaults across runs; allow caller overrides.
    : "${OPENAI_PROMPT_CACHE_KEY:=tbench2-gollem}"
    : "${OPENAI_PROMPT_CACHE_RETENTION:=in_memory}"
    export OPENAI_PROMPT_CACHE_KEY OPENAI_PROMPT_CACHE_RETENTION
    echo "OpenAI prompt cache: key=${OPENAI_PROMPT_CACHE_KEY} retention=${OPENAI_PROMPT_CACHE_RETENTION}"
    ;;
  google|vertexai|vertex)
    echo "Provider: vertexai | Project: ${GOOGLE_CLOUD_PROJECT:-not set}"
    if [[ -z "$GOOGLE_CLOUD_PROJECT" ]]; then
      echo "ERROR: GOOGLE_CLOUD_PROJECT is required for Vertex AI models."
      exit 1
    fi
    ADC_FILE="${GOOGLE_APPLICATION_CREDENTIALS:-$HOME/.config/gcloud/application_default_credentials.json}"
    if [[ ! -f "$ADC_FILE" ]]; then
      echo "ERROR: GCP ADC credentials file not found: $ADC_FILE"
      echo "Run: gcloud auth application-default login"
      exit 1
    fi
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
