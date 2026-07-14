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
Gollem's `threads` array through the live `ThreadListResult` compatibility
type. Existing `statuses` and `includeDeleted` parameters remain optional
Gollem lifecycle extensions; clients that need archived or deleted records
must now request them explicitly and paginate the result.

`thread/read` accepts public `threadId` and optional `includeTurns`. The live
`ThreadReadResult` nests loaded turns and timeline items under the durable
thread record while retaining Gollem's top-level `turns` and `items` arrays and
the existing `id`, `includeItems`, `afterSeq`, and `limit` request extensions.

Exact standalone `ThreadListResponse` and `ThreadReadResponse` definitions use
the public `Thread` parent. The list response requires non-null `data` plus
explicit nullable forward/backward cursors; read requires only `thread`.
Neither canonical response is bound to a live method until durable records are
projected into the public parent. The `*Result` binding names make that runtime
boundary explicit without changing JSON output.

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
clients do not lose current result data. The bound `ThreadUnarchiveResult`
still contains `ThreadRecord`; exact standalone `ThreadUnarchiveResponse`
contains public `Thread` and remains unbound until the runtime thread/turn/item
model converges.

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
`replace` fields remain optional Gollem extensions. The live
`ThreadMetadataUpdateResult` intentionally contains Gollem `ThreadRecord`.
Exact standalone `ThreadMetadataUpdateResponse` contains public `Thread` and
remains unbound until the nested runtime thread model converges.

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

These definitions are also used by the standalone public `ThreadItem` union.
They do not alias Gollem's durable `ThreadRecord`, `TurnRecord`, or
`TimelineItem`, or claim that the runtime emits message, memory-citation, or
hook-prompt items. Exact public lifecycle records now reference the union, but
the method bindings retain Gollem's generic durable and runtime-specialized
compatibility payloads until compatible producer paths are implemented.

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

The exact standalone `RawResponseItemCompletedNotification` requires thread
id, turn id, and a strict `ResponseItem`. It preserves nested arbitrary JSON
number precision and rejects unknown or malformed fields. The
`rawResponseItem/completed` method remains blocked and unbound because Gollem
has no provider/runtime producer for this event; client-originated
`thread/inject_items` keeps its existing durable lifecycle behavior.

The next standalone `ThreadItem` dependencies are also exported without live
item adoption: recursive precision-preserving `JsonValue`, strict image
generation records, and exact JSON-valued `McpToolCallResult`. The existing
all-caps MCP runtime result remains distinct because it still carries legacy
`MCPContent[]` rather than general JSON values.

The exact standalone `CommandExecutionSource` enum exports `agent`,
`userShell`, `unifiedExecStartup`, and `unifiedExecInteraction`. Gollem's live
v1 `CommandExecutionItem.source` deliberately remains its existing inline
`agent | userShell` field and does not reference the broader public enum. This
is an additive type definition only: it does not broaden accepted or emitted
runtime item values, change command producers, or require a protocol-version
bump.

The standalone `WebSearchItem` requires id, query, and an explicit nullable
action. Its action references the existing camel-case v2 `WebSearchAction`;
the root snake-case `ResponsesApiWebSearchAction` remains a separate Responses
API value. The item is a strict generated prerequisite only and does not add a
search provider/tool, alias runtime records, or bind item lifecycle methods.

The complete standalone public `ThreadItem` union exports all 18 generated v2
variants with strict closed objects. Generated required-nullable fields remain
explicit, only deprecated `mcpAppResourceUri` is optional non-null, JSON-valued
arguments retain number precision, string arrays and collab-agent state maps
reject null entries, and sleep duration is a nonnegative integer. Parent-local
validation keeps legacy command, dynamic, MCP, patch-status, file-change, and
MCP-error runtime types unchanged. This definition is intentionally not adopted
by live item producers, durable records, or public `Thread`/`Turn` projections.

Exact `ItemStartedNotification` and `ItemCompletedNotification` definitions
require a strict `ThreadItem`, thread id, turn id, and their corresponding
millisecond timestamp. They are additive canonical variants on `item/started`
and `item/completed`; the prior generic durable payload and four specialized
runtime payload families remain in each method union. Existing producers are
unchanged and continue to emit those explicit compatibility forms.

The first public `Thread`/`Turn` prerequisite group exports standalone
`ThreadId`, `NonSteerableTurnKind`, `TurnItemsView`, `TurnStatus`,
`ThreadActiveFlag`, `ThreadSource`, and `GitInfo`. The two open strings and four
closed enums match their generated contracts; `GitInfo` requires all three
nullable string fields. These definitions remain separate from Gollem's
durable `ThreadLifecycleStatus`, `TurnLifecycleStatus`, filter-facing
`ThreadSourceKind`, and patch-shaped `ThreadMetadataGitInfoUpdateParams`. No
method, item, store record, runtime producer, or parent `Thread`/`Turn` type is
bound by this prerequisite-only addition.

The dependent prerequisite layer exports standalone `CodexErrorInfo`,
`TurnError`, `ThreadStatus`, `SubAgentSource`, and `SessionSource`. Strict
closed unions enforce uint16 HTTP status and int32 spawn-depth bounds, fixed
generated required-nullable fields, exact camel/snake-case variants, and
non-null status flag arrays. Upstream serde omission defaults and the legacy
`agent_type` alias are intentionally rejected because they are absent from the
fixed generated TypeScript contract. These definitions still do not bind
methods/items, replace Gollem runtime or durable provenance/error records, or
project the public parent `Turn` and `Thread` records.

The thread-response policy prerequisite layer exports exact standalone
`ApprovalsReviewer`, `AskForApproval`, `NetworkAccess`, and `SandboxPolicy`
values. Closed reviewer/network enums, every required granular approval flag,
strict sandbox discriminants, fixed generated required booleans, non-null
writable-root arrays, and absolute normalized writable paths are validated at
the Go wire boundary. Upstream serde input defaults are intentionally rejected
when the fixed generated TypeScript output requires the field. These values do
not replace Gollem's runtime approval, permission-profile, workspace, or
sandbox policy models, and no method, item, grant, producer, or enforcement
path is bound by this prerequisite-only addition.

Exact standalone `ThreadStartResponse`, `ThreadResumeResponse`, and
`ThreadForkResponse` definitions share the same strict ten-field public shape.
Every fixed generated field is required; service tier and reasoning effort are
explicit nullable values, instruction source arrays/elements are non-null,
working directories are absolute and normalized, and nested public thread and
policy values retain their own strict validation. Experimental response fields
are excluded. The implemented live start/resume/fork handlers still return
incompatible durable thread/turn maps, so these definitions remain outside the
method-binding map and do not change runtime JSON or claim producer parity.

The request-prerequisite layer exports exact standalone `Personality`,
`SandboxMode`, and `ThreadStartSource` closed string enums. Their fixed public
spellings are validated at the Go wire boundary. `SandboxMode` remains distinct
from the tagged response-side `SandboxPolicy` and Gollem runtime policy models;
`ThreadStartSource` remains distinct from public thread provenance, list-filter
source kinds, and session provenance. These leaves do not bind methods/items,
export their parent start/resume/fork request records, or change runtime model,
policy, or transport behavior.

Exact standalone `ThreadStartParams`, `ThreadResumeParams`, and
`ThreadForkParams` expose only the fixed non-experimental generated fields.
Optional nullable overrides accept omission or null; resume and fork require an
open-string thread id; fork alone gives `ephemeral` optional non-null boolean
semantics. Config values retain strict recursive JSON and request `cwd` remains
a generic string. The current live handlers require incompatible prompt/input,
provider/settings, id-alias, title/metadata, and include-item fields, so these
definitions remain outside method bindings and do not change runtime behavior.

Exact standalone `ReasoningSummary`, `TurnStartParams`, and
`TurnStartResponse` establish the fixed canonical turn-start contract.
`TurnStartParams` requires an open-string thread id and non-null strict
`UserInput` array, preserves nullable model/policy/reasoning/personality and
strict JSON output-schema overrides, and excludes experimental plus Gollem v1
prompt/provider aliases. The current live handler accepts a broader
prompt-driven request and returns a durable turn map, so these definitions
remain outside method bindings pending a separate compatibility migration.

Exact standalone `TurnInterruptParams` and `TurnInterruptResponse` establish
the fixed canonical turn-interrupt contract. The params require open-string
thread and turn ids, and the response is a closed empty object. The current
live handler accepts a legacy id alias without required thread correlation and
returns a richer durable interrupt result, so these definitions remain outside
method bindings pending a separate compatibility migration.

Exact standalone `TurnSteerParams` and `TurnSteerResponse` establish the fixed
canonical turn-steer contract. The params require thread id, strict user input,
and an expected active turn id, with optional nullable client message id; the
response requires turn id. The current live handler instead accepts turn-id and
prompt aliases, does not enforce the public active-turn precondition, persists
a durable steer item, and returns richer status fields, so these definitions
remain outside method bindings pending a separate compatibility migration.

Exact standalone `TurnPlanStepStatus`, `TurnPlanStep`, and
`TurnPlanUpdatedNotification` establish the fixed turn-level plan snapshot.
Canonical output requires an explicit nullable explanation and a non-null plan
array; decoding also accepts an omitted explanation because the pinned Rust
wire type accepts it before serializing the value back as `null`. Gollem has no
exact live producer for `turn/plan/updated`, so the method remains blocked and
unbound rather than claiming notification delivery that does not exist.

Exact standalone `ThreadResumeInitialTurnsPageParams` and `TurnsPage` establish
the experimental resume-pagination value layer without widening the fixed
`ThreadResumeParams` or binding the incompatible live resume handler. Params
accept omitted or explicit-null uint32 limit, sort direction, and item view;
pages require a non-null strict turn array and canonical TypeScript requires
both nullable cursors. The pinned Rust decoder accepts omitted option values
and its serializer emits them as explicit null, so Gollem preserves that decode
compatibility and canonicalizes absent values to null on marshal.

Exact standalone agent-message, plan, reasoning-summary, and reasoning-text
delta notification records establish the fixed item-progress value layer. All
ids and deltas are required open strings; reasoning indexes are required signed
int64 values and generate as TypeScript `number`. The live agent-message and
reasoning-text producers remain unbound because they omit required `itemId` and
emit incompatible `index`/`at` extensions. Plan and reasoning-summary methods
remain blocked until exact producers exist.

Exact standalone warning, guardian-warning, and error notification records
establish the fixed public warning/error value layer. Canonical warning output
requires explicit nullable thread id, while decoding also accepts omission to
match the pinned Rust `Option` input. Guardian warnings require thread id and
message; errors require strict `TurnError`, retry state, thread id, and turn id.
The current live error producer remains unbound because it emits a string
error, optional ids, and an `at` timestamp without retry state. Warning and
guardian-warning methods remain blocked until exact producers exist.

Exact standalone config-warning position, range, and notification records
establish the fixed public config-diagnostic value layer. Positions use
unsigned line/column values; the source documents 1-based coordinates while
the wire type still accepts zero. Canonical config warnings require explicit
nullable details, while decoding also accepts omission to match the pinned
Rust `Option` input. Path and range are optional non-null fields and are
omitted from canonical output when absent. `configWarning` remains blocked and
unbound until Gollem has an exact config-diagnostic producer.

Exact standalone model-reroute, model-verification, turn-moderation-metadata,
and safety-buffering records establish the fixed public model-safety value
layer. Reroute reasons and verification values are closed one-value enums;
verification and buffering arrays are required non-null collections; moderation
metadata preserves arbitrary precision-safe `JsonValue`; and canonical
buffering output requires explicit nullable `fasterModel` while decoding also
accepts omission. All four notification methods remain blocked and unbound
until Gollem has exact model-safety producers.

Exact standalone input-modality, reasoning-effort-option, availability-notice,
service-tier, and upgrade-info records establish the dependency-free public
model-catalog leaf layer. Input modalities are closed; effort options reuse the
existing strict open `ReasoningEffort`; and canonical upgrade output requires
all three nullable strings while decoding also accepts their Rust `Option`
omission form. These values remain separate from `appserver/catalog`'s broader
live model/request/response records and do not change method bindings or runtime
JSON.

The exact standalone `Model` parent composes that leaf layer without binding
`model/list`. Canonical output includes all sixteen fields and non-null arrays;
decoding accepts omitted Rust `Option` values and applies the pinned serde
defaults for modalities, personality support, speed/service tiers, and the
default service tier. Gollem's live catalog intentionally adds provider ids,
capabilities, token limits, and provider-filter request extensions. The two
models remain distinct until an explicit list adapter can preserve those live
features without claiming that the extended response is the exact public
contract.

Exact standalone `ModelListParams` and `ModelListResponse` complete the public
list-envelope value layer without binding `model/list`. Params accept optional
nullable cursor, uint32 limit, and hidden-model selection while canonical
output emits all three fields. Responses require a non-null exact `Model`
array and canonical nullable next cursor while accepting omitted Rust `Option`
cursor input. The live request and response remain provider-extended, so an
explicit projection is still required before generated method inference can
replace `unknown`.

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
`typescript/testdata/thread_item_message_contract.ts`; standalone command
source compatibility is covered at
`typescript/testdata/command_execution_source_contract.ts`, and the standalone
search item at `typescript/testdata/web_search_item_contract.ts`. The full
standalone union has its strict equality and malformed-shape contract at
`typescript/testdata/thread_item_contract.ts`; exact lifecycle records and
their compatibility-preserving method unions are covered at
`typescript/testdata/item_lifecycle_notification_contract.ts`; the standalone,
unbound raw-response completion record is covered at
`typescript/testdata/raw_response_item_completed_contract.ts`; the public
thread/turn leaf prerequisites are covered at
`typescript/testdata/thread_turn_leaf_contract.ts`; their dependent error,
status, and session-source layer is covered at
`typescript/testdata/thread_turn_dependency_contract.ts`; the standalone
thread-response approval/sandbox prerequisites are covered at
`typescript/testdata/thread_response_policy_prerequisite_contract.ts`; the
three exact standalone start/resume/fork responses are covered at
`typescript/testdata/thread_session_response_contract.ts`; and their
request-side enum prerequisites are covered at
`typescript/testdata/thread_request_prerequisite_contract.ts`; the exact
standalone parent request records are covered at
`typescript/testdata/thread_session_param_contract.ts`; the exact
standalone turn-start enum, params, and response are covered at
`typescript/testdata/turn_start_contract.ts`; and the exact standalone
turn-interrupt params and empty response are covered at
`typescript/testdata/turn_interrupt_contract.ts`; and the exact standalone
turn-steer params and response are covered at
`typescript/testdata/turn_steer_contract.ts`; and the exact standalone
turn-plan status, step, and notification are covered at
`typescript/testdata/turn_plan_contract.ts`; and the standalone initial-turn
pagination params and page values are covered at
`typescript/testdata/thread_resume_pagination_contract.ts`; and the exact
standalone item-delta notification values are covered at
`typescript/testdata/item_delta_notification_contract.ts`; the exact
standalone config-warning position, range, and notification values are covered
at `typescript/testdata/config_warning_notification_contract.ts`; the exact
standalone model-safety enum and notification values are covered at
`typescript/testdata/model_safety_notification_contract.ts`; the exact
standalone model-catalog leaf values are covered at
`typescript/testdata/model_catalog_leaf_contract.ts`; and the exact standalone
model parent is covered at `typescript/testdata/model_contract.ts`. Tests
compare generated bytes with the checked-in files, verify every type binding
references a known method and definition, and enforce a compatibility floor for
runtime items, daemon status, initialization, thread discovery, thread controls, thread
goals, metadata, lifecycle notifications, command-exec controls, and file
changes.
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
