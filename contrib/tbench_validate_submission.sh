#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  contrib/tbench_validate_submission.sh <submission_dir_or_tb2_root>

Examples:
  contrib/tbench_validate_submission.sh submissions/terminal-bench/2.0/gollem__gpt-5
  contrib/tbench_validate_submission.sh submissions/terminal-bench/2.0

The path may point to:
1) A single submission directory containing metadata.yaml, or
2) A TB2 root directory containing one or more submission directories.
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "ERROR: jq is required but not found in PATH."
  exit 2
fi

if ! command -v rg >/dev/null 2>&1; then
  echo "ERROR: rg (ripgrep) is required but not found in PATH."
  exit 2
fi

INPUT_DIR="$1"
if [[ ! -d "$INPUT_DIR" ]]; then
  echo "ERROR: directory does not exist: $INPUT_DIR"
  exit 2
fi

declare -a SUBMISSION_DIRS=()
if [[ -f "$INPUT_DIR/metadata.yaml" ]]; then
  SUBMISSION_DIRS+=("$INPUT_DIR")
else
  while IFS= read -r sub; do
    SUBMISSION_DIRS+=("$sub")
  done < <(find "$INPUT_DIR" -mindepth 1 -maxdepth 1 -type d | sort)
fi

if [[ ${#SUBMISSION_DIRS[@]} -eq 0 ]]; then
  echo "ERROR: no submission directories found under: $INPUT_DIR"
  exit 2
fi

declare -a ERRORS=()
declare -a WARNINGS=()
validated_count=0

add_error() {
  ERRORS+=("$1")
}

add_warning() {
  WARNINGS+=("$1")
}

validate_metadata() {
  local submission_dir="$1"
  local metadata_file="$submission_dir/metadata.yaml"

  if [[ ! -f "$metadata_file" ]]; then
    add_error "$submission_dir: missing metadata.yaml"
    return
  fi

  local -a required_scalar_keys=(
    "agent_url"
    "agent_display_name"
    "agent_org_display_name"
  )
  local key
  for key in "${required_scalar_keys[@]}"; do
    if ! rg -q "^[[:space:]]*${key}:[[:space:]]*.+$" "$metadata_file"; then
      add_error "$metadata_file: missing or empty '$key'"
    fi
  done

  if ! rg -q "^[[:space:]]*models:[[:space:]]*$" "$metadata_file"; then
    add_error "$metadata_file: missing 'models:' section"
  fi
  if ! rg -q "^[[:space:]]*-[[:space:]]*model_name:[[:space:]]*.+$" "$metadata_file"; then
    add_error "$metadata_file: missing at least one '- model_name: ...' entry"
  fi
  if ! rg -q "^[[:space:]]*model_provider:[[:space:]]*.+$" "$metadata_file"; then
    add_error "$metadata_file: missing at least one 'model_provider: ...' entry"
  fi
}

validate_json_file() {
  local file="$1"
  if [[ ! -f "$file" ]]; then
    add_error "missing file: $file"
    return 1
  fi
  if ! jq -e . "$file" >/dev/null 2>&1; then
    add_error "invalid JSON: $file"
    return 1
  fi
  return 0
}

validate_no_overrides() {
  local job_config="$1"

  if ! jq -e '(.timeout_multiplier == 1 or .timeout_multiplier == 1.0)' "$job_config" >/dev/null; then
    add_error "$job_config: timeout_multiplier must be 1.0"
  fi
  if ! jq -e '.verifier.override_timeout_sec == null and .verifier.max_timeout_sec == null' "$job_config" >/dev/null; then
    add_error "$job_config: verifier timeout overrides must be null"
  fi
  if ! jq -e '.environment.override_cpus == null and .environment.override_memory_mb == null and .environment.override_storage_mb == null' "$job_config" >/dev/null; then
    add_error "$job_config: environment CPU/memory/storage overrides must be null"
  fi
  if ! jq -e '[.agents[]? | (.override_timeout_sec == null and .override_setup_timeout_sec == null and .max_timeout_sec == null)] | all' "$job_config" >/dev/null; then
    add_error "$job_config: agent timeout/setup overrides must be null"
  fi
}

validate_submission_dir() {
  local submission_dir="$1"
  local submission_name
  submission_name="$(basename "$submission_dir")"

  if [[ ! -f "$submission_dir/metadata.yaml" ]]; then
    add_warning "$submission_dir: skipping because metadata.yaml is missing"
    return
  fi

  validate_metadata "$submission_dir"
  validated_count=$((validated_count + 1))

  local -a job_dirs=()
  while IFS= read -r job_dir; do
    job_dirs+=("$job_dir")
  done < <(find "$submission_dir" -mindepth 1 -maxdepth 1 -type d ! -name '.*' | sort)

  if [[ ${#job_dirs[@]} -eq 0 ]]; then
    add_error "$submission_dir: no job directories found"
    return
  fi

  declare -A task_counts=()
  local total_trials=0

  local job_dir job_name job_config job_result job_id
  for job_dir in "${job_dirs[@]}"; do
    job_name="$(basename "$job_dir")"
    job_config="$job_dir/config.json"
    job_result="$job_dir/result.json"

    validate_json_file "$job_config" || true
    validate_json_file "$job_result" || true
    if [[ ! -f "$job_config" || ! -f "$job_result" ]]; then
      continue
    fi

    validate_no_overrides "$job_config"
    job_id="$(jq -r '.id // empty' "$job_result")"
    if [[ -z "$job_id" ]]; then
      add_error "$job_result: missing top-level 'id'"
    fi

    local -a trial_results=()
    while IFS= read -r file; do
      trial_results+=("$file")
    done < <(find "$job_dir" -mindepth 2 -maxdepth 2 -type f -name result.json | sort)

    if [[ ${#trial_results[@]} -eq 0 ]]; then
      add_error "$job_dir: no trial result.json files found"
      continue
    fi

    local trial_result trial_dir trial_job_id task_name extra_file_count
    for trial_result in "${trial_results[@]}"; do
      validate_json_file "$trial_result" || continue

      trial_dir="$(dirname "$trial_result")"
      trial_job_id="$(jq -r '.config.job_id // empty' "$trial_result")"
      if [[ -z "$trial_job_id" ]]; then
        add_error "$trial_result: missing config.job_id"
      elif [[ -n "$job_id" && "$trial_job_id" != "$job_id" ]]; then
        add_error "$trial_result: config.job_id '$trial_job_id' does not match job id '$job_id' in $job_result"
      fi

      task_name="$(jq -r '.task_name // empty' "$trial_result")"
      if [[ -z "$task_name" ]]; then
        add_error "$trial_result: missing task_name"
      else
        task_counts["$task_name"]=$(( ${task_counts["$task_name"]:-0} + 1 ))
      fi

      extra_file_count="$(find "$trial_dir" -type f ! -name result.json | wc -l | tr -d '[:space:]')"
      if [[ "$extra_file_count" -eq 0 ]]; then
        add_error "$trial_dir: contains only result.json; expected additional run artifacts"
      fi

      total_trials=$((total_trials + 1))
    done

    local -a trial_configs=()
    while IFS= read -r cfg; do
      trial_configs+=("$cfg")
    done < <(find "$job_dir" -mindepth 2 -maxdepth 2 -type f -name config.json | sort)

    local cfg trial_from_cfg
    for cfg in "${trial_configs[@]}"; do
      trial_from_cfg="$(dirname "$cfg")"
      if [[ ! -f "$trial_from_cfg/result.json" ]]; then
        add_error "$trial_from_cfg: has config.json but missing result.json"
      fi
    done

    if [[ "$job_name" != *"__"* ]]; then
      add_warning "$job_dir: job directory name does not match expected timestamp pattern"
    fi
  done

  local task
  for task in "${!task_counts[@]}"; do
    if (( task_counts["$task"] < 5 )); then
      add_error "$submission_name: task '$task' has ${task_counts["$task"]} trials (minimum required: 5)"
    fi
  done

  if (( ${#task_counts[@]} == 0 )); then
    add_error "$submission_name: no tasks discovered from trial results"
  fi

  if (( total_trials > 0 && total_trials < 400 )); then
    add_warning "$submission_name: only $total_trials trial results detected; full TB2 runs are usually much larger"
  fi
}

for submission_dir in "${SUBMISSION_DIRS[@]}"; do
  if [[ -f "$submission_dir/metadata.yaml" ]]; then
    validate_submission_dir "$submission_dir"
  else
    add_warning "$submission_dir: skipping because metadata.yaml is missing"
  fi
done

if (( validated_count == 0 )); then
  add_error "no valid submission directories found under: $INPUT_DIR"
fi

if (( ${#WARNINGS[@]} > 0 )); then
  echo "Warnings:"
  local_warning=""
  for local_warning in "${WARNINGS[@]}"; do
    echo "  - $local_warning"
  done
fi

if (( ${#ERRORS[@]} > 0 )); then
  echo "Validation failed with ${#ERRORS[@]} error(s):"
  local_error=""
  for local_error in "${ERRORS[@]}"; do
    echo "  - $local_error"
  done
  exit 1
fi

echo "Validation passed: no blocking issues found."
