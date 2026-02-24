# AGENTS.md

## Terminal-Bench 2.0 Submission Workflow

Use this before opening any leaderboard PR.

1. Prepare the submission directory under `submissions/terminal-bench/2.0/<agent>__<model>/`.
2. Run the local validator:

```bash
make tbench-validate-submission SUBMISSION_DIR=submissions/terminal-bench/2.0/<agent>__<model>
```

3. Remember timeout semantics:
- setup/build time is separate from agent execution time,
- but setup/build still have their own timeout budgets,
- and leaderboard submissions must not use timeout/resource overrides.

4. If validating multiple candidate submissions at once:

```bash
make tbench-validate-submission SUBMISSION_DIR=submissions/terminal-bench/2.0
```

## Canonical Reference

Detailed checklist and failure modes are documented in:

- `contrib/tbench_submission_checklist.md`
