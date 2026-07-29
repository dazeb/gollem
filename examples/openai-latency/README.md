# openai-latency

A reproducible harness for investigating latency in ChatGPT OAuth-backed OpenAI
requests ([issue #242](https://github.com/fugue-labs/gollem/issues/242)).

It loads ChatGPT subscription credentials, builds one provider per transport
(`http` `Request`, `http` `RequestStream`, `websocket`), runs a fixed prompt
across a growing conversation history, and prints a **secret-safe per-request
phase-breakdown table**.

## What it measures

For every physical provider request the harness prints, measured from request
start:

| column | meaning |
| ------ | ------- |
| `refresh` | OAuth token-refresh duration (distinguishes refresh I/O from the request itself) |
| `ttfb` | time to response headers (HTTP) or websocket dial+handshake |
| `first_event` | time to first SSE/websocket event |
| `first_token` | time to first text/tool delta |
| `terminal` | time to the terminal response event |
| `total` | wall-clock for the whole provider call |
| `ws_reused` | whether the websocket connection was reused |
| `prev_reused` | whether `previous_response_id` continuation was used |
| `cache` | prompt-cache-key fingerprint (continuity check, not the key) |
| `status` / `err` | HTTP status + sanitized error classification |

Together these let you separate refresh time from request upload, backend
queueing/reasoning (`first_event − ttfb`), streaming (`terminal − first_token`),
and retry gaps (separate rows).

## Credentials & safety

No access/refresh/ID tokens, account IDs, authorization URLs, callback queries,
device codes, or raw provider error bodies are ever printed — only sizes,
timings, sanitized error markers, and a non-reversible cache-key fingerprint.

Log in once to populate `~/.golem/auth.json` with a ChatGPT Plus/Pro/Team
account, then run:

```bash
OPENAI_MODEL=gpt-5.3-codex go run ./examples/openai-latency
```

### Environment

| variable | default | notes |
| -------- | ------- | ----- |
| `OPENAI_MODEL` | `gpt-5.3-codex` | model to request |
| `OPENAI_AUTH_PATH` | `~/.golem/auth.json` | path to chatgpt auth file |
| `OPENAI_TRANSPORTS` | `http,stream,websocket` | comma list of transports |
| `OPENAI_HISTORY_TURNS` | `1,5,15` | comma list of history sizes |

## Comparing against official Codex

This harness produces Gollem-side before/after evidence across transports and
history sizes. The comparison against **official Codex 0.144.6** (the version
bundled by `openai/codex-security`) is intentionally run separately because it
requires that binary; collect its per-request timings the same way (same account,
model, prompt, reasoning effort, and history sizes) and diff the phase
breakdowns. The questions this is designed to answer (from the issue):

- Is the difference total completion time, time to first token, or both?
- Is Gollem refreshing OAuth tokens more often than intended?
- Is HTTP full-context upload/reprocessing the dominant difference as history grows?
- Are retries or `Retry-After` sleeps hidden inside the observed latency?
- Is provider recreation defeating prompt-cache continuity?

## Follow-ups (explicitly tracked separately)

This investigation deliberately does **not** change any transport default. Per
the issue, the following are tracked as follow-up work pending this evidence:

- Canonical ChatGPT account-claim parsing (`https://api.openai.com/auth` object,
  including `chatgpt_account_id`, plan type, FedRAMP routing) for OAuth parity.
- Refresh/recovery parity (JSON refresh grants, serialized refreshes, guarded
  credential reload, 401 recovery via reload → refresh → retry).
- Any proposed default change (e.g. websocket-by-default for ChatGPT) — to be
  evaluated only with the evidence this harness produces.
