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
| Reasoning visibility | Catalog-supported where applicable | Unsupported | Catalog-supported where applicable | Provider-specific tests; no common conformance claim yet |
| Prompt cache / Responses API | Catalog-supported where applicable | Unsupported | Catalog-supported where applicable | Provider-specific tests; no common conformance claim yet |
| Malformed stream normalization | Not yet proven | Not yet proven | Not yet proven | Follow-up conformance scenario required |
| Partial-stream / peer disconnect result | Not yet proven | Not yet proven | Not yet proven | Follow-up conformance scenario required |
| Timeout and retryability classification | Not yet proven | Not yet proven | Not yet proven | Follow-up conformance scenario required |
| Endpoint health probe and capability mismatch | Not yet proven | Not yet proven | Not yet proven | Must remain a typed unavailable/degraded state |

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
