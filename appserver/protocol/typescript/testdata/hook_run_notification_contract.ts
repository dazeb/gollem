import type {
  HookCompletedNotification,
  HookRunSummary,
  HookStartedNotification,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Run = {
  id: string;
  eventName: "preToolUse" | "permissionRequest" | "postToolUse" | "preCompact" | "postCompact" | "sessionStart" | "userPromptSubmit" | "subagentStart" | "subagentStop" | "stop";
  handlerType: "command" | "prompt" | "agent";
  executionMode: "sync" | "async";
  scope: "thread" | "turn";
  sourcePath: string;
  source: "system" | "user" | "project" | "mdm" | "sessionFlags" | "plugin" | "cloudRequirements" | "cloudManagedConfig" | "legacyManagedConfigFile" | "legacyManagedConfigMdm" | "unknown";
  displayOrder: bigint;
  status: "running" | "completed" | "failed" | "blocked" | "stopped";
  statusMessage: string | null;
  startedAt: bigint;
  completedAt: bigint | null;
  durationMs: bigint | null;
  entries: Array<{ kind: "warning" | "stop" | "feedback" | "context" | "error"; text: string }>;
};
type Notification = { threadId: string; turnId: string | null; run: HookRunSummary };

type Contracts = [
  Expect<Equal<HookRunSummary, Run>>,
  Expect<Equal<HookStartedNotification, Notification>>,
  Expect<Equal<HookCompletedNotification, Notification>>,
  Expect<Equal<HookStartedNotification, HookCompletedNotification>>,
];
declare const contracts: Contracts;
void contracts;

const run = {
  id: "",
  eventName: "preToolUse",
  handlerType: "command",
  executionMode: "sync",
  scope: "thread",
  sourcePath: "/hooks.json",
  source: "unknown",
  displayOrder: -9_223_372_036_854_775_808n,
  status: "running",
  statusMessage: null,
  startedAt: 9_223_372_036_854_775_807n,
  completedAt: null,
  durationMs: 0n,
  entries: [],
} satisfies HookRunSummary;
({ ...run, entries: [{ kind: "warning", text: "" }, { kind: "warning", text: "" }] }) satisfies HookRunSummary;
({ threadId: "", turnId: null, run }) satisfies HookStartedNotification;
({ threadId: "thread", turnId: " turn ", run }) satisfies HookCompletedNotification;

// @ts-expect-error id is required.
({ ...run, id: undefined }) satisfies HookRunSummary;
// @ts-expect-error eventName is required.
({ ...run, eventName: undefined }) satisfies HookRunSummary;
// @ts-expect-error handlerType is closed.
({ ...run, handlerType: "shell" }) satisfies HookRunSummary;
// @ts-expect-error executionMode is closed.
({ ...run, executionMode: "blocking" }) satisfies HookRunSummary;
// @ts-expect-error scope is closed.
({ ...run, scope: "session" }) satisfies HookRunSummary;
// @ts-expect-error sourcePath is required.
({ ...run, sourcePath: undefined }) satisfies HookRunSummary;
// @ts-expect-error source is required by exact ts-rs output.
({ ...run, source: undefined }) satisfies HookRunSummary;
// @ts-expect-error source is non-null.
({ ...run, source: null }) satisfies HookRunSummary;
// @ts-expect-error displayOrder is bigint.
({ ...run, displayOrder: 0 }) satisfies HookRunSummary;
// @ts-expect-error status is closed.
({ ...run, status: "pending" }) satisfies HookRunSummary;
// @ts-expect-error statusMessage is required by exact ts-rs output.
({ ...run, statusMessage: undefined }) satisfies HookRunSummary;
// @ts-expect-error statusMessage is nullable string only.
({ ...run, statusMessage: 1 }) satisfies HookRunSummary;
// @ts-expect-error startedAt is bigint.
({ ...run, startedAt: 0 }) satisfies HookRunSummary;
// @ts-expect-error completedAt is required by exact ts-rs output.
({ ...run, completedAt: undefined }) satisfies HookRunSummary;
// @ts-expect-error completedAt is nullable bigint only.
({ ...run, completedAt: 0 }) satisfies HookRunSummary;
// @ts-expect-error durationMs is required by exact ts-rs output.
({ ...run, durationMs: undefined }) satisfies HookRunSummary;
// @ts-expect-error durationMs is nullable bigint only.
({ ...run, durationMs: "0" }) satisfies HookRunSummary;
// @ts-expect-error entries are required.
({ ...run, entries: undefined }) satisfies HookRunSummary;
// @ts-expect-error entries are non-null.
({ ...run, entries: null }) satisfies HookRunSummary;
// @ts-expect-error nested entries retain exact kinds.
({ ...run, entries: [{ kind: "info", text: "" }] }) satisfies HookRunSummary;
// @ts-expect-error nested entry text is required.
({ ...run, entries: [{ kind: "warning" }] }) satisfies HookRunSummary;
// @ts-expect-error snake-case aliases do not replace canonical fields.
({ ...run, eventName: undefined, event_name: "preToolUse" }) satisfies HookRunSummary;
// @ts-expect-error fields absent from the public record are rejected.
({ ...run, extra: true }) satisfies HookRunSummary;
// @ts-expect-error threadId is required.
({ turnId: null, run }) satisfies HookStartedNotification;
// @ts-expect-error threadId is non-null.
({ threadId: null, turnId: null, run }) satisfies HookStartedNotification;
// @ts-expect-error turnId is required by exact ts-rs output.
({ threadId: "", run }) satisfies HookStartedNotification;
// @ts-expect-error turnId is nullable string only.
({ threadId: "", turnId: 1, run }) satisfies HookStartedNotification;
// @ts-expect-error run is required.
({ threadId: "", turnId: null }) satisfies HookCompletedNotification;
// @ts-expect-error run is non-null.
({ threadId: "", turnId: null, run: null }) satisfies HookCompletedNotification;
// @ts-expect-error nested runs remain strict.
({ threadId: "", turnId: null, run: { ...run, source: null } }) satisfies HookCompletedNotification;
// @ts-expect-error snake-case aliases do not replace canonical notification fields.
({ thread_id: "", turnId: null, run }) satisfies HookStartedNotification;
// @ts-expect-error fields absent from notifications are rejected.
({ threadId: "", turnId: null, run, extra: true }) satisfies HookCompletedNotification;
