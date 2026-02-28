#!/usr/bin/env bash
# langsmith-tag-scores.sh — Attach Harbor verification rewards to LangSmith traces.
#
# Usage:
#   ./langsmith-tag-scores.sh <job-dir>
#
# Iterates trial directories in a Harbor job, reads the LangSmith trace ID from
# trajectory.json and the reward from result.json, then POSTs feedback to the
# LangSmith API. Idempotent: uses a deterministic feedback ID derived from the
# trace ID so re-runs update rather than duplicate.
#
# Requires: LANGSMITH_API_KEY
# Optional: LANGSMITH_ENDPOINT (defaults to https://api.smith.langchain.com)

set -euo pipefail

JOB_DIR="${1:?usage: langsmith-tag-scores.sh <job-dir>}"

if [[ -z "${LANGSMITH_API_KEY:-}" ]]; then
    echo "langsmith-tag-scores: skipping (LANGSMITH_API_KEY not set)" >&2
    exit 0
fi

LANGSMITH_ENDPOINT="${LANGSMITH_ENDPOINT:-https://api.smith.langchain.com}"

tagged=0
skipped=0
failed=0

for trial_dir in "$JOB_DIR"/*/; do
    trial_name="$(basename "$trial_dir")"

    # Skip non-trial entries (config.json, result.json, job.log).
    [[ -d "$trial_dir/agent" ]] || continue

    # Read LangSmith trace ID from trajectory.
    traj_file="$trial_dir/agent/trajectory.json"
    if [[ ! -f "$traj_file" ]]; then
        skipped=$((skipped + 1))
        continue
    fi
    trace_id="$(python3 -c "import json; d=json.load(open('$traj_file')); print(d.get('langsmith_trace_id',''))" 2>/dev/null)"
    if [[ -z "$trace_id" ]]; then
        echo "langsmith-tag-scores: $trial_name — no trace_id in trajectory.json" >&2
        skipped=$((skipped + 1))
        continue
    fi

    # Read reward from trial result.
    result_file="$trial_dir/result.json"
    if [[ ! -f "$result_file" ]]; then
        skipped=$((skipped + 1))
        continue
    fi
    reward="$(python3 -c "
import json, sys
d = json.load(open('$result_file'))
vr = d.get('verifier_result') or {}
rewards = vr.get('rewards') or {}
r = rewards.get('reward')
if r is None:
    sys.exit(1)
print(r)
" 2>/dev/null)" || {
        echo "langsmith-tag-scores: $trial_name — no reward in result.json" >&2
        skipped=$((skipped + 1))
        continue
    }

    # Deterministic feedback ID so re-runs are idempotent (update, not duplicate).
    feedback_id="$(echo -n "harbor_reward:${trace_id}" | sha256sum | cut -c1-32)"
    feedback_id="${feedback_id:0:8}-${feedback_id:8:4}-${feedback_id:12:4}-${feedback_id:16:4}-${feedback_id:20:12}"

    # Extract task name from trial directory name (format: <task>__<random>).
    task_name="${trial_name%__*}"

    http_code="$(curl -s -o /dev/null -w '%{http_code}' \
        -X POST "${LANGSMITH_ENDPOINT}/api/v1/feedback" \
        -H "x-api-key: ${LANGSMITH_API_KEY}" \
        -H "Content-Type: application/json" \
        -d "$(python3 -c "
import json
print(json.dumps({
    'id': '$feedback_id',
    'run_id': '$trace_id',
    'key': 'harbor_reward',
    'score': $reward,
    'comment': 'task=$task_name reward=$reward',
}))
")")"

    if [[ "$http_code" == "200" || "$http_code" == "201" ]]; then
        echo "langsmith-tag-scores: $trial_name — reward=$reward trace=$trace_id" >&2
        tagged=$((tagged + 1))
    else
        echo "langsmith-tag-scores: $trial_name — FAILED (HTTP $http_code)" >&2
        failed=$((failed + 1))
    fi
done

echo "langsmith-tag-scores: done (tagged=$tagged skipped=$skipped failed=$failed)" >&2
