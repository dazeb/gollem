# Terminal-Bench 2.0 Leaderboard Submission Checklist

Last verified: 2026-02-24

## Goal

Submit a valid `terminal-bench@2.0` run to the public leaderboard without failing validation.

## Hard Requirements (must pass)

1. Run at least 5 trials per task.
2. Do not modify timeouts or resources.
3. Include required submission structure and metadata.
4. Include valid `result.json` for every trial directory.
5. Keep job/config IDs consistent (no mixed/copied job directories with mismatched `config.job_id`).

## Canonical Run Commands

Use one of these patterns:

```bash
harbor run -d terminal-bench@2.0 -a "agent" -m "model" -k 5
```

```bash
harbor run -d terminal-bench@2.0 --agent-import-path "path.to.agent:SomeAgent" -k 5
```

## Submission Folder Layout

Place submissions in:

```text
submissions/terminal-bench/2.0/<agent>__<model(s)>/
```

Expected structure:

```text
submissions/
  terminal-bench/
    2.0/
      <agent>__<model>/
        metadata.yaml
        <job-folder>/
          config.json
          <trial-1>/result.json
          <trial-2>/result.json
          ...
```

## Required `metadata.yaml`

```yaml
agent_url: https://...
agent_display_name: "My Agent"
agent_org_display_name: "Org"

models:
  - model_name: gpt-5
    model_provider: openai
    model_display_name: "GPT-5"
    model_org_display_name: "OpenAI"
```

## Validation Rules to Check Before PR

1. `timeout_multiplier == 1.0`
2. No `override_timeout_sec`
3. No `override_setup_timeout_sec`
4. No `max_timeout_sec`
5. No verifier timeout overrides
6. No resource overrides:
   - `override_cpus`
   - `override_memory_mb`
   - `override_storage_mb`
7. Every trial directory has valid `result.json`
8. Every trial directory includes other run artifacts (not only `result.json`)
9. Each task has at least 5 trials in total (`-k 5` is the simplest way to satisfy this)

## Common Failure Modes We Should Avoid

1. `config.job_id` mismatch between `result.json` and enclosing job folder.
2. Submitting only one pass over tasks (89 trials) instead of 5x (typically 445 trials for TB2 full set).
3. Accidentally using custom timeout or resource overrides from local experiments.
4. Missing artifacts because jobs were partially copied/uploaded.
5. Assuming setup/build time is unlimited. Setup time is not agent-execution time, but setup/build still have their own timeout budgets and can fail the trial.

## Timeout Semantics (important)

1. Agent execution timeout comes from each task's `[agent].timeout_sec` and should not be overridden for leaderboard submissions.
2. Setup/install work is timed separately (`agent_setup`) and does not consume agent execution budget.
3. Environment build/start is also timed separately (`build_timeout_sec` in task config).
4. Practical implication: move dependency/LSP bootstrapping to setup when useful, but keep setup/build bounded and avoid heavy/unnecessary installs.

## Host Sizing Guidance (TB2)

TB2 task limits are declared in each task's `task.toml`:

1. `[environment]`: `cpus`, `memory`, `storage`, `build_timeout_sec`
2. `[agent]`: `timeout_sec`
3. `[verifier]`: `timeout_sec`

Current TB2 max per-task footprint is approximately:

1. 4 vCPU
2. 8G RAM
3. 10G storage

Recommended host sizing for `harbor run`:

| Concurrency (`-n`) | vCPU | RAM | NVMe Disk |
|---:|---:|---:|---:|
| 1 | 8 | 32 GB | 250 GB |
| 2 | 16 | 64 GB | 500 GB |
| 4 | 32 | 128 GB | 1 TB |

Rule of thumb:

1. vCPU ≈ `5 * n`
2. RAM ≈ `10 * n` GB
3. Disk working set ≈ `15 * n` GB, plus cache/log overhead

Use Linux `x86_64` for leaderboard runs, and do not override task timeouts/resources.

## PR Process

1. Fork leaderboard repo and create a branch.
2. Add submission folder under `submissions/terminal-bench/2.0/...`.
3. Open PR.
4. Wait for validator bot output.
5. If bot reports issues, fix and push until validation passes.
6. After merge, verify import on https://www.tbench.ai/leaderboard/terminal-bench/2.0

## Fast Pre-PR Sanity Script (manual checklist)

Run these checks before opening PR:

```bash
# 1) confirm we used 5 trials
echo "Expect >=5 trials per task"

# 2) spot timeout/resource override keys
rg -n "timeout_multiplier|override_timeout_sec|override_setup_timeout_sec|max_timeout_sec|override_cpus|override_memory_mb|override_storage_mb" submissions/terminal-bench/2.0/<agent>__<model(s)>/

# 3) ensure result files exist
find submissions/terminal-bench/2.0/<agent>__<model(s)> -name result.json | wc -l
```

## Automated Validator (recommended)

Use the repo validator target before opening a leaderboard PR:

```bash
make tbench-validate-submission SUBMISSION_DIR=submissions/terminal-bench/2.0/<agent>__<model(s)>
```

You can also validate a root directory containing multiple submissions:

```bash
make tbench-validate-submission SUBMISSION_DIR=submissions/terminal-bench/2.0
```

## Sources

1. https://www.tbench.ai/leaderboard/terminal-bench/2.0
2. https://huggingface.co/datasets/harborframework/terminal-bench-2-leaderboard
3. https://harborframework.com/docs/running-tbench
4. https://huggingface.co/datasets/harborframework/terminal-bench-2-leaderboard/discussions/17
5. https://huggingface.co/datasets/harborframework/terminal-bench-2-leaderboard/discussions/30
