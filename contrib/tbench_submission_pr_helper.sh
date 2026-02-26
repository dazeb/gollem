#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  contrib/tbench_submission_pr_helper.sh [submission_dir] [--execute]

Examples:
  contrib/tbench_submission_pr_helper.sh submissions/terminal-bench/2.0/gollem__gpt-5.3-codex
  contrib/tbench_submission_pr_helper.sh --execute

Behavior:
1) Resolves a submission directory (argument, or latest harbor official-run metadata).
2) Runs local TB2 validator.
3) Prints exact git add/commit/push and PR commands.
4) With --execute, creates a branch, commits the submission dir, and pushes.
EOF
}

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

EXECUTE=0
SUBMISSION_DIR=""

for arg in "$@"; do
  case "$arg" in
    -h|--help)
      usage
      exit 0
      ;;
    --execute)
      EXECUTE=1
      ;;
    *)
      if [[ -z "$SUBMISSION_DIR" ]]; then
        SUBMISSION_DIR="$arg"
      else
        echo "Unexpected argument: $arg"
        usage
        exit 2
      fi
      ;;
  esac
done

if [[ -z "$SUBMISSION_DIR" ]]; then
  latest_meta="$(ls -1t "$REPO_ROOT"/harbor/official-runs/*.meta.txt 2>/dev/null | head -n 1 || true)"
  if [[ -n "$latest_meta" && -f "$latest_meta" ]]; then
    SUBMISSION_DIR="$(sed -n 's/^submission_dir=//p' "$latest_meta" | head -n 1)"
  fi
fi

if [[ -z "$SUBMISSION_DIR" ]]; then
  echo "Could not infer submission_dir. Pass it explicitly."
  exit 2
fi

if [[ "${SUBMISSION_DIR#/}" == "$SUBMISSION_DIR" ]]; then
  SUBMISSION_DIR="$REPO_ROOT/$SUBMISSION_DIR"
fi

if [[ ! -d "$SUBMISSION_DIR" ]]; then
  echo "Submission directory not found: $SUBMISSION_DIR"
  exit 2
fi

if [[ ! -f "$SUBMISSION_DIR/metadata.yaml" ]]; then
  echo "Missing metadata.yaml in $SUBMISSION_DIR"
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required"
  exit 2
fi

VALIDATOR="$REPO_ROOT/contrib/tbench_validate_submission.sh"
if [[ ! -x "$VALIDATOR" ]]; then
  chmod +x "$VALIDATOR"
fi

echo "Validating submission: $SUBMISSION_DIR"
"$VALIDATOR" "$SUBMISSION_DIR"

submission_name="$(basename "$SUBMISSION_DIR")"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
branch_default="tbench2-submission-${submission_name}-${timestamp}"
commit_default="TB2 submission: ${submission_name}"

tmp_trials="$(mktemp)"
trap 'rm -f "$tmp_trials"' EXIT

while IFS= read -r -d '' file; do
  if jq -e '.task_name? != null' "$file" >/dev/null 2>&1; then
    printf '%s\n' "$file" >> "$tmp_trials"
  fi
done < <(find "$SUBMISSION_DIR" -type f -name result.json -print0)

trial_count="$(wc -l < "$tmp_trials" | tr -d '[:space:]')"
if [[ "$trial_count" == "0" ]]; then
  echo "No trial-level result.json files found in $SUBMISSION_DIR"
  exit 2
fi

task_count="$(xargs -I{} jq -r '.task_name' "{}" < "$tmp_trials" | sort -u | wc -l | tr -d '[:space:]')"
jobs_count="$(find "$SUBMISSION_DIR" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d '[:space:]')"

read -r agent_display <<<"$(sed -n 's/^agent_display_name:[[:space:]]*//p' "$SUBMISSION_DIR/metadata.yaml" | head -n 1 | sed "s/^'//;s/'$//")"
read -r agent_org <<<"$(sed -n 's/^agent_org_display_name:[[:space:]]*//p' "$SUBMISSION_DIR/metadata.yaml" | head -n 1 | sed "s/^'//;s/'$//")"

pr_body="$SUBMISSION_DIR/PR_BODY_${timestamp}.md"
cat > "$pr_body" <<EOF
## Terminal-Bench 2.0 Submission

- Agent: ${agent_display:-unknown} (${agent_org:-unknown})
- Submission dir: \`$(realpath --relative-to="$REPO_ROOT" "$SUBMISSION_DIR" 2>/dev/null || echo "$SUBMISSION_DIR")\`
- Jobs included: ${jobs_count}
- Trial results: ${trial_count}
- Distinct tasks: ${task_count}

## Validation

- [x] Ran: \`contrib/tbench_validate_submission.sh $(realpath --relative-to="$REPO_ROOT" "$SUBMISSION_DIR" 2>/dev/null || echo "$SUBMISSION_DIR")\`
- [x] Metadata present (\`metadata.yaml\`)
- [x] No timeout/resource overrides in submitted job configs

## Notes

- Generated via \`contrib/tbench_submission_pr_helper.sh\`.
EOF

echo
echo "Submission summary"
echo "  submission_dir: $SUBMISSION_DIR"
echo "  jobs:           $jobs_count"
echo "  trials:         $trial_count"
echo "  tasks:          $task_count"
echo "  pr_body:        $pr_body"
echo

echo "Suggested commands"
echo "  cd $REPO_ROOT"
echo "  git checkout -b $branch_default"
echo "  git add $(realpath --relative-to="$REPO_ROOT" "$SUBMISSION_DIR" 2>/dev/null || echo "$SUBMISSION_DIR")"
echo "  git commit -m \"$commit_default\""
echo "  git push -u origin $branch_default"
echo "  gh pr create --title \"$commit_default\" --body-file \"$pr_body\""
echo

if [[ "$EXECUTE" == "1" ]]; then
  cd "$REPO_ROOT"
  rel_submission="$(realpath --relative-to="$REPO_ROOT" "$SUBMISSION_DIR" 2>/dev/null || echo "$SUBMISSION_DIR")"

  if ! git diff --quiet || ! git diff --cached --quiet; then
    echo "Worktree is dirty. Refusing --execute. Commit/stash existing changes first."
    exit 1
  fi

  git checkout -b "$branch_default"
  git add "$rel_submission"
  git commit -m "$commit_default"
  git push -u origin "$branch_default"

  echo
  echo "Branch pushed: $branch_default"
  echo "Open PR with:"
  echo "  gh pr create --title \"$commit_default\" --body-file \"$pr_body\""
fi
