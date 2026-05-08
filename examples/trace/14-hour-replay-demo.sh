#!/usr/bin/env bash
set -euo pipefail

# Deterministic local version of the Gollem Trace "14-Hour Replay" demo.
# It uses the test provider so the flow is recordable without external model
# credentials, while still exercising trace streaming, export, replay, fork,
# planner swap, live re-exec, evaluator evidence, Sleepy evidence, and diff.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${GOLLEM_TRACE_DEMO_OUT:-$(mktemp -d "${TMPDIR:-/tmp}/gollem-trace-demo.XXXXXX")}"
BIN="$OUT/gollem"
GO_BIN="${GO:-go}"

mkdir -p "$OUT"

echo "demo output: $OUT"
echo "building gollem..."
(cd "$ROOT" && "$GO_BIN" build -o "$BIN" ./cmd/gollem)

BASE_TRACE="$OUT/base.trace.json"
BASE_STREAM="$OUT/base.trace.jsonl"
EXPORTED_TRACE="$OUT/exported.trace.json"
CRASH_SNAPSHOT="$OUT/crash-resume.snapshot.json"
RESUMED_TRACE="$OUT/resumed.trace.json"
FORK_TRACE="$OUT/fork.trace.json"
LIVE_REEXEC_TRACE="$OUT/live-reexec.trace.json"
EVALUATED_TRACE="$OUT/evaluated.trace.json"
SLEEPY_EVIDENCE="$OUT/sleepy-evidence.json"

echo "1. start long-run stand-in with live trace stream"
"$BIN" run --provider test --no-code-mode --trace-out "$BASE_TRACE" --trace-stream "$BASE_STREAM" "14-hour replay demo baseline"
grep -q '"kind":"run.started"' "$BASE_STREAM"
grep -q '"kind":"run.completed"' "$BASE_STREAM"

RUN_ID="$("$BIN" trace inspect "$BASE_TRACE" | awk 'NR==1 {print $2}')"

echo "2. export by local run id"
"$BIN" trace export "$RUN_ID" --trace-dir "$(dirname "$BASE_TRACE")" --out "$EXPORTED_TRACE"
"$BIN" trace validate "$EXPORTED_TRACE"

echo "3. inspect and strict replay"
"$BIN" trace inspect "$EXPORTED_TRACE" --events 20
"$BIN" trace replay "$EXPORTED_TRACE" --mode strict

echo "4. simulate crash/resume from a stable boundary snapshot"
"$BIN" trace fork "$EXPORTED_TRACE" \
  --from-kind model.responded \
  --planner-prompt "planner prompt after crash resume" \
  --append-user "continue after simulated crash" \
  --out "$CRASH_SNAPSHOT"
"$BIN" run --provider test --no-code-mode --resume-snapshot "$CRASH_SNAPSHOT" --trace-out "$RESUMED_TRACE" "continue"
"$BIN" trace validate "$RESUMED_TRACE"

echo "5. one-command fork with planner swap"
"$BIN" trace fork "$EXPORTED_TRACE" \
  --from-kind model.responded \
  --planner-prompt "planner prompt fork variant" \
  --append-user "try a branch with a different plan" \
  --continue \
  --provider test \
  --run-arg --no-code-mode \
  --out "$FORK_TRACE"
"$BIN" trace validate "$FORK_TRACE"

echo "6. live re-exec from replay boundary"
"$BIN" trace replay "$EXPORTED_TRACE" \
  --mode live-reexec \
  --from-kind model.responded \
  --planner-prompt "planner prompt live re-exec variant" \
  --append-user "continue from live re-exec" \
  --provider test \
  --run-arg --no-code-mode \
  --out "$LIVE_REEXEC_TRACE"
"$BIN" trace validate "$LIVE_REEXEC_TRACE"

echo "7. evaluator and Sleepy evidence"
"$BIN" trace evaluate "$FORK_TRACE" --evaluator status-succeeded --out "$EVALUATED_TRACE"
"$BIN" trace sleepy "$EXPORTED_TRACE" "$FORK_TRACE" "$LIVE_REEXEC_TRACE" --out "$SLEEPY_EVIDENCE"

echo "8. causal and quantitative diff"
"$BIN" trace diff "$EXPORTED_TRACE" "$FORK_TRACE"
"$BIN" trace regress "$EXPORTED_TRACE" "$FORK_TRACE" --require-status succeeded

if [[ "${GOLLEM_TRACE_DEMO_VIEW:-0}" == "1" ]]; then
  "$BIN" trace view "$EXPORTED_TRACE" "$FORK_TRACE"
else
  echo "set GOLLEM_TRACE_DEMO_VIEW=1 to open the TUI comparison viewer"
fi

echo "demo complete: $OUT"
