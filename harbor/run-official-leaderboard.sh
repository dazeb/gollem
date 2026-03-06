#!/usr/bin/env bash
set -euo pipefail

# Official Terminal-Bench 2.0 leaderboard runner wrapper.
# This wraps run-eval.sh with guardrails for reproducible submission runs.
#
# Usage:
#   ./run-official-leaderboard.sh [model] [concurrency] [tasks] [attempts]
#
# Examples:
#   ./run-official-leaderboard.sh openai/gpt-5.3-codex 1
#   ./run-official-leaderboard.sh openai/gpt-5.3-codex 1 gpt2-codegolf,chess-best-move 5

MODEL="${1:-openai/gpt-5.3-codex}"
CONCURRENCY="${2:-1}"
TASKS="${3:-}"
ATTEMPTS="${4:-${TBENCH_ATTEMPTS:-5}}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODEL_PROVIDER="${MODEL%%/*}"
MODEL_NAME="${MODEL#*/}"
if [[ "$MODEL_NAME" == "$MODEL" ]]; then
  MODEL_NAME="$MODEL"
fi

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "Official runs should execute on Linux x86_64 hosts."
  exit 1
fi

if [[ "$(uname -m)" != "x86_64" ]]; then
  echo "Official runs should execute on x86_64 (avoid qemu emulation)."
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required"
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "docker daemon is not ready"
  exit 1
fi

if [[ -f "$HOME/.config/gollem-harbor.env" ]]; then
  # shellcheck disable=SC1090
  source "$HOME/.config/gollem-harbor.env"
fi

if [[ -f "$HOME/.envrc" ]]; then
  # shellcheck disable=SC1090
  source "$HOME/.envrc"
fi

# Auto-derive failed tasks from the latest Harbor result and promote them to
# xhigh reasoning on the next official attempt. You can override with:
#   GOLLEM_XHIGH_TASKS="task-a,task-b"
#   GOLLEM_REASONING_SOURCE_RESULT_JSON="/path/to/result.json"
#   GOLLEM_REASONING_BY_TASK="..." (explicit map takes precedence)
if [[ -z "${GOLLEM_XHIGH_TASKS:-}" ]]; then
  RESULT_SOURCE="${GOLLEM_REASONING_SOURCE_RESULT_JSON:-}"
  if [[ -z "$RESULT_SOURCE" ]]; then
    RESULT_SOURCE="$(ls -1t "${SCRIPT_DIR}"/jobs/*/result.json 2>/dev/null | head -n 1 || true)"
  fi
  if [[ -n "$RESULT_SOURCE" && -f "$RESULT_SOURCE" ]]; then
    GOLLEM_XHIGH_TASKS="$(RESULT_SOURCE="$RESULT_SOURCE" python3 - <<'PY'
import json
import os
from pathlib import Path

path = Path(os.environ["RESULT_SOURCE"])
try:
    data = json.loads(path.read_text())
except Exception:
    print("")
    raise SystemExit(0)

evals = data.get("stats", {}).get("evals", {})
if isinstance(evals, dict) and evals:
    eval_entry = next(iter(evals.values()))
else:
    eval_entry = {}

reward_stats = eval_entry.get("reward_stats", {}).get("reward", {})
failed = reward_stats.get("0.0", []) if isinstance(reward_stats, dict) else []
if not isinstance(failed, list):
    failed = []

seen = set()
tasks = []
for name in failed:
    if not isinstance(name, str):
        continue
    base = name.rsplit("__", 1)[0].strip()
    if base and base not in seen:
        seen.add(base)
        tasks.append(base)

print(",".join(tasks))
PY
)"
  fi
fi

# Submission-oriented defaults.
: "${GOLLEM_TEAM_MODE:=off}"
: "${GOLLEM_TASK_TIMEOUT_BUFFER_SEC:=0}"
: "${GOLLEM_SETUP_INSTALL_LSP:=1}"
: "${GOLLEM_TBENCH_COMPETITION_PROMPT:=1}"
: "${GOLLEM_MODEL_REQUEST_TIMEOUT_SEC:=600}"
: "${GOLLEM_TOP_LEVEL_PERSONALITY:=0}"
: "${GOLLEM_REQUIRE_INVARIANT_CHECKLIST:=1}"

# Parse winning-configs.txt for per-task personality and delegate settings.
# Format: "task-name xhigh personality=0|1 delegate=0|1"
WINNING_CONFIGS="${REPO_ROOT}/winning-configs.txt"
if [[ -f "$WINNING_CONFIGS" ]]; then
  eval "$(WINNING_CONFIGS="$WINNING_CONFIGS" python3 - <<'PY'
import os
from pathlib import Path

lines = Path(os.environ["WINNING_CONFIGS"]).read_text().splitlines()
personality_parts = []
delegate_parts = []
reasoning_parts = []
for line in lines:
    line = line.strip()
    if not line or line.startswith("#"):
        continue
    tokens = line.split()
    if len(tokens) < 2:
        continue
    task = tokens[0]
    # Second token is reasoning effort (e.g. "xhigh")
    reasoning_parts.append(f"{task}={tokens[1]}")
    for t in tokens[2:]:
        if t.startswith("personality="):
            personality_parts.append(f"{task}={t.split('=',1)[1]}")
        elif t.startswith("delegate="):
            delegate_parts.append(f"{task}={t.split('=',1)[1]}")

personality_parts.append("*=0")
delegate_parts.append("*=0")
reasoning_parts.append("*=xhigh")

# Only emit if not already set by caller
if not os.environ.get("GOLLEM_PERSONALITY_BY_TASK"):
    print(f'GOLLEM_PERSONALITY_BY_TASK="{",".join(personality_parts)}"')
if not os.environ.get("GOLLEM_DELEGATE_BY_TASK"):
    print(f'GOLLEM_DELEGATE_BY_TASK="{",".join(delegate_parts)}"')
if not os.environ.get("GOLLEM_REASONING_BY_TASK"):
    print(f'GOLLEM_REASONING_BY_TASK="{",".join(reasoning_parts)}"')
PY
)"
  echo "Per-task configs loaded from winning-configs.txt"
fi
: "${GOLLEM_XHIGH_TASKS:=model-extraction-relu-logits}"
if [[ -z "${GOLLEM_REASONING_BY_TASK:-}" ]]; then
  GOLLEM_REASONING_BY_TASK="$(
    GOLLEM_XHIGH_TASKS="$GOLLEM_XHIGH_TASKS" python3 - <<'PY'
import os
raw = os.environ.get("GOLLEM_XHIGH_TASKS", "")
seen = set()
tasks = []
for token in raw.replace(",", " ").split():
    t = token.strip()
    if t and t not in seen:
        seen.add(t)
        tasks.append(t)
parts = [f"{t}=xhigh" for t in tasks]
parts.append("*=high")
print(",".join(parts))
PY
  )"
fi
: "${GOLLEM_REASONING_NO_SANDWICH_BY_TASK:=${GOLLEM_XHIGH_TASKS}}"
: "${GOLLEM_REASONING_NO_GREEDY_BY_TASK:=${GOLLEM_XHIGH_TASKS}}"
: "${OPENAI_PROMPT_CACHE_KEY:=tbench2-gollem}"
: "${OPENAI_PROMPT_CACHE_RETENTION:=24h}"
: "${OPENAI_SERVICE_TIER:=priority}"
: "${OPENAI_TRANSPORT:=websocket}"
: "${OPENAI_WEBSOCKET_HTTP_FALLBACK:=1}"

export GOLLEM_TEAM_MODE
export GOLLEM_TASK_TIMEOUT_BUFFER_SEC
export GOLLEM_SETUP_INSTALL_LSP
export GOLLEM_TBENCH_COMPETITION_PROMPT
export GOLLEM_MODEL_REQUEST_TIMEOUT_SEC
export GOLLEM_TOP_LEVEL_PERSONALITY
export GOLLEM_PERSONALITY_BY_TASK
export GOLLEM_DELEGATE_BY_TASK
export GOLLEM_REQUIRE_INVARIANT_CHECKLIST
export GOLLEM_XHIGH_TASKS
export GOLLEM_REASONING_BY_TASK
export GOLLEM_REASONING_NO_SANDWICH_BY_TASK
export GOLLEM_REASONING_NO_GREEDY_BY_TASK
export OPENAI_PROMPT_CACHE_KEY
export OPENAI_PROMPT_CACHE_RETENTION
export OPENAI_SERVICE_TIER
export OPENAI_TRANSPORT
export OPENAI_WEBSOCKET_HTTP_FALLBACK

# Submission metadata defaults (override via env if needed).
: "${TBENCH_AGENT_URL:=https://github.com/fugue-labs/gollem}"
: "${TBENCH_AGENT_DISPLAY_NAME:=gollem}"
: "${TBENCH_AGENT_ORG_DISPLAY_NAME:=Fugue Labs}"
: "${TBENCH_MODEL_DISPLAY_NAME:=${MODEL_NAME}}"
case "${MODEL_PROVIDER}" in
  openai) : "${TBENCH_MODEL_ORG_DISPLAY_NAME:=OpenAI}" ;;
  anthropic) : "${TBENCH_MODEL_ORG_DISPLAY_NAME:=Anthropic}" ;;
  google|vertex|vertexai|vertexai-anthropic) : "${TBENCH_MODEL_ORG_DISPLAY_NAME:=Google}" ;;
  *) : "${TBENCH_MODEL_ORG_DISPLAY_NAME:=${MODEL_PROVIDER}}" ;;
esac

# Submission export location.
: "${TBENCH_SUBMISSION_ROOT:=${REPO_ROOT}/submissions/terminal-bench/2.0}"
: "${TBENCH_SUBMISSION_NAME:=gollem__${MODEL_NAME}}"
SUBMISSION_DIR="${TBENCH_SUBMISSION_ROOT}/${TBENCH_SUBMISSION_NAME}"

if [[ -z "${OPENAI_API_KEY:-}" && -z "${ANTHROPIC_API_KEY:-}" ]]; then
  echo "No model API key found in environment (OPENAI_API_KEY/ANTHROPIC_API_KEY)."
  exit 1
fi

if ! [[ "$ATTEMPTS" =~ ^[0-9]+$ ]] || [[ "$ATTEMPTS" -lt 1 ]]; then
  echo "ATTEMPTS must be a positive integer (got: $ATTEMPTS)."
  exit 1
fi

if [[ -n "$TASKS" && "${ALLOW_PARTIAL_SUBMISSION:-0}" != "1" ]]; then
  echo "TASKS is set, but official leaderboard submissions should run the full dataset."
  echo "Unset TASKS for full runs, or set ALLOW_PARTIAL_SUBMISSION=1 for local experiments."
  exit 1
fi

if [[ "$ATTEMPTS" -lt 5 && "${ALLOW_NONCOMPLIANT_SUBMISSION:-0}" != "1" ]]; then
  echo "ATTEMPTS=$ATTEMPTS is not leaderboard-compliant (minimum is 5 trials/task)."
  echo "Use ATTEMPTS>=5, or set ALLOW_NONCOMPLIANT_SUBMISSION=1 for local experiments."
  exit 1
fi

cd "$REPO_ROOT"
if ! git diff --quiet || ! git diff --cached --quiet; then
  if [[ "${ALLOW_DIRTY_WORKTREE:-0}" != "1" ]]; then
    echo "Git worktree is dirty. For official runs, use a clean commit."
    echo "Set ALLOW_DIRTY_WORKTREE=1 to bypass."
    exit 1
  fi
fi

cd "$SCRIPT_DIR"
mkdir -p official-runs
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_LOG="official-runs/${STAMP}.log"
META_FILE="official-runs/${STAMP}.meta.txt"

{
  echo "timestamp_utc=${STAMP}"
  echo "model=${MODEL}"
  echo "concurrency=${CONCURRENCY}"
  echo "tasks=${TASKS:-all}"
  echo "attempts=${ATTEMPTS}"
  echo "git_commit=$(git -C "$REPO_ROOT" rev-parse HEAD)"
  echo "git_branch=$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
  echo "host_arch=$(uname -m)"
  echo "host_kernel=$(uname -sr)"
  echo "docker_server=$(docker version --format '{{.Server.Version}}' 2>/dev/null || true)"
  echo "prompt_cache_key=${OPENAI_PROMPT_CACHE_KEY}"
  echo "prompt_cache_retention=${OPENAI_PROMPT_CACHE_RETENTION}"
  echo "openai_service_tier=${OPENAI_SERVICE_TIER}"
  echo "openai_transport=${OPENAI_TRANSPORT}"
  echo "openai_websocket_http_fallback=${OPENAI_WEBSOCKET_HTTP_FALLBACK}"
  echo "top_level_personality=${GOLLEM_TOP_LEVEL_PERSONALITY}"
  echo "personality_by_task=${GOLLEM_PERSONALITY_BY_TASK:-}"
  echo "delegate_by_task=${GOLLEM_DELEGATE_BY_TASK:-}"
  echo "xhigh_tasks=${GOLLEM_XHIGH_TASKS}"
  echo "reasoning_by_task=${GOLLEM_REASONING_BY_TASK}"
  echo "reasoning_no_sandwich_by_task=${GOLLEM_REASONING_NO_SANDWICH_BY_TASK}"
  echo "reasoning_no_greedy_by_task=${GOLLEM_REASONING_NO_GREEDY_BY_TASK}"
  echo "submission_dir=${SUBMISSION_DIR}"
  echo "agent_url=${TBENCH_AGENT_URL}"
  echo "agent_display_name=${TBENCH_AGENT_DISPLAY_NAME}"
  echo "agent_org_display_name=${TBENCH_AGENT_ORG_DISPLAY_NAME}"
  echo "model_provider=${MODEL_PROVIDER}"
  echo "model_name=${MODEL_NAME}"
  echo "model_display_name=${TBENCH_MODEL_DISPLAY_NAME}"
  echo "model_org_display_name=${TBENCH_MODEL_ORG_DISPLAY_NAME}"
} > "$META_FILE"

echo "Starting official run..."
echo "metadata: $META_FILE"
echo "log:      $RUN_LOG"

set -o pipefail
if [[ -n "$TASKS" ]]; then
  ./run-eval.sh "$MODEL" "$CONCURRENCY" "$TASKS" "$ATTEMPTS" 2>&1 | tee "$RUN_LOG"
else
  ./run-eval.sh "$MODEL" "$CONCURRENCY" "" "$ATTEMPTS" 2>&1 | tee "$RUN_LOG"
fi
set +o pipefail

RESULT_PATH="$(grep -Eo 'jobs/[^ ]+/result.json' "$RUN_LOG" | tail -n 1 || true)"
if [[ -n "$RESULT_PATH" && -f "$RESULT_PATH" ]]; then
  RESULT_ABS="$(cd "$SCRIPT_DIR" && realpath "$RESULT_PATH")"
  echo "result_json=${RESULT_ABS}" >> "$META_FILE"
  echo "Result: $RESULT_ABS"

  JOB_DIR_ABS="$(dirname "$RESULT_ABS")"
  JOB_NAME="$(basename "$JOB_DIR_ABS")"
  mkdir -p "$SUBMISSION_DIR"

  # Write submission metadata.yaml expected by TB2 leaderboard validator.
  # shellcheck disable=SC2016
  yaml_q() { printf "'%s'" "${1//\'/\'\'}"; }
  cat > "${SUBMISSION_DIR}/metadata.yaml" <<EOF
agent_url: $(yaml_q "${TBENCH_AGENT_URL}")
agent_display_name: $(yaml_q "${TBENCH_AGENT_DISPLAY_NAME}")
agent_org_display_name: $(yaml_q "${TBENCH_AGENT_ORG_DISPLAY_NAME}")

models:
  - model_name: $(yaml_q "${MODEL_NAME}")
    model_provider: $(yaml_q "${MODEL_PROVIDER}")
    model_display_name: $(yaml_q "${TBENCH_MODEL_DISPLAY_NAME}")
    model_org_display_name: $(yaml_q "${TBENCH_MODEL_ORG_DISPLAY_NAME}")
EOF

  # Copy full Harbor job folder into submission dir for PR packaging.
  rm -rf "${SUBMISSION_DIR}/${JOB_NAME}"
  cp -R "${JOB_DIR_ABS}" "${SUBMISSION_DIR}/"
  echo "submission_dir=${SUBMISSION_DIR}" >> "$META_FILE"
  echo "submission_job_dir=${SUBMISSION_DIR}/${JOB_NAME}" >> "$META_FILE"
  echo "Submission artifacts copied to: ${SUBMISSION_DIR}/${JOB_NAME}"

  # Optional strict validation gate (off by default for ad-hoc runs).
  if [[ "${TBENCH_VALIDATE_SUBMISSION:-0}" == "1" ]]; then
    "${REPO_ROOT}/contrib/tbench_validate_submission.sh" "${SUBMISSION_DIR}"
  fi
else
  echo "Could not auto-detect result.json path from log."
fi

# Tag Langfuse traces with Harbor verification rewards.
if [[ -n "${LANGFUSE_SECRET_KEY:-}" && -n "${JOB_DIR_ABS:-}" ]]; then
  "${SCRIPT_DIR}/langfuse-tag-scores.sh" "${JOB_DIR_ABS}" || true
fi

echo "Official run wrapper complete."
