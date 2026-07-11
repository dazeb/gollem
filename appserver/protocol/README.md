# Gollem App-Server Protocol Versioning

`ProtocolVersion` identifies the JSON-RPC method and envelope contract.
`SchemaVersion` identifies the generated schema document format. They change
independently: adding an optional field or a new method/type definition does
not require an envelope-version change.

## Compatibility Rules

- Existing method names, directions, item discriminators, and required fields
  are stable within one protocol version.
- Additive methods, definitions, and optional fields are allowed within the
  current protocol version. Clients must ignore unknown optional fields and
  preserve unknown timeline payloads as raw JSON.
- Removing or renaming a method/type/required field, changing a field type, or
  changing method direction requires a new protocol version and an explicit
  migration note. Adding a value to a closed schema enum also requires a new
  protocol version unless the binding explicitly preserves unknown values.
- Changing schema metadata or binding representation requires a new schema
  version even when the JSON-RPC wire remains compatible.
- Deferred and unavailable methods remain in the method inventory so clients
  can distinguish capability gaps from unknown methods.

## Version 1 Initialize Migration

`gollem.appserver.v1` makes the initialize wire compatible with the public
Codex identity and environment fields. Clients must send non-empty
`clientInfo.name` and `clientInfo.version`; `clientInfo.title` remains nullable
and optional on the JSON wire. Initialize capabilities accept the public
experimental API, attestation, MCP form-elicitation, and notification opt-out
fields, while Gollem's keyed experimental map remains an extension.

Initialize responses now require `userAgent`, `codexHome`, `platformFamily`,
and `platformOs`. Existing Gollem `protocolVersion`, `serverInfo`, server
capabilities, and method inventory fields remain present as additive
extensions. This required-field change is why the protocol version advances
from v0 to v1; the schema metadata representation remains v1.

## Version 1 Thread Discovery

`thread/list` accepts the public Codex cursor, limit, sort, provider, source,
archive, cwd, state-DB, and title-search fields. Lists default to active threads,
created-at descending, and a 50-record page capped at 100 records. Responses
include the public `data`, `nextCursor`, and `backwardsCursor` fields and retain
Gollem's `threads` array. Existing `statuses` and `includeDeleted` parameters
remain optional Gollem lifecycle extensions; clients that need archived or
deleted records must now request them explicitly and paginate the result.

`thread/read` accepts public `threadId` and optional `includeTurns`. The response
nests loaded turns and timeline items under the durable thread record while
retaining Gollem's top-level `turns` and `items` arrays and the existing `id`,
`includeItems`, `afterSeq`, and `limit` request extensions.

`ThreadLifecycleStatus` and `TurnLifecycleStatus` describe Gollem persistence
and execution state. They intentionally do not claim compatibility with
Codex's runtime `ThreadStatus` or public `TurnStatus`; the exported durable
records remain separate types until those runtime and item models converge.

## Version 1 Thread Controls

`thread/loaded/list`, `thread/archive`, `thread/unarchive`, `thread/delete`,
`thread/unsubscribe`, and `thread/name/set` use exported request/result
contracts. Public thread ids, names, cursors, limits, loaded-id arrays, and
unsubscribe statuses retain their Codex wire names and optionality. Legacy
`id`/`title` request aliases remain optional Gollem extensions.

Codex archive, delete, and set-name responses are empty objects. Gollem keeps
its existing returned durable record/name fields as optional response
extensions, so public empty responses remain valid generated values and v1
clients do not lose current result data. `ThreadUnarchiveResponse` still
contains `ThreadRecord`; it must not be treated as public Codex `Thread` until
the runtime thread/turn/item model converges.

## Version 1 Thread Goals

`thread/goal/get`, `thread/goal/set`, and `thread/goal/clear` use the public
structured goal model: objective, status, nullable token budget, token/time
usage, and Unix-second timestamps. Goal update and clear notifications use the
same exported model; update notifications carry a required nullable `turnId`.
Legacy `id` and arbitrary `goal`/`text`/`value` request fields remain optional
Gollem extensions, and existing stored goal strings or objects are decoded into
the structured response without discarding the stored value. Oversized legacy
objectives are projected to the public 4,000-character limit without rewriting
the underlying compatibility data. Generated validators and TypeScript require
either public `threadId` or legacy `id`; empty identifier objects remain invalid.
New legacy-form goals start accounting at set time, so earlier thread turns are
not charged to a newly created goal.

Turn records are the durable source of goal usage. On turn completion Gollem
derives token/time counters and budget-limited state from persisted turns,
emits a turn-correlated goal update, and returns the same derived snapshot from
`thread/goal/get`. It does not write a stale accounting snapshot back over a
concurrent user goal edit. Public goal mutations persist the structured goal,
preserve creation time and prior usage, validate the six closed status values,
limit objectives to 4,000 characters, and require positive non-null budgets.
Clearing or raising a derived exhausted budget returns the goal to `active`
unless the request also supplies an explicit status.

## Version 1 Thread Metadata And Memory Mode

`thread/metadata/update` accepts the public nested `gitInfo` patch with
independently optional nullable `sha`, `branch`, and `originUrl` fields.
Omission preserves the stored field, explicit null clears it, and non-null
strings are trimmed and must remain non-empty. Unrelated metadata and unknown
keys inside `gitInfo` are preserved. Legacy `id`, arbitrary `metadata`, and
`replace` fields remain optional Gollem extensions. The response intentionally
contains Gollem `ThreadRecord`, so it is generated and live-typed but must not
be treated as public Codex `ThreadMetadataUpdateResponse` until the nested
runtime thread model converges.

`thread/memoryMode/set` uses the public `enabled`/`disabled` enum and accepts
the public `threadId`/`mode` pair. Legacy `id` and `memoryMode` aliases remain
explicit generated variants. Its public empty response remains valid while
Gollem returns thread id, selected mode, and durable thread state as optional
extensions. `thread/name/updated` now emits required `threadId` and optional
public `threadName`; legacy name, durable thread, and timestamp details remain
optional extensions.

## Version 1 Thread Lifecycle Notifications

`thread/archived`, `thread/closed`, `thread/deleted`, and `thread/unarchived`
use exported notification contracts with the required public `threadId` field.
Archive, delete, and unarchive notifications preserve Gollem's lifecycle
status, durable `ThreadRecord`, and timestamp as optional extensions. The close
notification remains the exact public `{ threadId }` shape. These lifecycle
types intentionally do not make `thread/status/changed` public: that method
still depends on the distinct Codex runtime `ThreadStatus` model.

## Version 1 File-Change Approval Responses

`item/fileChange/requestApproval` binds the public `accept`,
`acceptForSession`, `decline`, and `cancel` decisions to its direct JSON-RPC
response. Session acceptance is connection-scoped and suppresses later prompts
only for the same normalized mutation target; cancel interrupts an active
runtime turn. The `approval/respond` extension remains available for legacy
clients. Command-execution and permissions direct responses remain unbound
until their policy-amendment and granted-profile dependencies are implemented.

## Version 1 File-Change Item Contracts

`FileUpdateChange`, `PatchChangeKind`, and `PatchApplyStatus` now use the public
file-change item contracts. Add and delete kinds carry only their discriminator.
Update kinds require nullable snake-case `move_path`; Gollem also accepts the
previous camel-case `movePath` form and emits both fields so existing v1
consumers continue to decode updates. When both inputs are present, the public
field wins even when null or empty. Unknown kinds, fields, and update payloads
without either move-path field fail closed.

`FileChangeItem.status` uses the exact `inProgress`, `completed`, `failed`, and
`declined` enum. The current artifact-change tracker emits its proven
`inProgress` then `completed` lifecycle; exporting the remaining public values
does not invent failure or denial items where no live file-change event exists.

## Version 1 Command Exec Controls

`command/exec/write`, `command/exec/resize`, and `command/exec/terminate` use
exported request/result contracts. Public `processId`, nullable base64 input,
close-stdin, and nested terminal-size fields take precedence; legacy `id`,
text/encoding/close, and flat size fields remain explicit generated variants.
Public empty responses remain valid while Gollem's existing `ok` and `path`
details are optional extensions. Follow-up controls are connection-scoped to
processes started by `command/exec`, and resize remains typed-unavailable until
a PTY backend is configured.

`command/exec/outputDelta` now uses the exported public process id, stream,
base64 delta, and cap flag contract. This does not make `command/exec` start or
its final response public: Gollem currently starts a legacy string command
asynchronously and returns a process snapshot, while the public contract
requires argv, PTY/sandbox/stream/cap/timeout controls and a deferred buffered
result after process exit.

## Generation

Run:

```bash
go generate ./appserver/protocol
```

This rewrites `schema.json` and
`typescript/gollem_appserver_protocol.ts` from exported Go wire types and
binding metadata. It also rewrites the TypeScript compile fixtures at
`typescript/testdata/runtime_wire_v1.ts`,
`typescript/testdata/initialize_wire_v1.ts`, and
`typescript/testdata/thread_discovery_wire_v1.ts`, plus
`typescript/testdata/thread_control_wire_v1.ts` and
`typescript/testdata/thread_goal_wire_v1.ts`, plus
`typescript/testdata/thread_metadata_wire_v1.ts` and
`typescript/testdata/thread_lifecycle_notification_wire_v1.ts`, plus
`typescript/testdata/command_exec_control_wire_v1.ts` and
`typescript/testdata/file_change_item_wire_v1.ts`, plus
`typescript/testdata/dynamic_tool_call_wire_v1.ts`, plus
`typescript/testdata/user_input_wire_v1.ts`, plus
`typescript/testdata/mcp_elicitation_wire_v1.ts`, plus
`typescript/testdata/command_approval_response_wire_v1.ts`. File-change item,
dynamic tool-call, user-input, MCP-elicitation, and command-approval variants
also have strict negative compile contracts. Tests compare generated bytes with
the checked-in files, verify every type binding references a known method and
definition, and enforce a compatibility floor for runtime items, daemon status,
initialization, thread discovery, thread controls, thread goals, metadata,
lifecycle notifications, command-exec controls, and file changes.
Dynamic client-tool, structured user-input, MCP-elicitation, and command
approval requests/responses are covered by the same floor and strict fixtures.

`testdata/runtime_wire_v1.json` contains versioned request, response, and
notification examples for generated-client compatibility tests.
`testdata/initialize_wire_v1.json` independently covers the initialize
request, initialize response, advertised method metadata, and initialized
notification. `testdata/thread_discovery_wire_v1.json` covers typed list/read
requests and responses, including nested turns and timeline items.
`testdata/thread_control_wire_v1.json` covers loaded-list and lifecycle-control
requests/responses, including public empty response forms.
`testdata/thread_goal_wire_v1.json` covers structured goal get/set/clear and
goal update/clear notifications. `testdata/thread_metadata_wire_v1.json`
covers public and legacy metadata/memory requests, their responses, and the
public rename notification.
`testdata/thread_lifecycle_notification_wire_v1.json` covers archive, close,
delete, and unarchive notifications.
`testdata/command_exec_control_wire_v1.json` covers public and legacy write,
resize, and terminate controls, empty-compatible responses, and streamed
output. `testdata/file_change_item_wire_v1.json` covers add/delete, public and
legacy updates, public-field precedence, and all four patch statuses.
`testdata/dynamic_tool_call_wire_v1.json` covers the exact public request,
optional Gollem v1 request metadata, and text/image response variants.
`testdata/user_input_wire_v1.json` covers the structured question/options
request, optional Gollem v1 prompt metadata, and exact answer-map response.
`testdata/mcp_elicitation_wire_v1.json` covers form, OpenAI-form, and URL
requests, the exact primitive schema family, optional Gollem correlation
metadata, and action/content/meta responses.
`testdata/command_approval_response_wire_v1.json` covers the live command
approval request's advertised decision subset and the exact direct response.

The current `request_mcp_elicitation` runtime tool validates and emits only the
public `form` variant. The `openai/form` and `url` variants are exported for
wire-compatible clients but are not synthesized by the runtime. Bridging
protocol-originated MCP elicitation requests remains separate work.

Consumers can use `x-gollem-type-bindings` to select method parameter/result
definitions and `x-gollem-item-payload-bindings` to decode
`TimelineItem.payload` by `kind`.

When TypeScript is installed, verify the generated binding and v1 fixtures with:

```bash
tsc --project appserver/protocol/typescript/tsconfig.json
```

The generated TypeScript uses `unknown` for open JSON values and unclassified
timeline payloads. Consumers must narrow those values at runtime and preserve
unknown item kinds rather than casting them to a known variant.
