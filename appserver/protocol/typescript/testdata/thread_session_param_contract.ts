import type {
  ApprovalsReviewer,
  AskForApproval,
  JsonValue,
  Personality,
  SandboxMode,
  ThreadForkParams,
  ThreadResumeParams,
  ThreadSource,
  ThreadStartParams,
  ThreadStartSource,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type ThreadStartParamsExpected = {
  model?: string | null;
  modelProvider?: string | null;
  serviceTier?: string | null;
  cwd?: string | null;
  approvalPolicy?: AskForApproval | null;
  approvalsReviewer?: ApprovalsReviewer | null;
  sandbox?: SandboxMode | null;
  config?: { [key in string]?: JsonValue } | null;
  serviceName?: string | null;
  baseInstructions?: string | null;
  developerInstructions?: string | null;
  personality?: Personality | null;
  ephemeral?: boolean | null;
  sessionStartSource?: ThreadStartSource | null;
  threadSource?: ThreadSource | null;
};

type ThreadResumeParamsExpected = {
  threadId: string;
  model?: string | null;
  modelProvider?: string | null;
  serviceTier?: string | null;
  cwd?: string | null;
  approvalPolicy?: AskForApproval | null;
  approvalsReviewer?: ApprovalsReviewer | null;
  sandbox?: SandboxMode | null;
  config?: { [key in string]?: JsonValue } | null;
  baseInstructions?: string | null;
  developerInstructions?: string | null;
  personality?: Personality | null;
};

type ThreadForkParamsExpected = {
  threadId: string;
  lastTurnId?: string | null;
  model?: string | null;
  modelProvider?: string | null;
  serviceTier?: string | null;
  cwd?: string | null;
  approvalPolicy?: AskForApproval | null;
  approvalsReviewer?: ApprovalsReviewer | null;
  sandbox?: SandboxMode | null;
  config?: { [key in string]?: JsonValue } | null;
  baseInstructions?: string | null;
  developerInstructions?: string | null;
  ephemeral?: boolean;
  threadSource?: ThreadSource | null;
};

type Contracts = [
  Expect<Equal<ThreadStartParams, ThreadStartParamsExpected>>,
  Expect<Equal<ThreadResumeParams, ThreadResumeParamsExpected>>,
  Expect<Equal<ThreadForkParams, ThreadForkParamsExpected>>,
];

export const startEmpty = {} satisfies ThreadStartParams;
export const startNullable = {
  model: null,
  modelProvider: null,
  serviceTier: null,
  cwd: null,
  approvalPolicy: null,
  approvalsReviewer: null,
  sandbox: null,
  config: null,
  serviceName: null,
  baseInstructions: null,
  developerInstructions: null,
  personality: null,
  ephemeral: null,
  sessionStartSource: null,
  threadSource: null,
} satisfies ThreadStartParams;
export const startPopulated = {
  model: "",
  modelProvider: "provider",
  serviceTier: "",
  cwd: "relative",
  approvalPolicy: "never",
  approvalsReviewer: "user",
  sandbox: "workspace-write",
  config: { nil: null, nested: { ok: true }, omittedByJSON: undefined },
  serviceName: "service",
  baseInstructions: "base",
  developerInstructions: "developer",
  personality: "pragmatic",
  ephemeral: false,
  sessionStartSource: "startup",
  threadSource: "custom",
} satisfies ThreadStartParams;
export const resumeMinimal = { threadId: "" } satisfies ThreadResumeParams;
export const resumePopulated = {
  threadId: "thread",
  model: "model",
  approvalPolicy: "on-request",
  sandbox: "danger-full-access",
  personality: "friendly",
} satisfies ThreadResumeParams;
export const forkMinimal = { threadId: "" } satisfies ThreadForkParams;
export const forkPopulated = {
  threadId: "thread",
  lastTurnId: null,
  ephemeral: false,
  threadSource: "custom",
} satisfies ThreadForkParams;

// @ts-expect-error resume requires threadId.
export const rejectResumeWithoutThread = {} satisfies ThreadResumeParams;
// @ts-expect-error fork requires threadId.
export const rejectForkWithoutThread = {} satisfies ThreadForkParams;
// @ts-expect-error threadId is non-null.
export const rejectNullThread = { threadId: null } satisfies ThreadResumeParams;
// @ts-expect-error fork ephemeral is optional but non-null.
export const rejectNullForkEphemeral = { threadId: "thread", ephemeral: null } satisfies ThreadForkParams;
// @ts-expect-error start ephemeral alone is nullable.
export const rejectResumeEphemeral = { threadId: "thread", ephemeral: false } satisfies ThreadResumeParams;
// @ts-expect-error start-only service name is excluded from resume.
export const rejectResumeService = { threadId: "thread", serviceName: "service" } satisfies ThreadResumeParams;
// @ts-expect-error personality is excluded from fork.
export const rejectForkPersonality = { threadId: "thread", personality: "none" } satisfies ThreadForkParams;
// @ts-expect-error lastTurnId is fork-only.
export const rejectStartLastTurn = { lastTurnId: "turn" } satisfies ThreadStartParams;
// @ts-expect-error fixed start params exclude runtime workspace roots.
export const rejectExperimentalStart = { runtimeWorkspaceRoots: [] } satisfies ThreadStartParams;
// @ts-expect-error fixed resume params exclude history.
export const rejectExperimentalResume = { threadId: "thread", history: [] } satisfies ThreadResumeParams;
// @ts-expect-error fixed fork params exclude path.
export const rejectExperimentalFork = { threadId: "thread", path: "rollout.jsonl" } satisfies ThreadForkParams;
// @ts-expect-error cwd is nullable string, not an array.
export const rejectMalformedCwd = { cwd: [] } satisfies ThreadStartParams;
// @ts-expect-error config values are JSON-compatible or undefined only.
export const rejectFunctionConfig = { config: { bad: () => true } } satisfies ThreadStartParams;
// @ts-expect-error request sandbox uses SandboxMode, not tagged SandboxPolicy.
export const rejectTaggedSandbox = { sandbox: { type: "readOnly", networkAccess: false } } satisfies ThreadStartParams;

void (null as unknown as Contracts);
