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
`typescript/testdata/thread_metadata_wire_v1.ts`. Tests compare generated bytes
with the checked-in files, verify every type binding references a known method
and definition, and enforce a compatibility floor for runtime items, daemon
status, initialization, thread discovery, thread controls, and thread goals.

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
public rename notification. Consumers can use
`x-gollem-type-bindings` to select method parameter/result definitions and
`x-gollem-item-payload-bindings` to decode `TimelineItem.payload` by `kind`.

When TypeScript is installed, verify the generated binding and v1 fixtures with:

```bash
tsc --project appserver/protocol/typescript/tsconfig.json
```

The generated TypeScript uses `unknown` for open JSON values and unclassified
timeline payloads. Consumers must narrow those values at runtime and preserve
unknown item kinds rather than casting them to a known variant.
