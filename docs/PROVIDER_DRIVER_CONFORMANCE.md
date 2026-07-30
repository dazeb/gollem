# Provider Driver Conformance

This matrix is the authority for the common provider behavior that Slang may
present as runnable. A `proven` entry has deterministic fixture coverage through
the provider-neutral `core.Model` boundary. An entry marked `unsupported` must
remain unavailable in the catalog and renderer. `Not yet proven` is not a claim
of behavior and must not enable a control solely on provider identity.

## Common Driver Matrix

| Behavior | Native OpenAI | OpenAI-compatible local | Native Anthropic | Evidence |
| --- | --- | --- | --- | --- |
| Non-stream text response | Proven | Proven | Proven | `provider/conformance` |
| Function-tool request and normalized tool call | Proven | Proven | Proven | `provider/conformance` |
| Streaming text and terminal usage | Proven | Proven | Proven | `provider/conformance` |
| In-flight request cancellation | Proven | Proven | Proven | `provider/conformance` |
| Structured output | Catalog-supported | Unsupported | Catalog-supported | Provider-specific tests; no common conformance claim yet |
| Vision | Catalog-supported | Unsupported | Catalog-supported | Provider-specific tests; no common conformance claim yet |
| Reasoning visibility | Proven where catalog-supported | Unsupported | Proven where catalog-supported | `provider/conformance` verifies native `ThinkingPart` start/delta events and final retention; local Chat Completions remains unsupported |
| Prompt cache / Responses API | Catalog-supported where applicable | Unsupported | Catalog-supported where applicable | Provider-specific tests; no common conformance claim yet |
| Malformed JSON stream event normalization | Proven | Proven | Proven | `provider/conformance` plus provider parser tests; returns `StreamProtocolError` without raw event data |
| Abrupt EOF partial-stream result | Proven | Proven | Proven | `provider/conformance` plus provider parser tests; preserves partial response and returns `StreamIncompleteError` |
| Read-error peer-disconnect classification | Proven | Proven | Proven | `provider/conformance` plus provider parser tests; returns source-free `StreamTransportError`, while context cancellation remains intact |
| Retryable 429 request recovery | Proven | Proven | Proven | `provider/conformance` exercises bounded `modelutil.RetryModel` recovery |
| Request deadline propagation | Proven | Proven | Proven | `provider/conformance` confirms initial-request and post-header streaming cancellation with `context.DeadlineExceeded` normalization |
| Post-output retry / replay | Not yet proven | Not yet proven | Not yet proven | A stream may have produced caller-visible output; recovery must not replay it without an explicit safe-resume contract |
| Endpoint health probe | Unsupported | Proven | Unsupported | `provider/health/probe` performs a loopback-only `GET /v1/models`; it returns only a typed status and never starts a model turn |
| Capability mismatch | Catalog/daemon proven | Catalog/daemon proven | Catalog/daemon proven | `ValidateAgentRuntimeSelection` rejects unconfigured, unknown, cross-provider, and non-streaming/non-tool-capable selections before the daemon persists a thread or turn; Slang continues to render the same condition as unavailable |

`Catalog-supported` means the catalog may expose the capability only for the
listed provider/model profile. It does not make that behavior part of the common
driver contract until a deterministic conformance scenario covers it.

## Custody And Local Endpoint Rules

- Credentials, base URLs, headers, raw payloads, and transport identity remain
  inside the Gollem process. Catalog entries expose only provider IDs, model IDs,
  capability descriptors, and configuration variable names.
- The local OpenAI-compatible profile accepts only loopback HTTP(S) endpoints,
  forces Chat Completions, and does not inherit OpenAI Responses, ChatGPT,
  prompt-cache, or remote transport settings.
- A local endpoint connection failure is a bounded `local endpoint unavailable`
  error. It must not reveal the endpoint or local token through app-server,
  catalog, or renderer diagnostics.
- `provider/health/probe` is available only for the explicitly configured local
  profile. It returns `available`, `unavailable`, `not-configured`, or
  `unsupported`; the probe reads `/v1/models` and does not invoke a model.

## Adding Or Expanding A Claim

1. Add a deterministic fixture scenario to `provider/conformance` for every
   claimed driver.
2. Assert the same normalized `core.Model` result and terminal behavior for all
   drivers that claim it.
3. Add provider-specific tests when a wire format needs richer assertions.
4. Update the catalog capability and Slang control gate only after the common
   scenario is green.
5. Keep unsupported and not-yet-proven behavior visible only as typed
   unavailable or degraded state; do not silently fall back to another provider
   surface.
