import type {
  HookEventName,
  HookExecutionMode,
  HookHandlerType,
  HookOutputEntry,
  HookOutputEntryKind,
  HookRunStatus,
  HookScope,
  HookSource,
  HookTrustStatus,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Contracts = [
  Expect<Equal<HookEventName,
    | "preToolUse" | "permissionRequest" | "postToolUse"
    | "preCompact" | "postCompact" | "sessionStart"
    | "userPromptSubmit" | "subagentStart" | "subagentStop" | "stop"
  >>,
  Expect<Equal<HookExecutionMode, "sync" | "async">>,
  Expect<Equal<HookHandlerType, "command" | "prompt" | "agent">>,
  Expect<Equal<HookOutputEntryKind, "warning" | "stop" | "feedback" | "context" | "error">>,
  Expect<Equal<HookRunStatus, "running" | "completed" | "failed" | "blocked" | "stopped">>,
  Expect<Equal<HookScope, "thread" | "turn">>,
  Expect<Equal<HookSource,
    | "system" | "user" | "project" | "mdm" | "sessionFlags" | "plugin"
    | "cloudRequirements" | "cloudManagedConfig"
    | "legacyManagedConfigFile" | "legacyManagedConfigMdm" | "unknown"
  >>,
  Expect<Equal<HookTrustStatus, "managed" | "untrusted" | "trusted" | "modified">>,
  Expect<Equal<HookOutputEntry, { kind: HookOutputEntryKind; text: string }>>,
];

declare const contracts: Contracts;
void contracts;

"preToolUse" satisfies HookEventName;
"permissionRequest" satisfies HookEventName;
"postToolUse" satisfies HookEventName;
"preCompact" satisfies HookEventName;
"postCompact" satisfies HookEventName;
"sessionStart" satisfies HookEventName;
"userPromptSubmit" satisfies HookEventName;
"subagentStart" satisfies HookEventName;
"subagentStop" satisfies HookEventName;
"stop" satisfies HookEventName;
"sync" satisfies HookExecutionMode;
"async" satisfies HookExecutionMode;
"command" satisfies HookHandlerType;
"prompt" satisfies HookHandlerType;
"agent" satisfies HookHandlerType;
"warning" satisfies HookOutputEntryKind;
"stop" satisfies HookOutputEntryKind;
"feedback" satisfies HookOutputEntryKind;
"context" satisfies HookOutputEntryKind;
"error" satisfies HookOutputEntryKind;
"running" satisfies HookRunStatus;
"completed" satisfies HookRunStatus;
"failed" satisfies HookRunStatus;
"blocked" satisfies HookRunStatus;
"stopped" satisfies HookRunStatus;
"thread" satisfies HookScope;
"turn" satisfies HookScope;
"system" satisfies HookSource;
"user" satisfies HookSource;
"project" satisfies HookSource;
"mdm" satisfies HookSource;
"sessionFlags" satisfies HookSource;
"plugin" satisfies HookSource;
"cloudRequirements" satisfies HookSource;
"cloudManagedConfig" satisfies HookSource;
"legacyManagedConfigFile" satisfies HookSource;
"legacyManagedConfigMdm" satisfies HookSource;
"unknown" satisfies HookSource;
"managed" satisfies HookTrustStatus;
"untrusted" satisfies HookTrustStatus;
"trusted" satisfies HookTrustStatus;
"modified" satisfies HookTrustStatus;
({ kind: "warning", text: "" }) satisfies HookOutputEntry;
({ kind: "error", text: " error " }) satisfies HookOutputEntry;

// @ts-expect-error event names are closed and case-sensitive.
"PreToolUse" satisfies HookEventName;
// @ts-expect-error execution modes are closed.
"blocking" satisfies HookExecutionMode;
// @ts-expect-error handler types are closed.
"shell" satisfies HookHandlerType;
// @ts-expect-error entry kinds are closed.
"info" satisfies HookOutputEntryKind;
// @ts-expect-error run statuses are closed.
"pending" satisfies HookRunStatus;
// @ts-expect-error scopes are closed.
"session" satisfies HookScope;
// @ts-expect-error sources are closed and case-sensitive.
"session_flags" satisfies HookSource;
// @ts-expect-error trust statuses are closed.
"unknown" satisfies HookTrustStatus;
// @ts-expect-error kind is required.
({ text: "" }) satisfies HookOutputEntry;
// @ts-expect-error text is required.
({ kind: "warning" }) satisfies HookOutputEntry;
// @ts-expect-error kind is non-null.
({ kind: null, text: "" }) satisfies HookOutputEntry;
// @ts-expect-error text is non-null.
({ kind: "warning", text: null }) satisfies HookOutputEntry;
// @ts-expect-error kind retains its exact enum.
({ kind: "info", text: "" }) satisfies HookOutputEntry;
// @ts-expect-error text is a string.
({ kind: "warning", text: 1 }) satisfies HookOutputEntry;
// @ts-expect-error noncanonical aliases do not replace canonical fields.
({ Kind: "warning", text: "" }) satisfies HookOutputEntry;
// @ts-expect-error fields absent from the public record are rejected.
({ kind: "warning", text: "", extra: true }) satisfies HookOutputEntry;
