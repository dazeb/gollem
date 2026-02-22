#!/bin/bash
# Terminal-Bench 2.0 evaluation runner for gollem
# Usage: ./run-eval.sh [model] [concurrency]
#   model: provider/model (default: openai/grok-4-1-fast-reasoning)
#   concurrency: number of parallel tasks (default: 2)

MODEL="${1:-openai/grok-4-0709}"
CONCURRENCY="${2:-2}"

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
  *)
    echo "Provider: $PROVIDER"
    ;;
esac

echo "Model: $MODEL | Concurrency: $CONCURRENCY"
cd /Users/trevor/gt/gollem/mayor/rig/harbor
exec uv run harbor run \
  -d terminal-bench@2.0 \
  --agent-import-path gollem_harbor:GollemAgent \
  -m "$MODEL" \
  --env docker \
  -n "$CONCURRENCY"
