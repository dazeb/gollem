import type {
  ApprovalsReviewer,
  AskForApproval,
  JsonValue,
  Personality,
  ReasoningEffort,
  ReasoningSummary,
  SandboxPolicy,
  Turn,
  TurnStartParams,
  TurnStartResponse,
  UserInput,
} from "../gollem_appserver_protocol";

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2)
    ? true
    : false;
type Expect<T extends true> = T;

type TurnStartParamsExpected = {
  threadId: string;
  clientUserMessageId?: string | null;
  input: UserInput[];
  cwd?: string | null;
  approvalPolicy?: AskForApproval | null;
  approvalsReviewer?: ApprovalsReviewer | null;
  sandboxPolicy?: SandboxPolicy | null;
  model?: string | null;
  serviceTier?: string | null;
  effort?: ReasoningEffort | null;
  summary?: ReasoningSummary | null;
  personality?: Personality | null;
  outputSchema?: JsonValue | null;
};

type Contracts = [
  Expect<Equal<ReasoningSummary, "auto" | "concise" | "detailed" | "none">>,
  Expect<Equal<TurnStartParams, TurnStartParamsExpected>>,
  Expect<Equal<TurnStartResponse, { turn: Turn }>>,
];

export const summaries = ["auto", "concise", "detailed", "none"] satisfies ReasoningSummary[];
export const minimal = { threadId: "", input: [] } satisfies TurnStartParams;
export const nullable = {
  threadId: "thread",
  clientUserMessageId: null,
  input: [],
  cwd: null,
  approvalPolicy: null,
  approvalsReviewer: null,
  sandboxPolicy: null,
  model: null,
  serviceTier: null,
  effort: null,
  summary: null,
  personality: null,
  outputSchema: null,
} satisfies TurnStartParams;
export const populated = {
  threadId: "thread",
  clientUserMessageId: "message",
  input: [{ type: "text", text: "hello", text_elements: [] }],
  cwd: "relative",
  approvalPolicy: "never",
  approvalsReviewer: "user",
  sandboxPolicy: { type: "dangerFullAccess" },
  model: "model",
  serviceTier: "tier",
  effort: "high",
  summary: "detailed",
  personality: "pragmatic",
  outputSchema: { type: "object", required: ["answer"] },
} satisfies TurnStartParams;

declare const turn: Turn;
export const response = { turn } satisfies TurnStartResponse;

// @ts-expect-error reasoning summaries are closed.
export const rejectSummary = "summary" satisfies ReasoningSummary;
// @ts-expect-error reasoning summaries are non-null.
export const rejectNullSummary = null satisfies ReasoningSummary;
// @ts-expect-error threadId is required.
export const rejectMissingThread = { input: [] } satisfies TurnStartParams;
// @ts-expect-error threadId is non-null.
export const rejectNullThread = { threadId: null, input: [] } satisfies TurnStartParams;
// @ts-expect-error input is required.
export const rejectMissingInput = { threadId: "thread" } satisfies TurnStartParams;
// @ts-expect-error input is non-null.
export const rejectNullInput = { threadId: "thread", input: null } satisfies TurnStartParams;
// @ts-expect-error input elements are strict UserInput values.
export const rejectMalformedInput = { threadId: "thread", input: [{ type: "text" }] } satisfies TurnStartParams;
// @ts-expect-error request sandbox uses tagged SandboxPolicy.
export const rejectSandboxMode = { threadId: "thread", input: [], sandboxPolicy: "workspace-write" } satisfies TurnStartParams;
// @ts-expect-error approval policy remains strict.
export const rejectApproval = { threadId: "thread", input: [], approvalPolicy: "always" } satisfies TurnStartParams;
// @ts-expect-error fixed params exclude live prompt aliases.
export const rejectPrompt = { threadId: "thread", input: [], prompt: "hello" } satisfies TurnStartParams;
// @ts-expect-error fixed params exclude provider aliases.
export const rejectProvider = { threadId: "thread", input: [], providerId: "provider" } satisfies TurnStartParams;
// @ts-expect-error fixed params exclude experimental workspace roots.
export const rejectWorkspaceRoots = { threadId: "thread", input: [], runtimeWorkspaceRoots: [] } satisfies TurnStartParams;
// @ts-expect-error fixed params exclude experimental collaboration mode.
export const rejectCollaborationMode = { threadId: "thread", input: [], collaborationMode: null } satisfies TurnStartParams;
// @ts-expect-error outputSchema must remain JSON-compatible.
export const rejectOutputSchema = { threadId: "thread", input: [], outputSchema: () => true } satisfies TurnStartParams;
// @ts-expect-error response requires turn.
export const rejectMissingTurn = {} satisfies TurnStartResponse;
// @ts-expect-error nested public Turn remains strict.
export const rejectMalformedTurn = { turn: {} } satisfies TurnStartResponse;
// @ts-expect-error response excludes durable thread fields.
export const rejectResponseThread = { turn, thread: {} } satisfies TurnStartResponse;

void (null as unknown as Contracts);
