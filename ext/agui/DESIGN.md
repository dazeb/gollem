# AGUI Integration Design for gollem

This document defines the implementation contract for an AGUI adapter on top of gollem.
It maps AGUI-style session, event, approval, reconnect, and resume concepts onto the
runtime surfaces that already exist in `core`, `ext/temporal`, `ext/graph`, `ext/team`,
and the HTTP handler examples in `contrib`.

The goal is not to invent a new agent runtime. The goal is to normalize what gollem
already emits, identify where the mapping is lossy, and make the missing signals
explicit before implementation starts.

---

## 1. Design goals

An AGUI integration for gollem should provide:

1. A **stable session model** for an agent interaction, regardless of whether the run is:
   - in-process (`Run`, `RunStream`, `Iter`)
   - durable (`ext/temporal`)
   - embedded in a graph (`ext/graph`)
   - part of a team/orchestrator workflow (`ext/team`)
2. A **normalized event stream** that can drive a UI without forcing the UI to know
   gollem internals.
3. A **human approval/external input contract** for tools that block on humans or external
   systems.
4. A **reconnect/resume story** that works for both live transports and durable workflows.
5. An explicit **gap list** so implementation work is scoped and sequenced.

Non-goals for the first AGUI implementation:

- Replacing gollem's existing hooks, traces, or Temporal APIs.
- Inventing a brand-new durable state model when `Snapshot`, `WorkflowStatus`, and
  orchestrator state already exist.
- Perfect fidelity for every provider-specific streaming detail on day one.

---

## 2. Existing gollem surfaces relevant to AGUI

### Core runtime and message surfaces

- `core/message.go`
  - Conversation model: `ModelRequest`, `ModelResponse`
  - Request parts: `SystemPromptPart`, `UserPromptPart`, `ToolReturnPart`, `RetryPromptPart`, etc.
  - Response parts: `TextPart`, `ToolCallPart`, `ThinkingPart`
  - Streaming events: `PartStartEvent`, `PartDeltaEvent`, `PartEndEvent`
- `core/stream.go`
  - `StreamResult[T]` and `agentStream[T]`
  - Live text/event iteration via `StreamText()` and `StreamEvents()`
  - Turn orchestration for streaming runs
- `core/hooks.go`
  - Rich lifecycle hooks: run, turn, model request/response, tool start/end,
    guardrails, output validation/repair, run conditions, context compaction
- `core/runtime_events.go`
  - Event-bus-facing runtime events today: `RunStartedEvent`, `RunCompletedEvent`, `ToolCalledEvent`
- `core/iter.go`
  - Stepwise execution via `Agent.Iter`, one model response per `Next()`
- `core/agent_runtime.go`
  - Exposes runtime config for alternate execution engines like Temporal
- `core/snapshot.go`, `core/run_state.go`, `core/deferred.go`, `core/serialize.go`
  - Resume/checkpoint and deferred-tool primitives

### Temporal surfaces

- `ext/temporal/workflow.go`
  - Durable workflow loop, waiting states, continue-as-new, approval/deferred handling
- `ext/temporal/state.go`
  - `WorkflowStatus`, `ApprovalSignal`, `DeferredResultSignal`, `AbortSignal`
- `ext/temporal/model.go`
  - Durable model request and stream activities
- `ext/temporal/agent.go`
  - `WithEventHandler(...)` exists, but the built-in workflow does not currently use it
- `ext/temporal/README.md`
  - Documents query/signal surface and durable resume model

### Graph surfaces

- `ext/graph/graph.go`
  - Typed node graph with linear execution, conditional routing, fan-out, and reducer

### Team/orchestrator surfaces

- `ext/team/team.go`, `ext/team/teammate.go`, `ext/team/events.go`
  - Team lifecycle, teammate lifecycle, task execution
- `ext/team/store_adapter.go`, `ext/team/tasks.go`
  - Team task visibility and task views
- `ext/orchestrator/events.go`
  - Durable task/lease/command/artifact events

### Transport examples

- `contrib/chi/handler.go`, `contrib/ginhandler/handler.go`, `contrib/echohandler/handler.go`, `contrib/fiberhandler/handler.go`
  - Current SSE shape only streams text deltas plus a final `done` event

---

## 3. Proposed AGUI session contract

AGUI should treat every gollem interaction as a **session** with one canonical identity.

### 3.1 Session identity

Each AGUI session should carry:

| Field | Meaning | Current source |
| --- | --- | --- |
| `session_id` | Stable UI-facing identifier for reconnect/replay | New AGUI-owned ID |
| `run_id` | Current gollem run ID | `RunContext.RunID`, `RunResult.RunID`, `WorkflowStatus.RunID` |
| `parent_run_id` | Parent lineage | `RunContext.ParentRunID`, `WorkflowStatus.ParentRunID` |
| `mode` | `core-run`, `core-stream`, `core-iter`, `temporal`, `graph`, `team` | Adapter-derived |
| `created_at` | Session creation time | Adapter timestamp |
| `status` | `starting`, `running`, `waiting`, `completed`, `failed`, `cancelled`, `aborted` | Derived |

**Important distinction:**

- `run_id` is a gollem/runtime concept.
- `session_id` is the AGUI transport/replay concept.

A single AGUI session may survive reconnects and, in Temporal, may survive continue-as-new.
That is why AGUI needs its own session ID even though gollem already has run IDs.

### 3.2 Session lifecycle

The normalized lifecycle is:

1. `session.opened`
2. `session.input.accepted`
3. `run.started`
4. Zero or more cycles of:
   - `turn.started`
   - `model.request.started`
   - `model.output.*`
   - `model.response.completed`
   - `tool.*` and/or `approval.*` and/or `external_input.*`
   - `turn.completed`
5. Optional waiting state:
   - `session.waiting` with reason `approval` / `deferred` / `approval_and_deferred`
6. Optional resumption:
   - `session.resumed`
7. Terminal event:
   - `session.completed`
   - `session.failed`
   - `session.cancelled`
   - `session.aborted`

### 3.3 Session actions

The AGUI control plane should support these actions:

| AGUI action | Core mapping | Temporal mapping |
| --- | --- | --- |
| `approve_tool_call` | New async approval API required | `ApprovalSignal` |
| `deny_tool_call` | New async approval API required | `ApprovalSignal{Approved:false}` |
| `submit_deferred_result` | Resume with `WithDeferredResults(...)` | `DeferredResultSignal` |
| `abort_session` | Context cancel / new adapter endpoint | `AbortSignal` |
| `resume_session` | `WithSnapshot(...)` + optional `WithDeferredResults(...)` | Re-query stable workflow ID and continue signaling/querying |
| `reconnect_stream` | New replay cursor needed | Query + replay/event sink needed |

---

## 4. AGUI event taxonomy and current gollem mapping

The adapter should emit one normalized stream. Internally, the data comes from different
places today: stream events, hooks, event bus events, trace steps, Temporal queries/signals,
and team/orchestrator state.

## 4.1 Session and lifecycle events

| AGUI event | gollem source today | Notes |
| --- | --- | --- |
| `session.opened` | adapter-created | Not present in core |
| `run.started` | `Hook.OnRunStart`; `RunStartedEvent` in `core/runtime_events.go` | Good fit |
| `turn.started` | `Hook.OnTurnStart` | Hook-only today; not on event bus |
| `turn.completed` | `Hook.OnTurnEnd` | Hook-only today |
| `session.completed` | `Hook.OnRunEnd`; `RunCompletedEvent{Success:true}` | Good fit |
| `session.failed` | `Hook.OnRunEnd`; `RunCompletedEvent{Success:false}` | Good fit |
| `session.cancelled` | derived from context error | Not first-class |
| `session.aborted` | `ext/temporal` abort path | Temporal-only today |
| `session.waiting` | Temporal `WorkflowStatus.Waiting` | Missing in core |
| `session.resumed` | adapter-derived from snapshot/workflow reconnect | Missing as first-class signal |

## 4.2 Model request/response events

| AGUI event | gollem source today | Notes |
| --- | --- | --- |
| `model.request.started` | `Hook.OnModelRequest` | No matching runtime event today |
| `model.output.text.delta` | `PartDeltaEvent` + `TextPartDelta` | Available in `RunStream` |
| `model.output.text.part_started` | `PartStartEvent` + `TextPart` | Available in `RunStream` |
| `model.output.text.part_completed` | `PartEndEvent` | Available in `RunStream` |
| `model.output.thinking.delta` | `PartDeltaEvent` + `ThinkingPartDelta` | Available in `RunStream` |
| `model.output.tool_call.delta` | `PartDeltaEvent` + `ToolCallPartDelta` | Available in `RunStream` |
| `model.response.completed` | `Hook.OnModelResponse`; `StreamedResponse.Response()` | No event-bus runtime event today |
| `model.response.dropped` | response interceptors can drop; adapter can infer | No explicit signal today |

## 4.3 Tool events

| AGUI event | gollem source today | Notes |
| --- | --- | --- |
| `tool.call.requested` | `ToolCallPart` in model response | Can be derived |
| `tool.execution.started` | `Hook.OnToolStart`; `ToolCalledEvent` | Good fit, but runtime event is start-only |
| `tool.execution.completed` | `Hook.OnToolEnd` | Hook-only today |
| `tool.execution.failed` | `Hook.OnToolEnd` with error | Hook-only today |
| `tool.result.returned` | `ToolReturnPart` | Derivable from appended request message |
| `tool.retry_requested` | `RetryPromptPart` | Derivable but not explicit as event |
| `tool.deferred` | `CallDeferred` -> `DeferredToolRequest` | Core + Temporal concept exists |

## 4.4 Approval and external-input events

| AGUI event | gollem source today | Notes |
| --- | --- | --- |
| `approval.requested` | Temporal `PendingApprovals`; core `RequiresApproval` is synchronous only | Temporal has parity; core does not |
| `approval.approved` | Temporal `ApprovalSignal{Approved:true}` | Core lacks eventable async path |
| `approval.denied` | Temporal `ApprovalSignal{Approved:false}` | Core callback only |
| `external_input.requested` | `DeferredToolRequest` | Good fit |
| `external_input.provided` | `WithDeferredResults(...)`; `DeferredResultSignal` | Core resume is batch-based, not event-based |

## 4.5 Topology events for graph/team variants

| AGUI event | gollem source today | Notes |
| --- | --- | --- |
| `graph.node.started` | None | Missing |
| `graph.node.completed` | None | Missing |
| `graph.fanout.started` | None | Missing |
| `graph.fanout.joined` | None | Missing |
| `team.teammate.spawned` | `TeammateSpawnedEvent` | Good fit |
| `team.teammate.idle` | `TeammateIdleEvent` | Good fit |
| `team.teammate.terminated` | `TeammateTerminatedEvent` | Good fit |
| `team.task.claimed/completed/failed/cancelled` | `ext/orchestrator/events.go` | Available at orchestrator layer |
| `team.teammate.output.delta` | None | Missing |

## 4.6 Snapshot/replay events

| AGUI event | gollem source today | Notes |
| --- | --- | --- |
| `session.snapshot` | `core.Snapshot(...)`, `SerializedRunSnapshot`, Temporal `WorkflowStatus.Snapshot` | Good base |
| `session.trace.updated` | `RunTrace` / `TraceStep` / Temporal status trace | Post-hoc or snapshot-like, not live stream |
| `session.replay_checkpoint` | None | Missing |

---

## 5. Backend-specific mapping

## 5.1 In-process `RunStream`

`RunStream` is the best current source for a live AGUI stream.

### What maps cleanly

- `run.started` / `session.completed` via hooks and runtime events
- `model.output.text.delta`, `model.output.thinking.delta`, `model.output.tool_call.delta`
  via `StreamEvents()` and `PartDeltaEvent`
- `model.response.completed` via `completeTurn()` in `core/stream.go`
- `tool.execution.started` / `tool.execution.completed` via hooks
- `session.snapshot` via `Snapshot(rc)` from hook callbacks

### What is lossy today

- Stream events do **not** carry `run_id`, `turn_number`, timestamps, or sequence IDs.
- `RunStream` exposes raw part deltas but not a single normalized envelope.
- Waiting states are only represented for deferred tools as terminal `ErrDeferred`; there is
  no live `waiting` event in the in-process path.
- Core approvals are synchronous callbacks, so there is no transport-neutral
  `approval.requested -> approve/deny -> resume` flow.

## 5.2 In-process `Run`

`Run` can emit coarse AGUI lifecycle events, but not token-level deltas.

Use it for:

- request/response logging
- tool lifecycle
- final transcript assembly
- synchronous UI surfaces that do not need streaming

It is not enough by itself for an AGUI implementation whose main UX is live token streaming.

## 5.3 `Agent.Iter`

`Iter` gives a stable turn boundary, which is useful for AGUI step UIs.

### Good fit

- Each `Next()` corresponds to one model response / one visible step.
- Useful for debugger-like UIs, execution timelines, and step buttons.

### Limitations

- `Iter` is step-granular, not token-granular.
- No first-class pause/resume signals beyond `Close()` and external snapshot handling.

## 5.4 `ext/temporal`

Temporal is the strongest existing base for AGUI durability.

### What maps cleanly

- Stable durable run identity: `WorkflowStatus.RunID`
- Current operational state: `WorkflowStatus`
- Waiting states: `Waiting`, `WaitingReason`, `PendingApprovals`, `DeferredRequests`
- Human actions:
  - `ApprovalSignal`
  - `DeferredResultSignal`
  - `AbortSignal`
- Continue-as-new durability with stable workflow ID targeting
- Resume state via `WorkflowStatus.Snapshot` and `WorkflowOutput.Snapshot`

### What is missing

- The built-in workflow does **not** stream token deltas.
- `TemporalAgent.WithEventHandler(...)` stores a handler, but the built-in workflow never
  invokes it.
- `WorkflowStatus` is a **snapshot** query, not an event replay log.
- There is no reconnectable AGUI event cursor across live durable updates.

### Temporal AGUI model

For the first implementation, Temporal should be treated as:

- **authoritative state store** for reconnect/resume
- **control plane** for approvals and deferred results
- **coarse event source** for waiting/completed/aborted transitions

Not as a token-delta stream unless additional work is done.

## 5.5 `ext/graph`

A graph run should map to a parent AGUI session with nested graph execution events.

### Proposed mapping

- One AGUI session for the graph invocation
- One AGUI span/step per node execution
- Fan-out branches become child spans with branch metadata
- Reducer/join emits a join event before continuing

### Current reality

`ext/graph/graph.go` has no observer/event API. Node execution is internal to `Run(...)`.
So a general AGUI adapter cannot observe:

- node start
- node end
- branch start
- branch completion
- reducer/join

Any AGUI graph UI would therefore require wrappers around node functions or native graph events.

## 5.6 `ext/team` and orchestrator

A team run should map to a parent AGUI session representing the leader/coordinator, with
child sessions or child scopes for each teammate/task.

### What maps cleanly

- teammate lifecycle:
  - `TeammateSpawnedEvent`
  - `TeammateIdleEvent`
  - `TeammateTerminatedEvent`
- orchestrator task lifecycle from `ext/orchestrator/events.go`:
  - task created/claimed/completed/failed/cancelled
  - lease renewed/released
  - commands created/handled
- task result view via `teamTaskView` / `orchestrator.TaskResult`

### What is missing

- No built-in forwarding of each teammate agent's model/tool deltas into a team event stream
- No team-scoped event that says "teammate X is now running task Y" beyond store/task inspection
- No normalized child-session IDs for teammate runs
- No replayable per-teammate transcript stream

## 5.7 `contrib/*handler`

The contrib handlers are a useful proof that SSE works, but they are not AGUI transports.
They currently emit:

- unnamed `data:` events for text delta
- `event: error`
- `event: done` with final usage

They do **not** emit:

- session IDs
- run IDs
- sequence numbers
- lifecycle events
- approval requests
- reconnect cursors
- replay support

AGUI should therefore be implemented as a new transport/adapter layer, not as a thin rename of
existing contrib SSE handlers.

---

## 6. Approval flow contract

## 6.1 Target AGUI approval flow

The normalized approval flow should be:

1. Model emits `tool.call.requested`
2. Runtime decides approval is required
3. AGUI emits `approval.requested` with:
   - `session_id`
   - `run_id`
   - `tool_call_id`
   - `tool_name`
   - `args_json`
   - optional human-readable summary
4. Session enters `waiting`
5. Client sends `approve_tool_call` or `deny_tool_call`
6. AGUI emits `approval.approved` or `approval.denied`
7. Tool either executes or a retry/denial message is returned to the model
8. Session leaves `waiting`

## 6.2 Current core behavior

In core, approval is checked inside `executeSingleTool(...)` in `core/agent.go`:

- if `tool.RequiresApproval` and `toolApprovalFunc` is configured, gollem synchronously calls
  `ToolApprovalFunc(ctx, toolName, argsJSON)`
- if approved, execution continues
- if denied, a `RetryPromptPart` is returned to the model

This is adequate for embedding gollem into an application-specific callback, but **not** for
AGUI parity, because the waiting state is not first-class.

### Consequence

A general AGUI adapter cannot reliably surface a portable `approval.requested` event for
non-Temporal runs unless core grows an async approval API.

## 6.3 Current Temporal behavior

Temporal already has the async shape needed by AGUI:

- if the tool requires approval and no callback approval function is configured,
  the workflow appends `ToolApprovalRequest` to `PendingApprovals`
- `WorkflowStatus.Waiting` becomes true with `WaitingReason == "approval"`
- client resolves it via `ApprovalSignal`

This is the target behavior for AGUI parity across all runtimes.

## 6.4 Session-owned control state (decision)

The transport-facing session must own all mutable state that needs to survive reconnects or appear in
`session.snapshot`. The current `Adapter` should not own approval, deferred-input, or cancel/abort
state because it only formats protocol output and can be detached/recreated independently of the live
session.

For each active AGUI session, keep one run-local control record owned by the session manager (or a
`SessionRuntime` attached to `Session`) with at least:

- current run identity: `run_id`, `parent_run_id`, mode, status, waiting reason
- the active cancellation handle:
  - in-process/core runs: the `context.CancelCauseFunc` or equivalent abort callback for the live run
  - Temporal runs: workflow client handle / stable workflow ID used to send `AbortSignal`
- approval coordination:
  - the blocking `ApprovalBridge` (or Temporal signal bridge) used to resolve the live wait
  - a `pending_approvals` map keyed by `tool_call_id` containing `tool_name`, `args_json`,
    requested timestamp, optional human summary/message, and resolution status
- deferred/external-input coordination:
  - a `pending_external_inputs` map keyed by `tool_call_id` containing the deferred request metadata
- replay/snapshot state:
  - the session sequencer
  - replay buffer/event store
  - the latest resumable snapshot payload plus the replay watermark it reflects

That split gives each layer one job:

- `Adapter`: translate runtime signals into AG-UI protocol payloads
- `Session`/session manager: own mutable run-local state, sequence assignment, snapshots, replay, and
  action routing
- transport: expose SSE and POST/action endpoints over the session-owned state

Approval resolution therefore becomes a two-part model: the bridge/channel unblocks the live run, but
all metadata that must be replayable or snapshottable lives in the session-owned maps above.

---

## 7. Reconnect and resume contract

Reconnect and resume are related, but not the same.

- **Reconnect**: the client lost the transport but the run is still alive.
- **Resume**: the run or process ended/paused and execution must continue from saved state.

## 7.1 Reconnect contract

AGUI reconnect should work like this:

1. Client reconnects with `session_id` and last seen event sequence (`last_seq` / `Last-Event-ID`)
2. Server replays buffered/durable events after that sequence
3. Server resumes live streaming
4. If replay is unavailable, server sends a fresh `session.snapshot` followed by current state

### Transport decision: SSE IDs come from normalized session sequence

The transport must not invent an independent SSE cursor. The replay cursor is the normalized AGUI
session sequence already carried by `Event.Sequence`.

- Every replayable outgoing SSE frame carries `id: <sequence>` where `<sequence>` is the base-10 value
  of the normalized event sequence assigned by the session.
- `Event.ID` remains a stable opaque event identifier for deduplication/debugging, but **not** the SSE
  resume cursor.
- `Last-Event-ID` and `Action.LastSeq` both mean the same thing: the highest normalized sequence the
  client has fully processed.
- Live AG-UI protocol JSON emitted by `Adapter` therefore has to be wrapped before transport writes it:
  the session manager first converts/adopts it into a normalized `Event`, assigns the next sequence,
  appends it to replay storage, and only then writes the SSE frame.

That resolves the current mismatch: replay is defined over the normalized session event log, while the
AG-UI JSON payload remains the `data:` body of the SSE frame.

### Replay algorithm (decision)

Reconnect must use an atomic replay-to-live handoff so the client never misses or duplicates a
live event during the switchover.

On reconnect, the SSE handler should do exactly this:

1. Resolve the target session by `session_id`.
2. Read the resume cursor from `Last-Event-ID` if present; otherwise fall back to explicit `last_seq`.
3. Attach a live subscriber to the session **before** examining replay state. The subscriber should
   queue normalized events rather than writing directly to the socket.
4. In the same session/event-log critical section, capture a replay high-water mark equal to the
   latest committed normalized sequence visible to replay.
5. Ask the replay store for the range `(lastSeq, highWatermark]`.
6. If replay is complete, send those replayed events in order using their original sequence as SSE
   `id`.
7. Drain any queued live events with `sequence > highWatermark` in order.
8. Switch the subscriber from queued mode to direct live streaming.
9. If replay is incomplete (buffer gap / store compaction / process loss), do **not** attempt a
   partial replay from the oldest remaining event. Instead produce one authoritative
   `session.snapshot`, send it with `id == snapshot_sequence`, discard any queued events with
   `sequence <= snapshot_sequence`, then drain queued/live events with `sequence > snapshot_sequence`.

The important invariants are:

- the subscriber is attached before the high-water mark is captured
- the high-water mark is captured from the same session/event-log transaction that defines replay
  visibility
- the handoff to direct live writes happens only after replay-or-snapshot catch-up is complete

### Snapshot fallback contract (decision)

`session.snapshot` is the reconnect fallback whenever the transport cannot guarantee an exact ordered
replay after the requested cursor.

The snapshot payload should include at minimum:

- session identity and status
- current `run_id` / `parent_run_id`
- waiting reason
- pending approvals metadata
- pending deferred/external-input metadata
- any resumable core/Temporal snapshot payload needed for later `resume_session`
- `snapshot_sequence`: the highest normalized event sequence fully reflected in the snapshot

`snapshot_sequence` must be captured atomically with the snapshot contents. In other words, the
session manager must read mutable session state, pending approval/deferred maps, and replay watermark
from the same session/event-log transaction or lock scope so the snapshot cannot describe state that is
older or newer than its advertised watermark.

Transport behavior after sending snapshot fallback:

- SSE `id` for the snapshot frame is `snapshot_sequence`
- the client discards any prior incremental UI state and rebuilds from the snapshot
- live streaming resumes only for events with `sequence > snapshot_sequence`
- queued reconnect events with `sequence <= snapshot_sequence` are dropped because they are already
  covered by the snapshot
- if no live run exists anymore, the snapshot may be the final event returned by reconnect

This makes replay gaps deterministic: exact replay if possible, otherwise one authoritative snapshot
reset instead of a confusing partial history.

### Current gollem support

- In-process core: no native replay cursor or event log
- Temporal: current state is queryable via `WorkflowStatus`, but there is no event replay log
- Team/orchestrator: task state is durable, but message/token replay is not

### Design implication

AGUI needs its own replay buffer or durable event sink.

## 7.2 Resume contract

### Core resume

Core already supports most of the semantic resume payload:

- `core.Snapshot(...)`
- `core.WithSnapshot(...)`
- `core.WithDeferredResults(...)`
- serializable messages and snapshots in `core/serialize.go` and `core/snapshot.go`

This is enough to resume a conversation state, but not enough to resume a **live stream** from
an exact output offset.

### Temporal resume

Temporal resume is stronger:

- state is durable in workflow history
- `WorkflowStatus` exposes snapshot, trace, pending approvals, and deferred requests
- stable workflow ID survives continue-as-new when callers signal/query the workflow ID

The main missing piece is **event replay**, not state replay.

### Graph/team resume

- Graph has no native checkpointing
- Team/orchestrator has durable tasks/results, but not a unified replayable teammate event stream

---

## 8. Proposed AGUI adapter shape

The implementation should be split into two layers.

## 8.1 Normalization layer (`ext/agui`)

A new `ext/agui` package should own:

- `Session`
- `Event`
- event envelope and sequence assignment
- translators from:
  - core stream events
  - hooks/runtime events
  - Temporal status/signals
  - team/orchestrator events
  - graph observer events (once added)
- replay buffer / event store abstraction

## 8.2 Transport layer

The transport layer should expose AGUI over SSE first.

Responsibilities:

- create/open session
- reconnect using `session_id` + sequence
- stream normalized events
- accept actions (approval, deferred result, abort)

Transport implementation notes:

- outgoing SSE frames use normalized `Event.Sequence` as `id`
- the SSE `data:` body can contain either normalized AGUI JSON or wrapped protocol JSON, but replay and
  `Last-Event-ID` always operate on the normalized session sequence
- reconnect must use the atomic replay-to-live handoff defined in section 7.1
- snapshot fallback uses `session.snapshot` with an atomically captured `snapshot_sequence`

This should be a new transport surface, not a retrofit of the current `contrib/*handler`
text-only SSE examples.

---

## 9. Gap analysis: missing gollem signals for AGUI parity

This is the actionable list.

## P0: required for a credible AGUI implementation

| Gap | Why it matters | Files/packages that must change |
| --- | --- | --- |
| No unified AGUI event envelope with sequence/correlation metadata | UI/reconnect logic needs one stable event shape | new `ext/agui`; likely reads from `core/message.go`, `core/stream.go`, `core/runtime_events.go` |
| Runtime event coverage is too small (`run_started`, `run_completed`, `tool_called` only) | AGUI needs turn/model/tool-end/wait/resume events without depending only on hooks | `core/runtime_events.go`, `core/agent.go`, `core/stream.go`, `core/iter.go` |
| Stream events lack `run_id`, `turn_number`, timestamps, and sequence IDs | Raw part deltas cannot be replayed or correlated cleanly | `core/message.go` and/or `core/stream.go` |
| Core approval path is synchronous only | AGUI parity requires async `approval.requested -> approve/deny -> continue` | `core/agent.go`, `core/agent_runtime.go`, `core/hooks.go`, `core/runtime_events.go`, likely new `core/approval.go` |
| Deferred-tool flow has no live wait/resume events in core | AGUI needs explicit waiting + external-input lifecycle | `core/deferred.go`, `core/agent.go`, `core/hooks.go`, `core/runtime_events.go` |
| No replay checkpoint/event cursor in snapshots | Reconnect/resume cannot restart a structured event stream | `core/run_state.go`, `core/snapshot.go`, new `ext/agui` replay store |
| Temporal built-in workflow does not emit live stream events | Durable AGUI needs more than snapshot polling | `ext/temporal/agent.go`, `ext/temporal/workflow.go`, `ext/temporal/model.go`, `ext/temporal/state.go` |
| `TemporalAgent.WithEventHandler(...)` is unused by built-in workflow | There is already an obvious extension point, but it is inert | `ext/temporal/agent.go`, `ext/temporal/workflow.go` |
| Existing HTTP SSE handlers only emit text deltas | They are insufficient as AGUI transports | `contrib/chi/handler.go`, `contrib/ginhandler/handler.go`, `contrib/echohandler/handler.go`, `contrib/fiberhandler/handler.go`, or preferably new `ext/agui` transport package |

## P1: needed for full graph/team parity

| Gap | Why it matters | Files/packages that must change |
| --- | --- | --- |
| `ext/graph` has no observer/event API | AGUI cannot render node timelines, fan-out branches, or reducer joins | `ext/graph/graph.go`, likely new `ext/graph/events.go` |
| `ext/team` lacks per-task/per-teammate run progress events | AGUI can show team membership, but not rich teammate execution | `ext/team/events.go`, `ext/team/team.go`, `ext/team/teammate.go`, possibly `ext/team/tools.go` |
| Team layer does not forward child agent deltas | Needed for nested teammate transcript panes | `ext/team/team.go`, `ext/team/teammate.go`, new `ext/agui` translator |
| No normalized bridge from orchestrator task events to AGUI session/subsession model | Needed for durable team dashboards | `ext/orchestrator/events.go`, `ext/team/*`, new `ext/agui` |

## P2: quality-of-life and simplification

| Gap | Why it matters | Files/packages that must change |
| --- | --- | --- |
| Event bus has no obvious wildcard/runtime subscription API | AGUI adapters would prefer one subscription point instead of N concrete event types | `core/eventbus.go`, `core/runtime_events.go` |
| Trace is mainly post-hoc, not live | Good for history, not enough for real-time UI | `core/trace.go`, `core/agent.go`, `core/stream.go` |
| `Run`/`Iter`/`RunStream` do not expose a common event producer abstraction | AGUI adapter logic is harder than necessary | `core/stream.go`, `core/iter.go`, new adapter interface in `core` or `ext/agui` |

---

## 10. Concrete package-by-package change list

## 10.1 `core`

### `core/runtime_events.go`
Add first-class runtime events for at least:

- `turn_started`
- `turn_completed`
- `model_request_started`
- `model_response_completed`
- `tool_completed`
- `approval_requested`
- `approval_resolved`
- `deferred_requested`
- `deferred_resolved`
- `run_waiting`
- `run_resumed`
- `snapshot_created` or `checkpoint_created`

### `core/agent.go`
Emit the new runtime events and support async approval/deferred waiting in the in-process path.
This is the main file where parity is currently lost.

### `core/stream.go`
Either:

- extend stream events with correlation metadata, or
- wrap them into an AGUI envelope before leaving the core stream path.

Also expose turn/run correlation in a reusable way so transports do not need to reconstruct it.

### `core/hooks.go`
If hooks remain the richest lifecycle source, add hook coverage for:

- waiting entered/exited
- approval requested/resolved
- deferred requested/resolved
- snapshot/checkpoint emitted

### `core/deferred.go`
Keep the existing deferred result model, but add first-class runtime semantics around it.

### `core/run_state.go` and `core/snapshot.go`
Add replay/checkpoint metadata needed by AGUI, such as event sequence or replay watermark.

## 10.2 `ext/temporal`

### `ext/temporal/workflow.go`
This is the durable AGUI pivot point.

Needed changes:

- invoke `EventHandler` or replace it with an explicit workflow event sink
- emit structured live workflow events when waiting/approving/resuming/completing
- optionally persist replayable event envelopes for reconnect

### `ext/temporal/state.go`
`WorkflowStatus` is already strong. It likely only needs:

- session/replay metadata
- optional last event sequence
- optional child session metadata if team/graph are layered on top

### `ext/temporal/model.go`
If token-delta AGUI parity is desired for durable runs, this file needs a way to forward
stream chunks rather than collapsing `ModelRequestStreamActivity` into a final response.

### `ext/temporal/agent.go`
`WithEventHandler(...)` should become active or be replaced by a better-defined AGUI event hook.

## 10.3 `ext/graph`

### `ext/graph/graph.go`
Add graph observer callbacks or event publication for:

- node start/end
- fan-out start/end
- reducer/join
- graph completed/failed

Without this, AGUI graph rendering depends on app-specific wrappers.

## 10.4 `ext/team`

### `ext/team/events.go`
Add richer events such as:

- teammate running task
- teammate completed task
- teammate failed task
- teammate output available
- teammate waiting for approval/external input

### `ext/team/team.go` and `ext/team/teammate.go`
Forward child agent lifecycle into team-scoped events so AGUI can correlate teammate runs to
child sessions without scraping logs or task store state.

## 10.5 `contrib`

The contrib handlers should either:

- remain simple examples, while AGUI ships as a new package, or
- gain a separate AGUI mode that emits structured events and supports reconnect.

Either way, the current text-only SSE shape is not sufficient.

---

## 11. Recommended implementation order

1. **Define `ext/agui` event/session types**
   - normalized envelope
   - sequence IDs
   - replay abstraction
2. **Upgrade core lifecycle signaling**
   - add missing runtime events
   - add async approval/deferred semantics for non-Temporal runs
3. **Build core `RunStream` adapter**
   - easiest path to an end-to-end AGUI demo
4. **Add Temporal state + event bridge**
   - query/signal integration for durable waiting/resume
   - event sink or replay layer
5. **Add graph/team observers**
   - nested session visualization
6. **Add transport package**
   - SSE first, websocket optional later

---

## 12. Bottom line

gollem already has most of the raw ingredients AGUI needs:

- typed messages and streamed deltas
- rich lifecycle hooks
- event bus support
- snapshots and resume inputs
- durable Temporal queries/signals
- team/orchestrator lifecycle events

What it does **not** yet have is a single replayable event contract with first-class waiting,
approval, resume, graph, and team semantics.

The biggest parity gaps are:

1. **core async approval/wait semantics**
2. **broader runtime event coverage**
3. **replay/cursor metadata for reconnect**
4. **Temporal live event emission**
5. **graph/team observer events**

Those are the packages/files that must change before AGUI can be implemented as more than a
text-delta demo.
