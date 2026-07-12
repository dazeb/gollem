import type {
  AbsolutePathBuf,
  ApprovalsReviewer,
  AskForApproval,
  LegacyAppPathString,
  ReasoningEffort,
  SandboxPolicy,
  Thread,
  ThreadForkResponse,
  ThreadResumeResponse,
  ThreadStartResponse,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type Response = {
  thread: Thread;
  model: string;
  modelProvider: string;
  serviceTier: string | null;
  cwd: AbsolutePathBuf;
  instructionSources: LegacyAppPathString[];
  approvalPolicy: AskForApproval;
  approvalsReviewer: ApprovalsReviewer;
  sandbox: SandboxPolicy;
  reasoningEffort: ReasoningEffort | null;
};
type Contracts = [
  Expect<Equal<ThreadStartResponse, Response>>,
  Expect<Equal<ThreadResumeResponse, Response>>,
  Expect<Equal<ThreadForkResponse, Response>>,
];

declare const thread: Thread;
export const start = {
  thread,
  model: "",
  modelProvider: "",
  serviceTier: null,
  cwd: "/workspace",
  instructionSources: [],
  approvalPolicy: "never",
  approvalsReviewer: "user",
  sandbox: { type: "dangerFullAccess" },
  reasoningEffort: null,
} satisfies ThreadStartResponse;
export const resume = { ...start } satisfies ThreadResumeResponse;
export const fork = {
  ...start,
  serviceTier: "",
  instructionSources: ["", "relative.md"],
  approvalPolicy: {
    granular: {
      sandbox_approval: true,
      rules: false,
      skill_approval: true,
      request_permissions: false,
      mcp_elicitations: true,
    },
  },
  approvalsReviewer: "guardian_subagent",
  sandbox: {
    type: "workspaceWrite",
    writableRoots: ["/workspace"],
    networkAccess: true,
    excludeTmpdirEnvVar: false,
    excludeSlashTmp: true,
  },
  reasoningEffort: "high",
} satisfies ThreadForkResponse;

// @ts-expect-error every field is required.
export const missingModel: ThreadStartResponse = { ...start, model: undefined };
// @ts-expect-error serviceTier is required nullable, not optional.
export const missingServiceTier: ThreadResumeResponse = (({ serviceTier: _, ...value }) => value)(resume);
// @ts-expect-error instructionSources is a non-null array.
export const nullSources: ThreadForkResponse = { ...fork, instructionSources: null };
// @ts-expect-error array elements are non-null strings.
export const nullSource: ThreadStartResponse = { ...start, instructionSources: [null] };
// @ts-expect-error nested public Thread remains strict.
export const malformedThread: ThreadStartResponse = { ...start, thread: { id: "thread" } };
// @ts-expect-error approval policy remains strict.
export const malformedApproval: ThreadResumeResponse = { ...resume, approvalPolicy: "always" };
// @ts-expect-error reviewer remains strict.
export const malformedReviewer: ThreadForkResponse = { ...fork, approvalsReviewer: "other" };
// @ts-expect-error sandbox policy remains strict.
export const malformedSandbox: ThreadStartResponse = { ...start, sandbox: { type: "readOnly" } };
// @ts-expect-error generated responses exclude experimental fields.
export const experimental: ThreadResumeResponse = { ...resume, runtimeWorkspaceRoots: [] };
// @ts-expect-error generated responses exclude live durable turn fields.
export const durable: ThreadForkResponse = { ...fork, turn: {} };

void (null as unknown as Contracts);
