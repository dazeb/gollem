# Terminal-Bench 2.0 Readiness Report For Gollem Harness

Date: 2026-02-26

## Executive Summary

- Scope: all **89** tasks in Terminal-Bench 2.0 (official task repo commit `f5b891cb4f7c20e306f9d05887628b43af740f43`).
- Overall readiness score (mean): **57.2/100**.
- Score range: **20/100** to **86/100**.
- Highest-readiness cluster: git/software-engineering/debugging tasks.
- Lowest-readiness cluster: multimodal video/image, heavy ML/distributed training, R/Stan, and virtualization-heavy tasks.
- Competitive implication: current harness is solid for general coding loops, but to be leaderboard-competitive against top custom agents, we need explicit multimodal and specialized runtime/toolchain capabilities.

## What Was Scored

- Source of truth for tasks: `laude-institute/terminal-bench-2` task folders (`task.toml` + `instruction.md`).
- Scoring unit: one readiness score per benchmark task (0-100).
- Readiness definition: expected probability-adjusted execution quality for this harness (not raw model IQ), given current tools, setup, middleware, and runtime defaults.

## Scoring Method

- Base score components:
  - Base = `50`
  - Harness implementation boost = `+18` (verification checkpoint, setup prewarm, code-mode batching, time-budget controls, priority tier wiring).
  - Difficulty adjustment: easy `+8`, medium `+0`, hard `-10`.
  - Category adjustments by benchmark domain (e.g., software-engineering positive, model-training/video-processing negative).
- Capability flags from tags/instructions/task IDs then apply impacts, e.g.:
  - Multimodal/video/image/OCR, QEMU/Windows VM, ML-heavy/distributed, R/Stan, reverse engineering, crypto/math, large-data, strict performance targets.
- Final score clipped to `[5,95]` to avoid false precision at extremes.

### Difficulty Breakdown

| Difficulty | Task Count | Mean Score |
|---|---:|---:|
| easy | 4 | 72.8 |
| medium | 55 | 62.0 |
| hard | 30 | 46.3 |

### Category Breakdown

| Category | Task Count | Mean Score |
|---|---:|---:|
| personal-assistant | 1 | 69.0 |
| debugging | 5 | 67.4 |
| data-processing | 4 | 62.5 |
| software-engineering | 26 | 62.3 |
| security | 8 | 61.8 |
| file-operations | 5 | 60.8 |
| optimization | 1 | 60.0 |
| system-administration | 9 | 59.6 |
| data-querying | 1 | 55.0 |
| scientific-computing | 8 | 55.0 |
| games | 1 | 53.0 |
| data-science | 8 | 49.5 |
| mathematics | 4 | 45.5 |
| model-training | 4 | 40.5 |
| machine-learning | 3 | 37.3 |
| video-processing | 1 | 20.0 |

## Competitive Snapshot (Public Leaderboard Research)

- Leaderboard entries parsed: **111** (snapshot date: 2026-02-26).
- Top overall entries at snapshot:

| Rank | Agent | Model | Accuracy | Date | Verified |
|---:|---|---|---:|---|---|
| 1 | Droid | GPT-5.3-Codex | 0.7730 | 2026-02-24 | no |
| 2 | Simple Codex | GPT-5.3-Codex | 0.7506 | 2026-02-06 | yes |
| 3 | Terminus-KIRA | Gemini 3.1 Pro | 0.7483 | 2026-02-23 | no |
| 4 | Terminus-KIRA | Claude Opus 4.6 | 0.7472 | 2026-02-22 | no |
| 5 | Judy | Claude Opus 4.6 | 0.7191 | 2026-02-22 | no |
| 6 | CodeBrain-1 | GPT-5.3-Codex | 0.7034 | 2026-02-10 | no |
| 7 | Droid | Claude Opus 4.6 | 0.6989 | 2026-02-05 | no |
| 8 | Mux | GPT-5.3-Codex | 0.6854 | 2026-02-09 | no |
| 9 | Crux | Claude Opus 4.6 | 0.6687 | 2026-02-23 | no |
| 10 | Deep Agents | GPT-5.2-Codex | 0.6652 | 2026-02-12 | no |

- Top verified entries at snapshot:

| Rank* | Agent | Model | Accuracy | Date |
|---:|---|---|---:|---|
| 1 | Simple Codex | GPT-5.3-Codex | 0.7506 | 2026-02-06 |
| 2 | Terminus 2 | GPT-5.3-Codex | 0.6472 | 2026-02-05 |
| 3 | Ante | Gemini 3 Pro | 0.6472 | 2026-01-06 |
| 4 | Terminus 2 | Claude Opus 4.6 | 0.6292 | 2026-02-06 |
| 5 | Codex CLI | GPT-5.2 | 0.6292 | 2025-12-18 |
| 6 | Codex CLI | GPT-5.1-Codex-Max | 0.6045 | 2025-11-24 |
| 7 | Claude Code | Claude Opus 4.6 | 0.5798 | 2026-02-07 |
| 8 | Terminus 2 | Claude Opus 4.5 | 0.5775 | 2025-11-22 |

*Rank within verified-only subset, not absolute leaderboard rank.

- Comparable public agent baselines (best listed run per agent):

| Agent | Best Accuracy | Model | Date | Verified |
|---|---:|---|---|---|
| Deep Agents | 0.6652 | GPT-5.2-Codex | 2026-02-12 | no |
| Terminus 2 | 0.6472 | GPT-5.3-Codex | 2026-02-05 | yes |
| Codex CLI | 0.6292 | GPT-5.2 | 2025-12-18 | yes |
| Claude Code | 0.5798 | Claude Opus 4.6 | 2026-02-07 | yes |
| OpenHands | 0.5191 | Claude Opus 4.5 | 2026-01-04 | yes |
| Mini-SWE-Agent | 0.4253 | Claude Sonnet 4.5 | 2025-11-03 | yes |
| Gemini CLI | 0.5101 | Gemini 3 Flash | 2025-12-23 | no |
| Simple Codex | 0.7506 | GPT-5.3-Codex | 2026-02-06 | yes |

## Framework Gaps To Close (Prioritized)

| Priority | Capability Gap | Affected Tasks | Estimated Score Lift Potential* | What To Add |
|---:|---|---:|---:|---|
| 1 | ML training/inference runtime | 9 | +108 | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup). |
| 2 | image/diagram understanding | 6 | +84 | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop. |
| 3 | large-data streaming/chunking | 13 | +78 | Add streaming transforms and chunk-safe edit/search operations for huge files. |
| 4 | auto-profiling and optimization loops | 11 | +66 | Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| 5 | niche language toolchains (OCaml/Coq/Cobol/R) | 8 | +64 | Preinstall/verify niche compilers and language servers in setup. |
| 6 | video understanding | 2 | +40 | Add frame/audio extraction + VLM summarization tools for end-to-end video tasks. |
| 7 | QEMU/VM orchestration | 3 | +36 | Add first-class VM helpers (boot detection, port/protocol probes, readiness checks). |
| 8 | forensics/recovery utilities | 5 | +30 | Preinstall forensics stack (sqlite salvage, file-carving, metadata recovery). |
| 9 | advanced crypto/math attack templates | 3 | +30 | Add crypto-analysis notebooks/templates and symbolic math helpers. |
| 10 | bioinformatics helpers | 3 | +30 | Add FASTA/plasmid/primer helper tools and validation scripts. |

*Lift potential is a heuristic aggregate from rubric penalties; it represents directional impact, not guaranteed benchmark points.

## Recommended Implementation Plan

1. Add multimodal toolchain: image OCR/VLM + video frame extraction/VLM summarization tools integrated as first-class agent tools.
2. Add specialized runtime profiles: ML-heavy/GPU profile and virtualization profile with task-aware environment selection.
3. Add reverse-engineering/forensics pack: disassembly/decompilation wrappers and recovery utilities.
4. Add niche toolchain prewarm matrix: R+Stan, OCaml, Coq, Cobol, plus stronger setup verification in `agent.setup`.
5. Add performance loop middleware: automatic profile->optimize->verify cycles for strict runtime/model-size tasks.
6. Add robust network fetch helper with mirrors/checksums for tasks requiring external downloads.
7. Keep OpenAI priority defaults on all eval paths and enforce via preflight assertions.

## Harness Strengths Already Present

- Strong verification gate that blocks unverified/failing completions and detects regression/stagnation patterns.
- Rich coding toolset (`bash`, `view`, `write`, `edit`, `multi_edit`, `grep`, `glob`, `ls`, `lsp`) with code-mode batching support.
- Setup-time dependency prewarming and timeout-aware runtime behavior in Harbor adapter.
- Priority processing support for OpenAI wired through run scripts and environment defaults (`OPENAI_SERVICE_TIER=priority`).

## Full Task-by-Task Readiness Scores

| Task | Category | Difficulty | Readiness Score | Main Risk Drivers | Recommended Additions |
|---|---|---|---:|---|---|
| `adaptive-rejection-sampler` | scientific-computing | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `bn-fit-modify` | scientific-computing | hard | 41 | reverse engineering, large data volume | Integrate disassembler/decompiler wrappers (rizin/radare2/Ghidra headless).; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `break-filter-js-from-html` | security | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `build-cython-ext` | debugging | medium | 73 | heavy compilation | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache). |
| `build-pmars` | software-engineering | medium | 68 | heavy compilation | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache). |
| `build-pov-ray` | software-engineering | medium | 64 | heavy compilation, network/download dependency | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache).; Add resilient fetch utility with retry/backoff/mirror + checksum support. |
| `caffe-cifar-10` | machine-learning | medium | 44 | ML-heavy workload, network/download dependency | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Add resilient fetch utility with retry/backoff/mirror + checksum support. |
| `cancel-async-tasks` | software-engineering | hard | 62 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `chess-best-move` | games | medium | 53 | image understanding | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop. |
| `circuit-fibsqrt` | software-engineering | hard | 62 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `cobol-modernization` | software-engineering | easy | 72 | rare toolchain | Preinstall/verify niche compilers and language servers in setup. |
| `code-from-image` | software-engineering | medium | 48 | image understanding, OCR/doc parsing | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop.; Add PDF/JPG OCR + document classification primitives with confidence scoring. |
| `compile-compcert` | system-administration | medium | 55 | heavy compilation, rare toolchain | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache).; Preinstall/verify niche compilers and language servers in setup. |
| `configure-git-webserver` | system-administration | hard | 63 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `constraints-scheduling` | personal-assistant | medium | 69 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `count-dataset-tokens` | model-training | medium | 43 | ML-heavy workload, large data volume | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `crack-7z-hash` | security | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `custom-memory-heap-crash` | debugging | medium | 61 | heavy compilation, strict perf target | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache).; Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `db-wal-recovery` | file-operations | medium | 64 | forensics/recovery | Preinstall forensics stack (sqlite salvage, file-carving, metadata recovery). |
| `distribution-search` | machine-learning | medium | 42 | ML-heavy workload, large data volume | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `dna-assembly` | scientific-computing | hard | 45 | biology/scientific domain | Add FASTA/plasmid/primer helper tools and validation scripts. |
| `dna-insert` | scientific-computing | medium | 55 | biology/scientific domain | Add FASTA/plasmid/primer helper tools and validation scripts. |
| `extract-elf` | file-operations | medium | 62 | reverse engineering | Integrate disassembler/decompiler wrappers (rizin/radare2/Ghidra headless). |
| `extract-moves-from-video` | file-operations | hard | 36 | video understanding, network/download dependency | Add frame/audio extraction + VLM summarization tools for end-to-end video tasks.; Add resilient fetch utility with retry/backoff/mirror + checksum support. |
| `feal-differential-cryptanalysis` | mathematics | hard | 42 | crypto/math attack | Add crypto-analysis notebooks/templates and symbolic math helpers. |
| `feal-linear-cryptanalysis` | mathematics | hard | 42 | crypto/math attack | Add crypto-analysis notebooks/templates and symbolic math helpers. |
| `filter-js-from-html` | security | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `financial-document-processor` | data-processing | medium | 44 | image understanding, OCR/doc parsing | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop.; Add PDF/JPG OCR + document classification primitives with confidence scoring. |
| `fix-code-vulnerability` | security | hard | 55 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `fix-git` | software-engineering | easy | 86 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `fix-ocaml-gc` | software-engineering | hard | 44 | heavy compilation, strict perf target, rare toolchain | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache).; Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `gcode-to-text` | file-operations | medium | 70 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `git-leak-recovery` | software-engineering | medium | 72 | forensics/recovery | Preinstall forensics stack (sqlite salvage, file-carving, metadata recovery). |
| `git-multibranch` | system-administration | medium | 73 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `gpt2-codegolf` | software-engineering | hard | 56 | large data volume | Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `headless-terminal` | software-engineering | medium | 76 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `hf-model-inference` | data-science | medium | 50 | ML-heavy workload, network/download dependency | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Add resilient fetch utility with retry/backoff/mirror + checksum support. |
| `install-windows-3.11` | system-administration | hard | 30 | virtualization, windows legacy VM | Add first-class VM helpers (boot detection, port/protocol probes, readiness checks).; Prebake Windows/QEMU runbooks and verification macros for retro OS tasks. |
| `kv-store-grpc` | software-engineering | medium | 72 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `large-scale-text-editing` | file-operations | medium | 72 | large data volume | Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `largest-eigenval` | mathematics | medium | 56 | strict perf target | Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `llm-inference-batching-scheduler` | machine-learning | hard | 26 | ML-heavy workload, large data volume, strict perf target | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `log-summary-date-ranges` | data-processing | medium | 72 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `mailman` | system-administration | medium | 67 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `make-doom-for-mips` | software-engineering | hard | 62 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `make-mips-interpreter` | software-engineering | hard | 62 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `mcmc-sampling-stan` | data-science | hard | 30 | R/Stan workload, large data volume, rare toolchain | Provide prevalidated R+Stan environments and troubleshooting macros.; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `merge-diff-arc-agi-task` | debugging | medium | 77 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `model-extraction-relu-logits` | mathematics | hard | 42 | crypto/math attack | Add crypto-analysis notebooks/templates and symbolic math helpers. |
| `modernize-scientific-stack` | scientific-computing | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `mteb-leaderboard` | data-science | medium | 66 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `mteb-retrieve` | data-science | medium | 66 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `multi-source-data-merger` | data-processing | medium | 62 | large data volume | Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `nginx-request-logging` | system-administration | medium | 67 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `openssl-selfsigned-cert` | security | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `overfull-hbox` | debugging | easy | 61 | heavy compilation, strict perf target, rare toolchain | Add adaptive build orchestration (auto-timeout ramp + ccache-like warm cache).; Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `password-recovery` | security | hard | 49 | forensics/recovery | Preinstall forensics stack (sqlite salvage, file-carving, metadata recovery). |
| `path-tracing` | software-engineering | hard | 48 | image understanding | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop. |
| `path-tracing-reverse` | software-engineering | hard | 40 | image understanding, reverse engineering | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop.; Integrate disassembler/decompiler wrappers (rizin/radare2/Ghidra headless). |
| `polyglot-c-py` | software-engineering | medium | 72 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `polyglot-rust-c` | software-engineering | hard | 54 | weak oracle | Add fallback policy: multiple candidate generation + differential verification. |
| `portfolio-optimization` | optimization | medium | 60 | strict perf target | Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `protein-assembly` | scientific-computing | hard | 45 | biology/scientific domain | Add FASTA/plasmid/primer helper tools and validation scripts. |
| `prove-plus-comm` | software-engineering | easy | 72 | rare toolchain | Preinstall/verify niche compilers and language servers in setup. |
| `pypi-server` | software-engineering | medium | 72 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `pytorch-model-cli` | model-training | medium | 55 | ML-heavy workload | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup). |
| `pytorch-model-recovery` | model-training | medium | 37 | ML-heavy workload, forensics/recovery, large data volume | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Preinstall forensics stack (sqlite salvage, file-carving, metadata recovery). |
| `qemu-alpine-ssh` | system-administration | medium | 59 | virtualization | Add first-class VM helpers (boot detection, port/protocol probes, readiness checks). |
| `qemu-startup` | system-administration | medium | 55 | virtualization | Add first-class VM helpers (boot detection, port/protocol probes, readiness checks). |
| `query-optimize` | data-science | medium | 60 | strict perf target | Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `raman-fitting` | scientific-computing | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `regex-chess` | software-engineering | hard | 62 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `regex-log` | data-processing | medium | 72 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `reshard-c4-data` | data-science | medium | 60 | large data volume | Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `rstan-to-pystan` | data-science | medium | 34 | R/Stan workload, large data volume, strict perf target | Provide prevalidated R+Stan environments and troubleshooting macros.; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `sam-cell-seg` | data-science | hard | 30 | image understanding, ML-heavy workload | Add a vision tool (image read/OCR/board-state extraction) callable from the agent loop.; Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup). |
| `sanitize-git-repo` | security | medium | 65 | large data volume | Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `schemelike-metacircular-eval` | software-engineering | medium | 64 | rare toolchain | Preinstall/verify niche compilers and language servers in setup. |
| `sparql-university` | data-querying | hard | 55 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `sqlite-db-truncate` | debugging | medium | 65 | forensics/recovery | Preinstall forensics stack (sqlite salvage, file-carving, metadata recovery). |
| `sqlite-with-gcov` | system-administration | medium | 67 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `torch-pipeline-parallelism` | software-engineering | hard | 48 | distributed ML | Ship reusable tensor/pipeline parallel code templates and self-check harnesses. |
| `torch-tensor-parallelism` | software-engineering | hard | 48 | distributed ML | Ship reusable tensor/pipeline parallel code templates and self-check harnesses. |
| `train-fasttext` | model-training | hard | 27 | ML-heavy workload, large data volume, strict perf target | Introduce ML-capable runner profiles (GPU-capable envs + model/cache warmup).; Add streaming transforms and chunk-safe edit/search operations for huge files. |
| `tune-mjcf` | scientific-computing | medium | 59 | strict perf target | Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `video-processing` | video-processing | hard | 20 | video understanding, strict perf target | Add frame/audio extraction + VLM summarization tools for end-to-end video tasks.; Add automatic profile->optimize->retest loop with budget-aware cutoffs. |
| `vulnerable-secret` | security | medium | 65 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `winning-avg-corewars` | software-engineering | medium | 72 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |
| `write-compressor` | software-engineering | hard | 62 | none prominent | Keep current harness defaults; focus on model quality and iterative benchmark tuning. |

## Sources

- Terminal-Bench 2.0 task repository (authoritative tasks and metadata): https://github.com/laude-institute/terminal-bench-2
- Terminal-Bench 2.0 registry page: https://www.tbench.ai/registry/terminal-bench/2.0
- Terminal-Bench 2.0 leaderboard page: https://www.tbench.ai/leaderboard/terminal-bench/2.0
- Harbor installed agent wrappers (competitive integration patterns):
  - Codex wrapper: https://github.com/laude-institute/harbor/blob/main/src/harbor/agents/installed/codex.py
  - Claude Code wrapper: https://github.com/laude-institute/harbor/blob/main/src/harbor/agents/installed/claude_code.py
  - Gemini CLI wrapper: https://github.com/laude-institute/harbor/blob/main/src/harbor/agents/installed/gemini_cli.py
  - OpenHands wrapper: https://github.com/laude-institute/harbor/blob/main/src/harbor/agents/installed/openhands.py
- Deep Agents Harbor guide (public harness patterns): https://github.com/langchain-ai/deepagents/blob/main/libs/harbor/README.md
- LangChain public harness writeup (self-verification and harness gains): https://blog.langchain.com/how-to-build-great-agent-harnesses
- OpenAI priority processing docs: https://developers.openai.com/api/docs/guides/priority-processing
