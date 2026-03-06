#!/usr/bin/env bash
# Quick progress checker for an in-flight Harbor run.
# Usage: ./check-progress.sh [job-dir]
#   Defaults to the latest job directory in ./jobs/

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

if [[ -n "${1:-}" ]]; then
  JOB_DIR="$1"
else
  JOB_DIR="$(ls -1dt "$SCRIPT_DIR"/jobs/*/config.json 2>/dev/null | head -1 | xargs dirname 2>/dev/null || true)"
fi

if [[ -z "$JOB_DIR" || ! -d "$JOB_DIR" ]]; then
  echo "No job directory found."
  exit 1
fi

JOB_NAME="$(basename "$JOB_DIR")"

# Let Python do all the heavy lifting — it handles grouping by task cleanly.
python3 - "$JOB_DIR" "$JOB_NAME" <<'PYEOF'
import json, sys, os
from pathlib import Path
from datetime import datetime
from collections import defaultdict

job_dir = Path(sys.argv[1])
job_name = sys.argv[2]

print(f"Job: {job_name}")
print(f"Dir: {job_dir}")
print()

# Gather all trials
tasks = defaultdict(lambda: {"pass": 0, "fail": 0, "error": 0, "running": 0,
                              "durations": [], "tokens_in": 0, "tokens_out": 0,
                              "cache": 0, "tools": 0})

for trial_dir in sorted(job_dir.iterdir()):
    if not trial_dir.is_dir():
        continue
    name = trial_dir.name
    task = name.split("__")[0]
    result_path = trial_dir / "result.json"

    if not result_path.exists():
        tasks[task]["running"] += 1
        continue

    try:
        r = json.loads(result_path.read_text())
    except Exception:
        tasks[task]["error"] += 1
        continue

    exc = r.get("exception_info")
    reward = (r.get("verifier_result") or {}).get("rewards", {}).get("reward", -1)

    if exc:
        tasks[task]["error"] += 1
    elif reward == 1.0:
        tasks[task]["pass"] += 1
    elif reward == 0.0:
        tasks[task]["fail"] += 1
    else:
        tasks[task]["error"] += 1

    # Duration
    started = r.get("started_at", "")
    finished = r.get("finished_at", "")
    if started and finished:
        try:
            s = datetime.fromisoformat(started.replace("Z", "+00:00"))
            f = datetime.fromisoformat(finished.replace("Z", "+00:00"))
            tasks[task]["durations"].append(int((f - s).total_seconds()))
        except Exception:
            pass

    ar = r.get("agent_result") or {}
    tasks[task]["tokens_in"] += ar.get("n_input_tokens") or 0
    tasks[task]["tokens_out"] += ar.get("n_output_tokens") or 0
    tasks[task]["cache"] += ar.get("n_cache_tokens") or 0
    tasks[task]["tools"] += (ar.get("metadata") or {}).get("tool_invocations_total") or 0

# Classify tasks
done_tasks = []  # all trials finished
partial_tasks = []  # some trials finished, some running
running_tasks = []  # only running trials (no results yet)

for task, info in sorted(tasks.items()):
    completed = info["pass"] + info["fail"] + info["error"]
    total = completed + info["running"]
    avg_dur = sum(info["durations"]) // max(len(info["durations"]), 1)
    entry = {
        "task": task,
        "pass": info["pass"],
        "fail": info["fail"],
        "error": info["error"],
        "running": info["running"],
        "completed": completed,
        "total": total,
        "avg_dur": avg_dur,
        "tokens_in": info["tokens_in"],
        "tokens_out": info["tokens_out"],
        "tools": info["tools"],
    }
    if info["running"] > 0 and completed == 0:
        running_tasks.append(entry)
    elif info["running"] > 0:
        partial_tasks.append(entry)
    else:
        done_tasks.append(entry)

# Totals
total_trials = sum(e["completed"] for e in done_tasks + partial_tasks + running_tasks)
total_pass = sum(e["pass"] for e in done_tasks + partial_tasks + running_tasks)
total_fail = sum(e["fail"] for e in done_tasks + partial_tasks + running_tasks)
total_error = sum(e["error"] for e in done_tasks + partial_tasks + running_tasks)
total_running = sum(e["running"] for e in done_tasks + partial_tasks + running_tasks)
n_tasks = len(tasks)
n_done = len(done_tasks)

# Tasks where every trial passed
perfect_tasks = [e for e in done_tasks if e["fail"] == 0 and e["error"] == 0 and e["pass"] > 0]
# Tasks with at least one failure
any_fail = [e for e in done_tasks + partial_tasks if e["fail"] > 0 or e["error"] > 0]

print("=== SUMMARY ===")
print(f"Tasks: {n_done} done / {n_tasks} seen  |  Trials: {total_trials} done, {total_running} running")
print(f"Trial results: {total_pass} pass, {total_fail} fail, {total_error} error")
print(f"Perfect tasks (all trials pass): {len(perfect_tasks)}/{n_done}")
if total_trials > 0:
    print(f"Trial pass rate: {total_pass*100//total_trials}%")
print()

# Running containers
try:
    import subprocess
    containers = subprocess.check_output(
        ["docker", "ps", "--format", "{{.Names}} ({{.RunningFor}})"],
        stderr=subprocess.DEVNULL, text=True
    ).strip()
except Exception:
    containers = ""

if running_tasks or partial_tasks:
    active = running_tasks + partial_tasks
    print(f"=== ACTIVE ({len(active)} tasks, {total_running} trials running) ===")
    fmt = "  {:<45s} {:>6s}"
    print(fmt.format("TASK", "STATUS"))
    for e in active:
        parts = []
        if e["pass"]: parts.append(f'{e["pass"]}P')
        if e["fail"]: parts.append(f'{e["fail"]}F')
        if e["error"]: parts.append(f'{e["error"]}E')
        if e["running"]: parts.append(f'{e["running"]}R')
        print(fmt.format(e["task"], "/".join(parts)))
    if containers:
        print()
        print("  Containers:")
        for line in containers.splitlines():
            print(f"    {line}")
    print()

if any_fail:
    print(f"=== PROBLEMS ({len(any_fail)} tasks with failures) ===")
    fmt = "  {:<45s} {:>5s} {:>5s} {:>5s} {:>8s} {:>7s}"
    print(fmt.format("TASK", "PASS", "FAIL", "ERR", "AVG DUR", "TOOLS"))
    for e in sorted(any_fail, key=lambda x: x["fail"], reverse=True):
        print(fmt.format(e["task"], str(e["pass"]), str(e["fail"]),
                         str(e["error"]), f'{e["avg_dur"]}s', str(e["tools"])))
    print()

if perfect_tasks:
    print(f"=== PERFECT ({len(perfect_tasks)} tasks, all trials pass) ===")
    fmt = "  {:<45s} {:>5s} {:>8s} {:>7s} {:>12s}"
    print(fmt.format("TASK", "PASS", "AVG DUR", "TOOLS", "TOKENS"))
    for e in sorted(perfect_tasks, key=lambda x: x["task"]):
        tok = f'{e["tokens_in"]//1000}K+{e["tokens_out"]//1000}K'
        print(fmt.format(e["task"], str(e["pass"]), f'{e["avg_dur"]}s',
                         str(e["tools"]), tok))
    print()
PYEOF
