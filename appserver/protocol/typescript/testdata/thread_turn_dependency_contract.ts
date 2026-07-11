import type {
  AgentPath,
  CodexErrorInfo,
  NonSteerableTurnKind,
  SessionSource,
  SubAgentSource,
  ThreadActiveFlag,
  ThreadId,
  ThreadStatus,
  TurnError,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type CodexErrorInfoIsExact = Expect<
  Equal<
    CodexErrorInfo,
    | "contextWindowExceeded"
    | "sessionBudgetExceeded"
    | "usageLimitExceeded"
    | "serverOverloaded"
    | "cyberPolicy"
    | { httpConnectionFailed: { httpStatusCode: number | null } }
    | { responseStreamConnectionFailed: { httpStatusCode: number | null } }
    | "internalServerError"
    | "unauthorized"
    | "badRequest"
    | "threadRollbackFailed"
    | "sandboxError"
    | { responseStreamDisconnected: { httpStatusCode: number | null } }
    | { responseTooManyFailedAttempts: { httpStatusCode: number | null } }
    | { activeTurnNotSteerable: { turnKind: NonSteerableTurnKind } }
    | "other"
  >
>;
type TurnErrorIsExact = Expect<
  Equal<
    TurnError,
    { additionalDetails: string | null; codexErrorInfo: CodexErrorInfo | null; message: string }
  >
>;
type ThreadStatusIsExact = Expect<
  Equal<
    ThreadStatus,
    | { type: "notLoaded" }
    | { type: "idle" }
    | { type: "systemError" }
    | { activeFlags: ThreadActiveFlag[]; type: "active" }
  >
>;
type SubAgentSourceIsExact = Expect<
  Equal<
    SubAgentSource,
    | "review"
    | "compact"
    | {
        thread_spawn: {
          agent_nickname: string | null;
          agent_path: AgentPath | null;
          agent_role: string | null;
          depth: number;
          parent_thread_id: ThreadId;
        };
      }
    | "memory_consolidation"
    | { other: string }
  >
>;
type SessionSourceIsExact = Expect<
  Equal<
    SessionSource,
    "cli" | "vscode" | "exec" | "appServer" | { custom: string } | { subAgent: SubAgentSource } | "unknown"
  >
>;

export const simpleErrors = [
  "contextWindowExceeded",
  "sessionBudgetExceeded",
  "usageLimitExceeded",
  "serverOverloaded",
  "cyberPolicy",
  "internalServerError",
  "unauthorized",
  "badRequest",
  "threadRollbackFailed",
  "sandboxError",
  "other",
] satisfies CodexErrorInfo[];
export const structuredErrors = [
  { httpConnectionFailed: { httpStatusCode: null } },
  { responseStreamConnectionFailed: { httpStatusCode: 503 } },
  { responseStreamDisconnected: { httpStatusCode: null } },
  { responseTooManyFailedAttempts: { httpStatusCode: 429 } },
  { activeTurnNotSteerable: { turnKind: "review" } },
] satisfies CodexErrorInfo[];
export const turnErrors = [
  { message: "", codexErrorInfo: null, additionalDetails: null },
  { message: "failed", codexErrorInfo: "sandboxError", additionalDetails: "details" },
] satisfies TurnError[];
export const threadStatuses = [
  { type: "notLoaded" },
  { type: "idle" },
  { type: "systemError" },
  { type: "active", activeFlags: [] },
  { type: "active", activeFlags: ["waitingOnApproval", "waitingOnUserInput"] },
] satisfies ThreadStatus[];
export const subAgentSources = [
  "review",
  "compact",
  "memory_consolidation",
  { other: "" },
  {
    thread_spawn: {
      parent_thread_id: "thread-1",
      depth: 0,
      agent_path: null,
      agent_nickname: null,
      agent_role: null,
    },
  },
] satisfies SubAgentSource[];
export const sessionSources = [
  "cli",
  "vscode",
  "exec",
  "appServer",
  "unknown",
  { custom: "" },
  { subAgent: "review" },
] satisfies SessionSource[];

// @ts-expect-error error strings are closed.
export const rejectUnknownError = "unknown" satisfies CodexErrorInfo;
// @ts-expect-error HTTP status is required nullable.
export const rejectMissingHttpStatus = { httpConnectionFailed: {} } satisfies CodexErrorInfo;
// @ts-expect-error active turn errors require a closed turn kind.
export const rejectUnknownTurnKind = { activeTurnNotSteerable: { turnKind: "other" } } satisfies CodexErrorInfo;
// @ts-expect-error TurnError requires codexErrorInfo.
export const rejectMissingCodexInfo = { message: "failed", additionalDetails: null } satisfies TurnError;
// @ts-expect-error TurnError requires additionalDetails.
export const rejectMissingAdditionalDetails = { message: "failed", codexErrorInfo: null } satisfies TurnError;
// @ts-expect-error simple status variants cannot carry active flags.
export const rejectIdleFlags = { type: "idle", activeFlags: [] } satisfies ThreadStatus;
// @ts-expect-error active status requires flags.
export const rejectMissingFlags = { type: "active" } satisfies ThreadStatus;
export const rejectMissingAgentPath = {
  // @ts-expect-error spawn fields are required nullable rather than optional.
  thread_spawn: { parent_thread_id: "thread-1", depth: 0, agent_nickname: null, agent_role: null },
} satisfies SubAgentSource;
export const rejectLegacyAgentType = {
  thread_spawn: {
    parent_thread_id: "thread-1",
    depth: 0,
    agent_path: null,
    agent_nickname: null,
    agent_role: null,
    // @ts-expect-error legacy agent_type is not part of the generated contract.
    agent_type: "reviewer",
  },
} satisfies SubAgentSource;
// @ts-expect-error session strings use exact appServer casing.
export const rejectMcpSession = "mcp" satisfies SessionSource;

declare const codexErrorInfoIsExact: CodexErrorInfoIsExact;
declare const turnErrorIsExact: TurnErrorIsExact;
declare const threadStatusIsExact: ThreadStatusIsExact;
declare const subAgentSourceIsExact: SubAgentSourceIsExact;
declare const sessionSourceIsExact: SessionSourceIsExact;
void codexErrorInfoIsExact;
void turnErrorIsExact;
void threadStatusIsExact;
void subAgentSourceIsExact;
void sessionSourceIsExact;
