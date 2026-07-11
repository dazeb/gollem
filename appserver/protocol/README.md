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

## Version 1 Exact Live Notification Values

`deprecationNotice`, `thread/compacted`, `thread/tokenUsage/updated`, and
`turn/diff/updated` bind the exact public `DeprecationNoticeNotification`,
`ContextCompactedNotification`, `ThreadTokenUsageUpdatedNotification`, and
`TurnDiffUpdatedNotification` names. Token usage uses the exact public
`ThreadTokenUsage` value and keeps both `modelContextWindow` and deprecation
`details` required and nullable on the wire. Gollem's original
`ThreadCompactedNotificationParams`, `ThreadTokenUsageUpdatedNotificationParams`,
`TokenUsage`, and `TurnDiffUpdatedNotificationParams` exports remain aliases so
existing protocol-v1 generated clients continue to compile.

## Version 1 Exact Live Primitive Values

`item/commandExecution/outputDelta`, `item/fileChange/patchUpdated`,
`item/mcpToolCall/progress`, and `serverRequest/resolved` bind the exact public
notification names. `RequestId` preserves either a string or signed integer
JSON-RPC id without coercion, and empty file-change patch arrays are emitted as
`[]` rather than `null`. Command-execution, dynamic-tool-call, and MCP-tool-call
item statuses reference their exact public closed enums; MCP errors reference
the exact public `McpToolCallError` spelling.

Gollem's original `*NotificationParams` names and `MCPToolCallError` remain
generated aliases. Their JSON shapes and source compatibility are preserved
while new clients infer the exact public names from method bindings.

## Version 1 File-Change Approval Responses

`item/fileChange/requestApproval` binds the public `accept`,
`acceptForSession`, `decline`, and `cancel` decisions to its direct JSON-RPC
response. Session acceptance is connection-scoped and suppresses later prompts
only for the same normalized mutation target; cancel interrupts an active
runtime turn. The `approval/respond` extension remains available for legacy
clients.

## Version 1 Command-Execution Approval Responses

`item/commandExecution/requestApproval` binds the exact public six-variant
decision and response types. Current requests advertise only `accept`,
`decline`, and `cancel`; unoffered session, exec-policy-amendment, and
network-policy-amendment results fail closed. Cancel interrupts an active
runtime turn before the blocked process can start. The additional public
variants remain exported for wire-compatible clients but are not applied until
Gollem has matching policy enforcement.

## Exported Permission Profile Contracts

The schema exports the public absolute/legacy path, filesystem path and special
path unions, sandbox entries, filesystem/network overlays, request/granted
profiles, active profile, turn/session scope, and permission request/response
types. These definitions are intentionally not bound to the live
`item/permissions/requestApproval` result. Gollem currently uses that method for
specific Git and MCP action approval through `PermissionsApprovalRequestParams`;
it does not yet request, intersect, store, or enforce environment-keyed
filesystem/network grants or strict-auto-review state. Binding a profile result
to that action gate would fabricate broader authority. Direct permission
responses therefore remain fail closed while legacy `approval/respond`
continues to resolve the existing action request. Generated TypeScript uses the
public canonical output shape with explicit nullable profile fields and scope;
Go decoding also accepts omitted upstream-defaulted fields and normalizes them
to that canonical form.

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

## Version 1 Filesystem Contracts

The nine `fs/*` requests and `fs/changed` notification bind the exact public
filesystem family. Generated clients use absolute paths, base64 file contents,
required metadata flags and millisecond timestamps, and non-null directory and
watch-path arrays. Gollem continues to accept its existing relative workspace
paths and text/encoding write form at runtime. Read, metadata, directory, and
mutation responses include the public fields plus optional legacy detail fields;
public empty mutation responses remain valid.

Create and remove preserve the public default of recursive operation, with
remove also defaulting to force. Explicit false values use non-recursive and
non-force service operations rather than being ignored. Public `sourcePath` and
`destinationPath` directory copies require `recursive: true`, while legacy
`source`/`destination` requests retain Gollem's recursive default. Both forms
stay on the same scoped path-resolution, approval, and audit path.

`fs/changed` has two explicit payload variants. `FsChangedNotification` is the
public watch event with `watchId` and absolute `changedPaths`;
`FileChangedNotification` is Gollem's mutation-level extension with operation,
timestamp, and optional source/destination paths. Consumers must narrow the
union by fields instead of assuming every event belongs to a watch.

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

## Exported Thread-Item Prerequisites

The schema exports the exact public `ByteRange`, `TextElement`, `ImageDetail`,
`UserInput`, `MessagePhase`, `MemoryCitationEntry`, `MemoryCitation`, and
`HookPromptFragment` contracts. Byte and citation offsets are nonnegative but
do not impose range ordering. Text elements retain a required nullable
placeholder, text input retains the public snake-case `text_elements` field,
and image detail remains optional but rejects explicit null. Citation and hook
fields use their public camel-case spellings, and paths remain uninterpreted
strings rather than gaining absolute-path requirements.

These definitions are prerequisites only. They do not alias Gollem's durable
`ThreadRecord`, `TurnRecord`, or `TimelineItem`; bind `item/started` or
`item/completed` to the still-incomplete public `ThreadItem`; or claim that the
runtime emits message, memory-citation, or hook-prompt items. Those bindings
remain separate work after the full item family and its runtime data paths are
implemented.

The exact public `CommandAction` union is also exported independently. Its
`read` path reuses the absolute normalized `AbsolutePathBuf` contract, while
`listFiles.path`, `search.query`, and `search.path` are required nullable
unrestricted strings. The four read/listFiles/search/unknown variants reject
crossed and unknown fields. Exporting this parsed-command value does not parse
live commands, change `CommandExecutionSource`, bind it into Gollem's legacy
command item, or claim that the runtime emits public command actions.

The exact public `McpToolCallAppContext` value is exported with required
`connectorId` and required nullable `linkId`, `resourceUri`, `appName`,
`templateId`, and `actionName` fields. All use the public camel-case spelling,
and canonical output includes explicit nulls. This definition is not bound into
Gollem's legacy MCP item, projected from deprecated `mcpAppResourceUri`, or
treated as evidence that connector metadata exists on current runtime calls.

The exact public `WebSearchAction` union is exported independently with strict
`search`, `openPage`, `findInPage`, and `other` variants. The search query and
query-list, open-page URL, and find-in-page URL/pattern follow the generated v2
TypeScript contract: each variant field is required nullable and canonical
output includes explicit nulls. This deliberately resolves the public JSON
Schema's looser `Option`-derived required list in favor of the generated client
contract. It does not alias the distinct snake-case
`ResponsesApiWebSearchAction`, add a search provider, bind a public thread item,
or change current runtime events.

The schema also exports the exact standalone collab/subagent prerequisite
values: unrestricted logical `AgentPath`, non-empty open `ReasoningEffort`,
closed collab status/tool/tool-call-status and subagent activity enums, and
`CollabAgentState` with required status plus required nullable message. The
state follows the generated v2 TypeScript required-nullable shape rather than
the looser `Option`-derived public JSON Schema required list. These definitions
do not alias current internal agent state, add multi-agent methods, bind collab
or subagent items, alter provider reasoning settings, or claim full public item
parity.

The exact standalone raw-response content prerequisites are exported as strict
validated unions: snake-case input-text/encrypted-content agent message input,
reasoning-text/text reasoning content, and summary-text reasoning summary.
Required content stays non-null, crossed and unknown fields fail closed, and
canonical output preserves public snake-case names. They remain independently
validated dependencies; the parent `ResponseItem` export does not bind
raw-response notifications, map provider payloads, or change live
agent/reasoning deltas.

The distinct `ResponsesApiWebSearchAction` union is also exported independently
with snake-case search/open-page/find-in-page/other discriminators. Optional
query, query-list, URL, and pattern fields are non-null when present and omit
canonically when absent, following generated TypeScript rather than the looser
`Option`-derived JSON Schema nullability. This remains distinct from the
app-server action referenced by `ResponseItem` and does not add a provider/tool
or bind live web-search events.

The standalone `ResponseItem` dependencies and full 16-variant response-item
union are exported as strict validated values. Optional ids, phases, statuses,
namespaces, prompts, actions, content, encrypted content, and metadata are
non-null when present; fields declared nullable by generated TypeScript remain
required and explicit. Arbitrary tool-search arguments and tool arrays retain
all valid JSON values with number precision. The union references the existing
app-server `WebSearchAction` contract and does not bind provider payloads,
durable timeline items, live shell execution, lifecycle methods, or
raw-response notifications.

The next standalone `ThreadItem` dependencies are also exported without live
item adoption: recursive precision-preserving `JsonValue`, strict image
generation records, and exact JSON-valued `McpToolCallResult`. The existing
all-caps MCP runtime result remains distinct because it still carries legacy
`MCPContent[]` rather than general JSON values.

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
`typescript/testdata/command_approval_response_wire_v1.ts`, plus the unbound
permission profile fixture at
`typescript/testdata/permission_profile_wire_v1.ts`. File-change item, dynamic
tool-call, user-input, MCP-elicitation, command-approval, and permission-profile
variants also have strict negative compile contracts. The exported thread-item
message prerequisites have a separate strict compile contract at
`typescript/testdata/thread_item_message_contract.ts`. Tests compare generated
bytes with the checked-in files, verify every type binding references a known
method and definition, and enforce a compatibility floor for runtime items,
daemon status, initialization, thread discovery, thread controls, thread goals,
metadata, lifecycle notifications, command-exec controls, and file changes.
Dynamic client-tool, structured user-input, MCP-elicitation, command approval,
permission-profile, and thread-item prerequisite contracts are covered by the
same floor and strict fixtures.

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
`testdata/permission_profile_wire_v1.json` covers the exact exported public
permission request/response and filesystem/network profile family without
adding an incompatible live result binding. `testdata/filesystem_wire_v1.json`
covers all public filesystem requests and responses, empty response forms,
watch notifications, and the mutation notification extension.

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
