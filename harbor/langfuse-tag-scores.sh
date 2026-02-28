#!/usr/bin/env bash
# langfuse-tag-scores.sh — Attach Harbor verification rewards to Langfuse traces.
#
# Usage:
#   ./langfuse-tag-scores.sh <job-dir>
#
# Iterates trial directories in a Harbor job, reads the Langfuse trace ID from
# trajectory.json and the reward from result.json, then POSTs a score to the
# Langfuse API. Idempotent: uses a deterministic score ID derived from the
# trace ID so re-runs update rather than duplicate.
#
# Requires: LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, LANGFUSE_BASE_URL

set -euo pipefail

JOB_DIR="${1:?usage: langfuse-tag-scores.sh <job-dir>}"

if [[ -z "${LANGFUSE_PUBLIC_KEY:-}" || -z "${LANGFUSE_SECRET_KEY:-}" ]]; then
    echo "langfuse-tag-scores: skipping (LANGFUSE_PUBLIC_KEY or LANGFUSE_SECRET_KEY not set)" >&2
    exit 0
fi

LANGFUSE_BASE_URL="${LANGFUSE_BASE_URL:-https://cloud.langfuse.com}"

tagged=0
skipped=0
failed=0

for trial_dir in "$JOB_DIR"/*/; do
    trial_name="$(basename "$trial_dir")"

    # Skip non-trial entries (config.json, result.json, job.log).
    [[ -d "$trial_dir/agent" ]] || continue

    # Read Langfuse trace ID from trajectory.
    traj_file="$trial_dir/agent/trajectory.json"
    if [[ ! -f "$traj_file" ]]; then
        skipped=$((skipped + 1))
        continue
    fi
    trace_id="$(python3 -c "import json; d=json.load(open('$traj_file')); print(d.get('langfuse_trace_id',''))" 2>/dev/null)"
    if [[ -z "$trace_id" ]]; then
        echo "langfuse-tag-scores: $trial_name — no trace_id in trajectory.json" >&2
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
        echo "langfuse-tag-scores: $trial_name — no reward in result.json" >&2
        skipped=$((skipped + 1))
        continue
    }

    # Deterministic score ID so re-runs are idempotent (update, not duplicate).
    score_id="$(echo -n "harbor_reward:${trace_id}" | sha256sum | cut -c1-32)"
    score_id="${score_id:0:8}-${score_id:8:4}-${score_id:12:4}-${score_id:16:4}-${score_id:20:12}"

    # Extract task name from trial directory name (format: <task>__<random>).
    task_name="${trial_name%__*}"

    http_code="$(curl -s -o /dev/null -w '%{http_code}' \
        -X POST "${LANGFUSE_BASE_URL}/api/public/scores" \
        -u "${LANGFUSE_PUBLIC_KEY}:${LANGFUSE_SECRET_KEY}" \
        -H "Content-Type: application/json" \
        -d "$(python3 -c "
import json
print(json.dumps({
    'id': '$score_id',
    'traceId': '$trace_id',
    'name': 'harbor_reward',
    'value': $reward,
    'dataType': 'NUMERIC',
    'comment': 'task=$task_name reward=$reward',
}))
")")"

    if [[ "$http_code" == "200" || "$http_code" == "201" ]]; then
        echo "langfuse-tag-scores: $trial_name — reward=$reward trace=$trace_id" >&2
        tagged=$((tagged + 1))
    else
        echo "langfuse-tag-scores: $trial_name — FAILED (HTTP $http_code)" >&2
        failed=$((failed + 1))
    fi
done

echo "langfuse-tag-scores: done (tagged=$tagged skipped=$skipped failed=$failed)" >&2
